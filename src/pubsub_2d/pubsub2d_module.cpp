/*
 * Copyright 2017-2020 Yuji Ito <llamerada.jp@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#include "pubsub2d_module.hpp"

#include <cassert>

#include "core/context.hpp"
#include "core/convert.hpp"
#include "core/coord_system.hpp"
#include "core/definition.hpp"
#include "core/scheduler.hpp"
#include "core/utils.hpp"
#include "core/value_impl.hpp"
#include "pubsub_2d_protocol.pb.h"

namespace colonio {

PubSub2DModuleDelegate ::~PubSub2DModuleDelegate() {
}

PubSub2DModule::PubSub2DModule(
    Context& context, ModuleDelegate& module_delegate, Module2DDelegate& module_2d_delegate,
    PubSub2DModuleDelegate& delegate_, APIChannel::Type channel, ModuleChannel::Type module_channel,
    uint32_t cache_time) :
    Module2D(context, module_delegate, module_2d_delegate, channel, module_channel),
    delegate(delegate_),
    CONF_CACHE_TIME(cache_time) {
  context.scheduler.add_interval_task(this, std::bind(&PubSub2DModule::clear_cache, this), 1000);
}

PubSub2DModule::~PubSub2DModule() {
  context.scheduler.remove_task(this);
}

void PubSub2DModule::publish(
    const std::string& name, double x, double y, double r, const Value& value, const std::function<void()>& on_success,
    const std::function<void(Exception::Code)>& on_failure) {
  uint64_t uid  = assign_uid();
  Cache& c      = cache[uid];
  c.name        = name;
  c.center      = Coordinate(x, y);
  c.r           = r;
  c.uid         = uid;
  c.create_time = Utils::get_current_msec();
  c.data        = value;

  if (context.coord_system->get_distance(c.center, context.get_my_position()) < r) {
    if (c.data.get_type() == Value::STRING_T) {
      send_packet_knock(NodeID::NONE, c);

    } else {
      for (auto& it : next_positions) {
        if (context.coord_system->get_distance(c.center, it.second) < r) {
          send_packet_deffuse(it.first, c);
        }
      }
    }
    on_success();

  } else {
    send_packet_pass(c, on_success, on_failure);
  }
}

void PubSub2DModule::module_process_command(std::unique_ptr<const Packet> packet) {
  switch (packet->command_id) {
    case CommandID::PubSub2D::PASS:
      recv_packet_pass(std::move(packet));
      break;

    case CommandID::PubSub2D::KNOCK:
      recv_packet_knock(std::move(packet));
      break;

    case CommandID::PubSub2D::DEFFUSE:
      recv_packet_deffuse(std::move(packet));
      break;

    default:
      // TODO(llamerada.jp@gmail.com) Warning on recving invalid packet.
      assert(false);
  }
}

void PubSub2DModule::module_2d_on_change_my_position(const Coordinate& position) {
  // Ignore.
}

void PubSub2DModule::module_2d_on_change_nearby(const std::set<NodeID>& nids) {
  // Ignore.
}

void PubSub2DModule::module_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) {
  next_positions = positions;
}

PubSub2DModule::CommandKnock::CommandKnock(PubSub2DModule& parent_, uint64_t uid_) :
    Command(CommandID::PubSub2D::KNOCK, PacketMode::NONE),
    parent(parent_),
    uid(uid_) {
}

void PubSub2DModule::CommandKnock::on_error(const std::string& message) {
  // @todo fixme
  assert(false);
}

void PubSub2DModule::CommandKnock::on_failure(std::unique_ptr<const Packet> packet) {
  // Ingore.
}

void PubSub2DModule::CommandKnock::on_success(std::unique_ptr<const Packet> packet) {
  auto it_c = parent.cache.find(uid);
  if (it_c != parent.cache.end()) {
    parent.send_packet_deffuse(packet->src_nid, it_c->second);
  }
}

PubSub2DModule::CommandPass::CommandPass(
    PubSub2DModule& parent_, uint64_t uid_, const std::function<void()>& cb_on_success_,
    const std::function<void(Exception::Code)>& cb_on_failure_) :
    Command(CommandID::PubSub2D::PASS, PacketMode::NONE),
    parent(parent_),
    uid(uid_),
    cb_on_success(cb_on_success_),
    cb_on_failure(cb_on_failure_) {
}

void PubSub2DModule::CommandPass::on_error(const std::string& message) {
  // @todo output log.
  cb_on_failure(Exception::Code::SYSTEM_ERROR);
}

void PubSub2DModule::CommandPass::on_failure(std::unique_ptr<const Packet> packet) {
  Pubsub2DProtocol::PassFailure content;
  packet->parse_content(&content);
  Exception::Code reason = static_cast<Exception::Code>(content.reason());
  cb_on_failure(reason);
}

void PubSub2DModule::CommandPass::on_success(std::unique_ptr<const Packet> packet) {
  cb_on_success();
}

uint64_t PubSub2DModule::assign_uid() {
  uint64_t uid = Utils::get_rnd_64();
  while (cache.find(uid) != cache.end()) {
    uid = Utils::get_rnd_64();
  }
  return uid;
}

void PubSub2DModule::clear_cache() {
  auto it_c = cache.begin();
  while (it_c != cache.end()) {
    if (it_c->second.create_time + CONF_CACHE_TIME < Utils::get_current_msec()) {
      it_c = cache.erase(it_c);

    } else {
      it_c++;
    }
  }
}

void PubSub2DModule::recv_packet_knock(std::unique_ptr<const Packet> packet) {
  Pubsub2DProtocol::Knock content;
  packet->parse_content(&content);
  Coordinate center = Coordinate::from_pb(content.center());
  double r          = content.r();
  uint64_t uid      = content.uid();

  if (cache.find(uid) == cache.end() && context.coord_system->get_distance(context.get_my_position(), center) < r) {
    send_success(*packet, nullptr);
  } else {
    send_failure(*packet, nullptr);
  }
}

void PubSub2DModule::recv_packet_deffuse(std::unique_ptr<const Packet> packet) {
  Pubsub2DProtocol::Deffuse content;
  packet->parse_content(&content);
  Coordinate center = Coordinate::from_pb(content.center());
  double r          = content.r();
  uint64_t uid      = content.uid();

  if (cache.find(uid) == cache.end() && context.coord_system->get_distance(context.get_my_position(), center) < r) {
    const std::string& name = content.name();
    const Value data        = ValueImpl::from_pb(content.data());
    Cache& c                = cache[uid];
    c.name                  = name;
    c.center                = center;
    c.r                     = r;
    c.uid                   = uid;
    c.create_time           = std::time(nullptr);
    c.data                  = data;

    if (c.data.get_type() == Value::STRING_T) {
      send_packet_knock(packet->src_nid, c);

    } else {
      for (auto& it_np : next_positions) {
        if (it_np.first != packet->src_nid && context.coord_system->get_distance(c.center, it_np.second) < r) {
          send_packet_deffuse(it_np.first, c);
        }
      }
    }

    delegate.pubsub2d_module_on_on(*this, name, data);
  }
}

void PubSub2DModule::recv_packet_pass(std::unique_ptr<const Packet> packet) {
  Pubsub2DProtocol::Pass content;
  packet->parse_content(&content);
  Coordinate center = Coordinate::from_pb(content.center());
  double r          = content.r();

  if (context.coord_system->get_distance(center, context.get_my_position()) < r) {
    uint64_t uid  = content.uid();
    Cache& c      = cache[uid];
    c.name        = content.name();
    c.center      = center;
    c.r           = r;
    c.uid         = uid;
    c.create_time = Utils::get_current_msec();
    c.data        = ValueImpl::from_pb(content.data());

    if (c.data.get_type() == Value::STRING_T) {
      // @todo send_success after check the result.
      send_packet_knock(NodeID::NONE, c);

    } else {
      for (auto& it : next_positions) {
        if (context.coord_system->get_distance(c.center, it.second) < r) {
          send_packet_deffuse(it.first, c);
        }
      }
    }
    send_success(*packet, nullptr);

  } else {
    const NodeID& dest = get_relay_nid(center);
    if (dest == NodeID::THIS) {
      Pubsub2DProtocol::PassFailure param;
      param.set_reason(static_cast<uint32_t>(Exception::Code::NO_ONE_RECV));
      send_failure(*packet, serialize_pb(param));
    } else {
      relay_packet(dest, std::move(packet));
    }
  }
}

void PubSub2DModule::send_packet_knock(const NodeID& exclude, const Cache& cache) {
  Pubsub2DProtocol::Knock param;
  cache.center.to_pb(param.mutable_center());
  param.set_r(cache.r);
  param.set_uid(cache.uid);
  std::shared_ptr<const std::string> param_bin = serialize_pb(param);

  for (auto& it_np : next_positions) {
    const NodeID& nid    = it_np.first;
    Coordinate& position = it_np.second;

    if (nid != exclude && context.coord_system->get_distance(cache.center, position) < cache.r) {
      std::unique_ptr<Command> command = std::make_unique<CommandKnock>(*this, cache.uid);
      send_packet(std::move(command), nid, param_bin);
    }
  }
}

void PubSub2DModule::send_packet_deffuse(const NodeID& dst_nid, const Cache& cache) {
  Pubsub2DProtocol::Deffuse param;
  cache.center.to_pb(param.mutable_center());
  param.set_r(cache.r);
  param.set_uid(cache.uid);
  param.set_name(cache.name);
  ValueImpl::to_pb(param.mutable_data(), cache.data);

  send_packet(dst_nid, PacketMode::ONE_WAY, CommandID::PubSub2D::DEFFUSE, serialize_pb(param));
}

void PubSub2DModule::send_packet_pass(
    const Cache& cache, const std::function<void()>& on_success,
    const std::function<void(Exception::Code)>& on_failure) {
  Pubsub2DProtocol::Pass param;
  cache.center.to_pb(param.mutable_center());
  param.set_r(cache.r);
  param.set_uid(cache.uid);
  param.set_name(cache.name);
  ValueImpl::to_pb(param.mutable_data(), cache.data);

  std::unique_ptr<Command> command = std::make_unique<CommandPass>(*this, cache.uid, on_success, on_failure);
  const NodeID& nid                = get_relay_nid(cache.center);
  send_packet(std::move(command), nid, serialize_pb(param));
}
}  // namespace colonio
