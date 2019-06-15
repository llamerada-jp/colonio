/*
 * Copyright 2017-2019 Yuji Ito <llamerada.jp@gmail.com>
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

#include <cassert>

#include "core/convert.hpp"
#include "core/definition.hpp"
#include "core/utils.hpp"
#include "core/value_impl.hpp"
#include "pubsub_2d_impl.hpp"

namespace colonio {
PubSub2DImpl::PubSub2DImpl(Context& context, ModuleDelegate& module_delegate,
                           System2DDelegate& system_delegate, const picojson::object& config) :
    System2D(context, module_delegate, system_delegate, Utils::get_json<double>(config, "channel")),
    conf_cache_time(PUBSUB2D_CACHE_TIME) {
  Utils::check_json_optional(config, "cacheTime", &conf_cache_time);

  context.scheduler.add_interval_task(this, std::bind(&PubSub2DImpl::clear_cache, this), 1000);
}

PubSub2DImpl::~PubSub2DImpl() {
  context.scheduler.remove_task(this);
}

void PubSub2DImpl::publish(const std::string& name, double x, double y, double r, const Value& value,
                           const std::function<void()>& on_success,
                           const std::function<void(PubSub2DFailureReason)>& on_failure) {
  uint64_t uid = assign_uid();
  Cache& c = cache[uid];
  c.name = name;
  c.center = Coordinate(x, y);
  c.r = r;
  c.uid = uid;
  c.create_time = Utils::get_current_msec();
  c.data = ValueImpl::to_json(value);

  if (context.coord_system->get_distance(c.center, context.get_my_position()) < r) {
    if (c.data.is<std::string>()) {
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

void PubSub2DImpl::on(const std::string& name,
                      const std::function<void(const Value&)>& subscriber) {
  assert(funcs_subscriber.find(name) == funcs_subscriber.end());

  funcs_subscriber.insert(std::make_pair(name, subscriber));
}

void PubSub2DImpl::off(const std::string& name) {
  funcs_subscriber.erase(name);
}

void PubSub2DImpl::module_process_command(std::unique_ptr<const Packet> packet) {
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

void PubSub2DImpl::system_2d_on_change_my_position(const Coordinate& position) {
  // Ignore.
}

void PubSub2DImpl::system_2d_on_change_nearby(const std::set<NodeID>& nids) {
  // Ignore.
}

void PubSub2DImpl::system_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) {
  next_positions = positions;
}

PubSub2DImpl::CommandKnock::CommandKnock(PubSub2DImpl& parent_, uint64_t uid_) :
    Command(CommandID::PubSub2D::KNOCK, PacketMode::NONE),
    parent(parent_),
    uid(uid_) {
}

void PubSub2DImpl::CommandKnock::on_error(const std::string& message) {
  // @todo fixme
  assert(false);
}

void PubSub2DImpl::CommandKnock::on_failure(std::unique_ptr<const Packet> packet) {
  // Ingore.
}

void PubSub2DImpl::CommandKnock::on_success(std::unique_ptr<const Packet> packet) {
  auto it_c = parent.cache.find(uid);
  if (it_c != parent.cache.end()) {
    parent.send_packet_deffuse(packet->src_nid, it_c->second);
  }
}


PubSub2DImpl::CommandPass::CommandPass(PubSub2DImpl& parent_, uint64_t uid_,
                                       const std::function<void()>& cb_on_success_,
                                       const std::function<void(PubSub2DFailureReason)>& cb_on_failure_) :
    Command(CommandID::PubSub2D::PASS, PacketMode::NONE),
    parent(parent_),
    uid(uid_),
    cb_on_success(cb_on_success_),
    cb_on_failure(cb_on_failure_) {
}

void PubSub2DImpl::CommandPass::on_error(const std::string& message) {
  // @todo output log.
  cb_on_failure(PubSub2DFailureReason::SYSTEM_ERROR);
}

void PubSub2DImpl::CommandPass::on_failure(std::unique_ptr<const Packet> packet) {
  PubSub2DFailureReason reason = static_cast<PubSub2DFailureReason>
    (Convert::json2int<uint32_t>(packet->content.at("reason")));
  cb_on_failure(reason);
}

void PubSub2DImpl::CommandPass::on_success(std::unique_ptr<const Packet> packet) {
  cb_on_success();
}

uint64_t PubSub2DImpl::assign_uid() {
  uint64_t uid = context.get_rnd_64();
  while (cache.find(uid) != cache.end()) {
    uid = context.get_rnd_64();
  }
  return uid;
}

void PubSub2DImpl::clear_cache() {
  auto it_c = cache.begin();
  while (it_c != cache.end()) {
    if (it_c->second.create_time + conf_cache_time < Utils::get_current_msec()) {
      it_c = cache.erase(it_c);

    } else {
      it_c ++;
    }
  }
}

void PubSub2DImpl::recv_packet_knock(std::unique_ptr<const Packet> packet) {
  Coordinate center = Convert::json2coordinate(packet->content.at("center"));
  double r = packet->content.at("r").get<double>();
  uint64_t uid = Convert::json2int<uint64_t>(packet->content.at("uid"));

  if (cache.find(uid) == cache.end() &&
      context.coord_system->get_distance(context.get_my_position(), center) < r) {
    send_success(*packet, picojson::object());
  } else {

    send_failure(*packet, picojson::object());
  }
}

void PubSub2DImpl::recv_packet_deffuse(std::unique_ptr<const Packet> packet) {
  Coordinate center = Convert::json2coordinate(packet->content.at("center"));
  double r = packet->content.at("r").get<double>();
  uint64_t uid = Convert::json2int<uint64_t>(packet->content.at("uid"));
  const std::string& name = packet->content.at("name").get<std::string>();
  const picojson::value& data = packet->content.at("data");

  if (cache.find(uid) == cache.end() &&
      context.coord_system->get_distance(context.get_my_position(), center) < r) {
    Cache& c = cache[uid];
    c.name = name;
    c.center = center;
    c.r = r;
    c.uid = uid;
    c.create_time = std::time(nullptr);
    c.data = data;

    if (c.data.is<std::string>()) {
      send_packet_knock(packet->src_nid, c);

    } else {
      for (auto& it_np : next_positions) {
        if (it_np.first != packet->src_nid &&
            context.coord_system->get_distance(c.center, it_np.second) < r) {
          send_packet_deffuse(it_np.first, c);
        }
      }
    }

    auto subscriber = funcs_subscriber.find(name);
    if (subscriber != funcs_subscriber.end()) {
      subscriber->second(ValueImpl::from_json(data));
    }
  }
}

void PubSub2DImpl::recv_packet_pass(std::unique_ptr<const Packet> packet) {
  Coordinate center = Convert::json2coordinate(packet->content.at("center"));
  double r = packet->content.at("r").get<double>();
  
  if (context.coord_system->get_distance(center, context.get_my_position()) < r) {
    uint64_t uid = Convert::json2int<uint64_t>(packet->content.at("uid"));
    Cache& c = cache[uid];
    c.name = packet->content.at("name").get<std::string>();
    c.center = center;
    c.r = r;
    c.uid = uid;
    c.create_time = Utils::get_current_msec();
    c.data = packet->content.at("data");

    if (c.data.is<std::string>()) {
      // @todo send_success after check the result.
      send_packet_knock(NodeID::NONE, c);

    } else {
      for (auto& it : next_positions) {
        if (context.coord_system->get_distance(c.center, it.second) < r) {
          send_packet_deffuse(it.first, c);
        }
      }
    }
    send_success(*packet, picojson::object());

  } else {
    const NodeID& dest = get_relay_nid(center);
    if (dest == NodeID::THIS) {
      picojson::object param;
      param.insert(std::make_pair("reason", Convert::int2json(static_cast<uint32_t>(PubSub2DFailureReason::NOONE_RECV))));
      send_failure(*packet, param);
    } else {
      relay_packet(dest, std::move(packet));
    }
  }
}

void PubSub2DImpl::send_packet_knock(const NodeID& exclude, const Cache& cache) {
  picojson::object param;
  param.insert(std::make_pair("center", Convert::coordinate2json(cache.center)));
  param.insert(std::make_pair("r", picojson::value(cache.r)));
  param.insert(std::make_pair("uid", Convert::int2json(cache.uid)));

  for (auto& it_np : next_positions) {
    const NodeID& nid = it_np.first;
    Coordinate& position = it_np.second;

    if (nid != exclude &&
        context.coord_system->get_distance(cache.center, position) < cache.r) {
      std::unique_ptr<Command> command = std::make_unique<CommandKnock>(*this, cache.uid);
      send_packet(std::move(command), nid, param);
    }
  }
}

void PubSub2DImpl::send_packet_deffuse(const NodeID& dst_nid, const Cache& cache) {
  picojson::object param;
  param.insert(std::make_pair("center", Convert::coordinate2json(cache.center)));
  param.insert(std::make_pair("r", picojson::value(cache.r)));
  param.insert(std::make_pair("uid", Convert::int2json(cache.uid)));
  param.insert(std::make_pair("name", picojson::value(cache.name)));
  param.insert(std::make_pair("data", cache.data));

  send_packet(dst_nid, PacketMode::ONE_WAY, CommandID::PubSub2D::DEFFUSE, param);
}

void PubSub2DImpl::send_packet_pass(const Cache& cache,
                                    const std::function<void()>& on_success,
                                    const std::function<void(PubSub2DFailureReason)>& on_failure) {
  picojson::object param;
  param.insert(std::make_pair("center", Convert::coordinate2json(cache.center)));
  param.insert(std::make_pair("r", picojson::value(cache.r)));
  param.insert(std::make_pair("uid", Convert::int2json(cache.uid)));
  param.insert(std::make_pair("name", picojson::value(cache.name)));
  param.insert(std::make_pair("data", cache.data));

  std::unique_ptr<Command> command = std::make_unique<CommandPass>(*this, cache.uid, on_success, on_failure);
  const NodeID& nid = get_relay_nid(cache.center);
  send_packet(std::move(command), nid, param);
}
}  // namespace colonio
