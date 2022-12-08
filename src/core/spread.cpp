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

#include "spread.hpp"

#include <cassert>

#include "command_manager.hpp"
#include "coord_system.hpp"
#include "definition.hpp"
#include "logger.hpp"
#include "node_id.hpp"
#include "random.hpp"
#include "scheduler.hpp"
#include "utils.hpp"
#include "value_impl.hpp"

namespace colonio {
SpreadDelegate::~SpreadDelegate() {
}

Spread::Spread(
    Logger& l, Random& r, Scheduler& s, CommandManager& c, const NodeID& n, CoordSystem& cs, SpreadDelegate& d,
    const picojson::object& config) :
    CONF_CACHE_TIME(Utils::get_json<double>(config, "cacheTime", SPREAD_CACHE_TIME)),
    logger(l),
    random(r),
    scheduler(s),
    command_manager(c),
    local_nid(n),
    coord_system(cs),
    delegate(d) {
  scheduler.repeat_task(this, std::bind(&Spread::clear_cache, this), 1000);

  command_manager.set_handler(
      proto::PacketContent::kSpread, std::bind(&Spread::recv_spread, this, std::placeholders::_1));
  command_manager.set_handler(
      proto::PacketContent::kSpreadKnock, std::bind(&Spread::recv_knock, this, std::placeholders::_1));
  command_manager.set_handler(
      proto::PacketContent::kSpreadRelay, std::bind(&Spread::recv_relay, this, std::placeholders::_1));
}

Spread::~Spread() {
  scheduler.remove(this);
}

void Spread::post(
    double x, double y, double r, const std::string& name, const Value& message, uint32_t opt,
    std::function<void()>&& on_success, std::function<void(const Error&)>&& on_failure) {
  uint64_t uid              = assign_uid();
  Coordinate local_position = coord_system.get_local_position();
  Cache& c                  = cache[uid];
  c.src                     = local_nid;
  c.name                    = name;
  c.center                  = Coordinate(x, y);
  c.r                       = r;
  c.uid                     = uid;
  c.create_time             = Utils::get_current_msec();
  c.message                 = message;
  c.opt                     = opt;

  if (coord_system.get_distance(c.center, local_position) < r) {
    if (c.message.get_type() == Value::STRING_T || c.message.get_type() == Value::BINARY_T) {
      send_knock(NodeID::NONE, c);

    } else {
      for (auto& it : next_positions) {
        if (coord_system.get_distance(c.center, it.second) < r) {
          send_spread(it.first, c);
        }
      }
    }
    on_success();

  } else {
    send_relay(c, on_success, on_failure);
  }
}

void Spread::set_handler(const std::string& name, std::function<void(const Colonio::SpreadRequest&)>&& handler) {
  if (handlers.find(name) == handlers.end()) {
    handlers.insert(std::make_pair(name, handler));
  } else {
    handlers.at(name) = handler;
  }
}

void Spread::unset_handler(const std::string& name) {
  handlers.erase(name);
}

void Spread::update_next_positions(const std::map<NodeID, Coordinate>& positions) {
  next_positions = positions;
}

Spread::CommandKnock::CommandKnock(Spread& p, uint64_t u) :
    Command(PacketMode::NONE), logger(p.logger), parent(p), uid(u) {
}

void Spread::CommandKnock::on_error(ErrorCode code, const std::string& message) {
  // @todo fixme
  assert(false);
}

void Spread::CommandKnock::on_response(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  if (!c.has_spread_response()) {
    log_debug("an unexpected packet received").map("packet", packet);
    return;
  }

  const proto::SpreadResponse& response = c.spread_response();
  // The node send response may be out of target range.
  if (!response.success()) {
    return;
  }

  auto it_c = parent.cache.find(uid);
  if (it_c != parent.cache.end()) {
    parent.send_spread(packet.src_nid, it_c->second);
  }
}

Spread::CommandRelay::CommandRelay(Spread& p, std::function<void()>& s, std::function<void(const Error&)>& f) :
    Command(PacketMode::NONE), logger(p.logger), parent(p), cb_on_success(s), cb_on_failure(f) {
}

void Spread::CommandRelay::on_response(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  if (!c.has_spread_response()) {
    log_debug("an unexpected packet received").map("packet", packet);
    return;
  }

  const proto::SpreadResponse& response = c.spread_response();
  if (!response.success()) {
    cb_on_failure(colonio_error(static_cast<ErrorCode>(response.code()), response.message()));
    return;
  }

  cb_on_success();
}

void Spread::CommandRelay::on_error(ErrorCode code, const std::string& message) {
  // @todo output log.
  cb_on_failure(colonio_error(code, message));
}

uint64_t Spread::assign_uid() {
  uint64_t uid = random.generate_u64();
  while (cache.find(uid) != cache.end()) {
    uid = random.generate_u64();
  }
  return uid;
}

void Spread::clear_cache() {
  auto it_c = cache.begin();
  while (it_c != cache.end()) {
    if (it_c->second.create_time + CONF_CACHE_TIME < Utils::get_current_msec()) {
      it_c = cache.erase(it_c);

    } else {
      it_c++;
    }
  }
}

void Spread::recv_spread(const Packet& packet) {
  proto::Spread content = packet.content->as_proto().spread();
  Coordinate center     = Coordinate::from_pb(content.center());
  double r              = content.r();
  uint64_t uid          = content.uid();

  if (cache.find(uid) == cache.end() && coord_system.get_distance(coord_system.get_local_position(), center) < r) {
    const std::string& name = content.name();
    Cache& c                = cache[uid];
    c.src                   = NodeID::from_pb(content.source());
    c.name                  = name;
    c.center                = center;
    c.r                     = r;
    c.uid                   = uid;
    c.create_time           = Utils::get_current_msec();
    c.message               = ValueImpl::from_pb(content.message());
    c.opt                   = content.opt();

    if (c.message.get_type() == Value::STRING_T || c.message.get_type() == Value::BINARY_T) {
      send_knock(packet.src_nid, c);

    } else {
      for (auto& it_np : next_positions) {
        if (it_np.first != packet.src_nid && coord_system.get_distance(c.center, it_np.second) < r) {
          send_spread(it_np.first, c);
        }
      }
    }

    auto handler = handlers.find(name);
    if (handler != handlers.end()) {
      Colonio::SpreadRequest request;
      request.source_nid = c.src.to_str();
      request.message    = c.message;
      request.options    = c.opt;
      handler->second(request);
    }
  }
}

void Spread::recv_knock(const Packet& packet) {
  proto::SpreadKnock content = packet.content->as_proto().spread_knock();
  Coordinate center          = Coordinate::from_pb(content.center());
  double r                   = content.r();
  uint64_t uid               = content.uid();

  if (cache.find(uid) == cache.end() && coord_system.get_distance(coord_system.get_local_position(), center) < r) {
    send_response(packet, true, ErrorCode::UNDEFINED, "");
  } else {
    send_response(packet, false, ErrorCode::UNDEFINED, "");
  }
}

void Spread::recv_relay(const Packet& packet) {
  proto::SpreadRelay content = packet.content->as_proto().spread_relay();
  Coordinate center          = Coordinate::from_pb(content.center());
  uint64_t uid               = content.uid();
  uint32_t opt               = content.opt();

  if (cache.find(uid) == cache.end()) {
    Cache& c      = cache[uid];
    c.src         = NodeID::from_pb(content.source());
    c.name        = content.name();
    c.center      = center;
    c.r           = content.r();
    c.uid         = uid;
    c.create_time = Utils::get_current_msec();
    c.message     = ValueImpl::from_pb(content.message());
    c.opt         = opt;

    if (coord_system.get_distance(center, coord_system.get_local_position()) < c.r) {
      if (c.message.get_type() == Value::STRING_T || c.message.get_type() == Value::BINARY_T) {
        // @todo send_success after check the result.
        send_knock(NodeID::NONE, c);

      } else {
        for (auto& it : next_positions) {
          if (coord_system.get_distance(c.center, it.second) < c.r) {
            send_spread(it.first, c);
          }
        }
      }
      send_response(packet, true, ErrorCode::UNDEFINED, "");

      auto handler = handlers.find(c.name);
      if (handler != handlers.end()) {
        Colonio::SpreadRequest request;
        request.source_nid = c.src.to_str();
        request.message    = c.message;
        request.options    = c.opt;
        handler->second(request);
      }
      return;

    } else {
      const NodeID& dst = delegate.spread_do_get_relay_nid(center);
      if (dst == NodeID::THIS) {
        if (opt & Colonio::SPREAD_SOMEONE_MUST_RECEIVE) {
          send_response(packet, false, ErrorCode::SPREAD_NO_ONE_RECEIVE, "no one receive the message");

        } else {
          send_response(packet, true, ErrorCode::UNDEFINED, "");
        }
      } else {
        command_manager.relay_packet(dst, packet.make_copy());
      }
    }

  } else {
    if (opt & Colonio::SPREAD_SOMEONE_MUST_RECEIVE) {
      send_response(packet, false, ErrorCode::SPREAD_NO_ONE_RECEIVE, "no one receive the message");

    } else {
      send_response(packet, true, ErrorCode::UNDEFINED, "");
    }
  }
}

void Spread::send_spread(const NodeID& dst_nid, const Cache& cache) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::Spread& param                          = *content->mutable_spread();

