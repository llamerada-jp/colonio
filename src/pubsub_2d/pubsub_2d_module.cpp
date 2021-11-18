/*
 * Copyright 2017 Yuji Ito <llamerada.jp@gmail.com>
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

#include "pubsub_2d_module.hpp"

#include <cassert>

#include "colonio/error.hpp"
#include "colonio/pubsub_2d.hpp"
#include "core/convert.hpp"
#include "core/coord_system.hpp"
#include "core/definition.hpp"
#include "core/random.hpp"
#include "core/scheduler.hpp"
#include "core/utils.hpp"
#include "core/value_impl.hpp"
#include "pubsub_2d_protocol.pb.h"

namespace colonio {
Pubsub2DModule* Pubsub2DModule::new_instance(
    ModuleParam& param, Module2DDelegate& module_2d_delegate, const CoordSystem& coord_system,
    const picojson::object& config) {
  Channel::Type channel = static_cast<Channel::Type>(Utils::get_json<double>(config, "channel"));
  uint32_t cache_time   = Utils::get_json<double>(config, "cacheTime", PUBSUB_2D_CACHE_TIME);

  return new Pubsub2DModule(param, module_2d_delegate, coord_system, channel, cache_time);
}

Pubsub2DModule::Pubsub2DModule(
    ModuleParam& param, Module2DDelegate& module_2d_delegate, const CoordSystem& coord_system, Channel::Type channel,
    uint32_t cache_time) :
    Pubsub2DBase(param, module_2d_delegate, coord_system, channel), CONF_CACHE_TIME(cache_time) {
  scheduler.add_controller_loop(this, std::bind(&Pubsub2DModule::clear_cache, this), 1000);
}

Pubsub2DModule::~Pubsub2DModule() {
  scheduler.remove_task(this);
}

void Pubsub2DModule::publish(
    const std::string& name, double x, double y, double r, const Value& value, uint32_t opt,
    std::function<void()>&& on_success, std::function<void(const Error&)>&& on_failure) {
  scheduler.add_controller_task(this, [=] {
    uint64_t uid              = assign_uid();
    Coordinate local_position = coord_system.get_local_position();
    Cache& c                  = cache[uid];
    c.name                    = name;
    c.center                  = Coordinate(x, y);
    c.r                       = r;
    c.uid                     = uid;
    c.create_time             = Utils::get_current_msec();
    c.data                    = value;
    c.opt                     = opt;

    if (coord_system.get_distance(c.center, local_position) < r) {
      if (c.data.get_type() == Value::STRING_T) {
        send_packet_knock(NodeID::NONE, c);

      } else {
        for (auto& it : next_positions) {
          if (coord_system.get_distance(c.center, it.second) < r) {
            send_packet_deffuse(it.first, c);
          }
        }
      }
      on_success();

    } else {
      send_packet_pass(c, on_success, on_failure);
    }
  });
}

void Pubsub2DModule::on(const std::string& name, std::function<void(const Value&)>&& subscriber) {
  if (subscribers.find(name) == subscribers.end()) {
    subscribers.insert(std::make_pair(name, subscriber));
  } else {
    subscribers.at(name) = subscriber;
  }
}

void Pubsub2DModule::off(const std::string& name) {
  auto it = subscribers.find(name);
  if (it != subscribers.end()) {
    subscribers.erase(it);
  }
}

void Pubsub2DModule::module_process_command(std::unique_ptr<const Packet> packet) {
  switch (packet->command_id) {
    case CommandID::Pubsub2D::PASS:
      recv_packet_pass(std::move(packet));
      break;

    case CommandID::Pubsub2D::KNOCK:
      recv_packet_knock(std::move(packet));
      break;

    case CommandID::Pubsub2D::DEFFUSE:
      recv_packet_deffuse(std::move(packet));
      break;

    default:
      // TODO(llamerada.jp@gmail.com) Warning on recving invalid packet.
      assert(false);
  }
}

void Pubsub2DModule::module_2d_on_change_local_position(const Coordinate& position) {
  // Ignore.
}

void Pubsub2DModule::module_2d_on_change_nearby(const std::set<NodeID>& nids) {
  // Ignore.
}

void Pubsub2DModule::module_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) {
  next_positions = positions;
}

Pubsub2DModule::CommandKnock::CommandKnock(Pubsub2DModule& parent_, uint64_t uid_) :
    Command(CommandID::Pubsub2D::KNOCK, PacketMode::NONE), parent(parent_), uid(uid_) {
}

void Pubsub2DModule::CommandKnock::on_error(const std::string& message) {
  // @todo fixme
  assert(false);
}

void Pubsub2DModule::CommandKnock::on_failure(std::unique_ptr<const Packet> packet) {
  // Ingore.
}

void Pubsub2DModule::CommandKnock::on_success(std::unique_ptr<const Packet> packet) {
  auto it_c = parent.cache.find(uid);
  if (it_c != parent.cache.end()) {
    parent.send_packet_deffuse(packet->src_nid, it_c->second);
  }
}

Pubsub2DModule::CommandPass::CommandPass(
    Pubsub2DModule& parent_, uint64_t uid_, std::function<void()>& cb_on_success_,
    std::function<void(const Error&)>& cb_on_failure_) :
    Command(CommandID::Pubsub2D::PASS, PacketMode::NONE),
    parent(parent_),
    uid(uid_),
    cb_on_success(cb_on_success_),
    cb_on_failure(cb_on_failure_) {
}

void Pubsub2DModule::CommandPass::on_error(const std::string& message) {
  // @todo output log.
  cb_on_failure(Error(ErrorCode::SYSTEM_ERROR, ""));
}

void Pubsub2DModule::CommandPass::on_failure(std::unique_ptr<const Packet> packet) {
  Pubsub2DProtocol::PassFailure content;
  packet->parse_content(&content);
  ErrorCode reason = static_cast<ErrorCode>(content.reason());
  cb_on_failure(Error(reason, ""));
}

void Pubsub2DModule::CommandPass::on_success(std::unique_ptr<const Packet> packet) {
  cb_on_success();
}

uint64_t Pubsub2DModule::assign_uid() {
  uint64_t uid = random.generate_u64();
  while (cache.find(uid) != cache.end()) {
    uid = random.generate_u64();
  }
  return uid;
}

void Pubsub2DModule::clear_cache() {
  auto it_c = cache.begin();
  while (it_c != cache.end()) {
    if (it_c->second.create_time + CONF_CACHE_TIME < Utils::get_current_msec()) {
      it_c = cache.erase(it_c);

    } else {
      it_c++;
    }
  }
}

void Pubsub2DModule::recv_packet_knock(std::unique_ptr<const Packet> packet) {
  Pubsub2DProtocol::Knock content;
  packet->parse_content(&content);
  Coordinate center = Coordinate::from_pb(content.center());
  double r          = content.r();
  uint64_t uid      = content.uid();

  if (cache.find(uid) == cache.end() && coord_system.get_distance(coord_system.get_local_position(), center) < r) {
    send_success(*packet, nullptr);
  } else {
    send_failure(*packet, nullptr);
  }
}

void Pubsub2DModule::recv_packet_deffuse(std::unique_ptr<const Packet> packet) {
  Pubsub2DProtocol::Deffuse content;
  packet->parse_content(&content);
  Coordinate center = Coordinate::from_pb(content.center());
  double r          = content.r();
  uint64_t uid      = content.uid();

  if (cache.find(uid) == cache.end() && coord_system.get_distance(coord_system.get_local_position(), center) < r) {
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
        if (it_np.first != packet->src_nid && coord_system.get_distance(c.center, it_np.second) < r) {
          send_packet_deffuse(it_np.first, c);
        }
      }
    }

    auto subscriber = subscribers.find(name);
    if (subscriber != subscribers.end()) {
      std::function<void(const Value&)> s = subscriber->second;
      scheduler.add_user_task(this, [=] {
        s(data);
      });
    }
  }
}

void Pubsub2DModule::recv_packet_pass(std::unique_ptr<const Packet> packet) {
  Pubsub2DProtocol::Pass content;
  packet->parse_content(&content);
  Coordinate center = Coordinate::from_pb(content.center());
  uint64_t uid      = content.uid();
  uint32_t opt      = content.opt();

  if (cache.find(uid) == cache.end()) {
    const Cache& c = cache
                         .insert(std::make_pair(
                             uid,
                             Cache{
                                 content.name(), center, content.r(), uid, Utils::get_current_msec(),
                                 ValueImpl::from_pb(content.data()), opt}))
                         .first->second;
    if (coord_system.get_distance(center, coord_system.get_local_position()) < c.r) {
      if (c.data.get_type() == Value::STRING_T) {
        // @todo send_success after check the result.
        send_packet_knock(NodeID::NONE, c);

      } else {
        for (auto& it : next_positions) {
          if (coord_system.get_distance(c.center, it.second) < c.r) {
            send_packet_deffuse(it.first, c);
          }
        }
      }
      send_success(*packet, nullptr);

      auto subscriber = subscribers.find(c.name);
      if (subscriber != subscribers.end()) {
        std::function<void(const Value&)> s = subscriber->second;
        Value d                             = c.data;
        scheduler.add_user_task(this, [=]() {
          s(d);
        });
      }

    } else {
      const NodeID& dst = get_relay_nid(center);
      if (dst == NodeID::THIS) {
        if (opt & Pubsub2D::RAISE_NO_ONE_RECV) {
          Pubsub2DProtocol::PassFailure param;
          param.set_reason(static_cast<uint32_t>(ErrorCode::NO_ONE_RECV));
          send_failure(*packet, serialize_pb(param));
        } else {
          send_success(*packet, nullptr);
        }
      } else {
        relay_packet(dst, std::move(packet));
      }
    }
  } else {
    if (opt & Pubsub2D::RAISE_NO_ONE_RECV) {
      Pubsub2DProtocol::PassFailure param;
      param.set_reason(static_cast<uint32_t>(ErrorCode::NO_ONE_RECV));
      send_failure(*packet, serialize_pb(param));
    } else {
      send_success(*packet, nullptr);
    }
  }
}

void Pubsub2DModule::send_packet_knock(const NodeID& exclude, const Cache& cache) {
  Pubsub2DProtocol::Knock param;
  cache.center.to_pb(param.mutable_center());
  param.set_r(cache.r);
  param.set_uid(cache.uid);
  std::shared_ptr<const std::string> param_bin = serialize_pb(param);

  for (auto& it_np : next_positions) {
    const NodeID& nid    = it_np.first;
    Coordinate& position = it_np.second;

    if (nid != exclude && coord_system.get_distance(cache.center, position) < cache.r) {
      std::unique_ptr<Command> command = std::make_unique<CommandKnock>(*this, cache.uid);
      send_packet(std::move(command), nid, param_bin);
    }
  }
}

void Pubsub2DModule::send_packet_deffuse(const NodeID& dst_nid, const Cache& cache) {
  Pubsub2DProtocol::Deffuse param;
  cache.center.to_pb(param.mutable_center());
  param.set_r(cache.r);
  param.set_uid(cache.uid);
  param.set_name(cache.name);
  ValueImpl::to_pb(param.mutable_data(), cache.data);

  send_packet(dst_nid, PacketMode::ONE_WAY, CommandID::Pubsub2D::DEFFUSE, serialize_pb(param));
}

void Pubsub2DModule::send_packet_pass(
    const Cache& cache, std::function<void()> on_success, std::function<void(const Error&)> on_failure) {
  Pubsub2DProtocol::Pass param;
  cache.center.to_pb(param.mutable_center());
  param.set_r(cache.r);
  param.set_uid(cache.uid);
  param.set_name(cache.name);
  ValueImpl::to_pb(param.mutable_data(), cache.data);
  param.set_opt(cache.opt);

  std::unique_ptr<Command> command = std::make_unique<CommandPass>(*this, cache.uid, on_success, on_failure);
  const NodeID& nid                = get_relay_nid(cache.center);
  send_packet(std::move(command), nid, serialize_pb(param));
}
}  // namespace colonio