  cache.src.to_pb(param.mutable_source());
  cache.center.to_pb(param.mutable_center());
  param.set_r(cache.r);
  param.set_uid(cache.uid);
  param.set_name(cache.name);
  ValueImpl::to_pb(param.mutable_message(), cache.message);
  param.set_opt(cache.opt);

  command_manager.send_packet_one_way(dst_nid, 0, std::move(content));
}

void Spread::send_knock(const NodeID& exclude, const Cache& cache) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::SpreadKnock& param                     = *content->mutable_spread_knock();

  cache.center.to_pb(param.mutable_center());
  param.set_r(cache.r);
  param.set_uid(cache.uid);

  std::shared_ptr<Packet::Content> pc = std::make_shared<Packet::Content>(std::move(content));
  for (auto& it_np : next_positions) {
    const NodeID& nid    = it_np.first;
    Coordinate& position = it_np.second;

    if (nid != exclude && coord_system.get_distance(cache.center, position) < cache.r) {
      std::shared_ptr<Command> command = std::make_shared<CommandKnock>(*this, cache.uid);
      command_manager.send_packet(nid, pc, command);
    }
  }
}

void Spread::send_relay(
    const Cache& cache, std::function<void()> on_success, std::function<void(const Error&)> on_failure) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::SpreadRelay& param                     = *content->mutable_spread_relay();

  cache.src.to_pb(param.mutable_source());
  cache.center.to_pb(param.mutable_center());
  param.set_r(cache.r);
  param.set_uid(cache.uid);
  param.set_name(cache.name);
  ValueImpl::to_pb(param.mutable_message(), cache.message);
  param.set_opt(cache.opt);

  std::shared_ptr<Command> command = std::make_shared<CommandRelay>(*this, on_success, on_failure);
  const NodeID& nid                = delegate.spread_do_get_relay_nid(cache.center);
  command_manager.send_packet(nid, std::move(content), command);
}

void Spread::send_response(const Packet& packet, bool success, ErrorCode code, const std::string& message) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::SpreadResponse& param                  = *content->mutable_spread_response();

  param.set_success(success);
  if (!success) {
    param.set_code(static_cast<uint32_t>(code));
    param.set_message(message);
  }

  command_manager.send_response(packet, std::move(content));
}

}  // namespace colonio
