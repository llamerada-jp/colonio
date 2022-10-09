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
#include "seed_accessor.hpp"

#include <cassert>
#include <istream>
#include <map>
#include <memory>
#include <sstream>
#include <string>
#include <vector>

#include "colonio.pb.h"
#include "convert.hpp"
#include "logger.hpp"
#include "packet.hpp"
#include "scheduler.hpp"
#include "utils.hpp"

namespace colonio {
SeedAccessorDelegate::~SeedAccessorDelegate() {
}

SeedAccessor::SeedAccessor(
    Logger& l, Scheduler& s, const NodeID& n, SeedAccessorDelegate& d, const std::string& u, const std::string& t) :
    logger(l),
    scheduler(s),
    local_nid(n),
    delegate(d),
    url(u),
    token(t),
    last_connect_time(0),
    auth_status(AuthStatus::NONE),
    hint(SeedHint::NONE) {
}

/**
 */
SeedAccessor::~SeedAccessor() {
  scheduler.remove(this);
  disconnect();
}

/**
 * Try to connect to the seed server over HTTP(S).
 */
void SeedAccessor::connect(unsigned int interval) {
  assert(get_link_state() == LinkState::OFFLINE);
  int64_t current_msec = Utils::get_current_msec();

  // Ignore interval if first connect
  if (last_connect_time == 0) {
    interval = 0;
  }

  auth_status = AuthStatus::NONE;

  SeedLinkParam sl_param(*this, logger);
  link.reset(SeedLink::new_instance(sl_param));

  if (last_connect_time + interval < current_msec) {
    last_connect_time = current_msec;
    link->connect(url);

  } else {
    scheduler.add_task(
        this,
        [this]() {
          int64_t current_msec = Utils::get_current_msec();
          last_connect_time    = current_msec;
          if (link) {
            link->connect(url);
            delegate.seed_accessor_on_change_state();
          }
        },
        last_connect_time + interval - current_msec);
  }

  delegate.seed_accessor_on_change_state();
}

/**
 * Disconnect from the server.
 */
void SeedAccessor::disconnect() {
  log_debug("SeedAccessor::disconnect");
  if (get_link_state() != LinkState::OFFLINE) {
    link.reset();

    delegate.seed_accessor_on_change_state();
  }
}

AuthStatus::Type SeedAccessor::get_auth_status() const {
  return auth_status;
}

LinkState::Type SeedAccessor::get_link_state() const {
  if (link) {
    if (auth_status == AuthStatus::SUCCESS) {
      return LinkState::ONLINE;

    } else {
      return LinkState::CONNECTING;
    }

  } else {
    return LinkState::OFFLINE;
  }
}

bool SeedAccessor::is_only_one() {
  if (get_link_state() == LinkState::ONLINE && (hint & SeedHint::ONLY_ONE) != 0) {
    return true;

  } else {
    return false;
  }
}

void SeedAccessor::relay_packet(std::unique_ptr<const Packet> packet) {
  if (!link) {
    log_warn("reject relaying packet to seed").map("packet", *packet);
    return;
  }

  proto::SeedPacket p;
  proto::SeedRelayPacket* rp = p.mutable_relay_packet();
  packet->dst_nid.to_pb(rp->mutable_dst_nid());
  packet->src_nid.to_pb(rp->mutable_src_nid());
  rp->set_hop_count(packet->hop_count);
  rp->set_id(packet->id);
  rp->set_mode(packet->mode);
  *rp->mutable_content() = packet->content->as_proto();

  log_debug("relay packet to seed").map("packet", p.DebugString());

  std::string bin;
  p.SerializeToString(&bin);
  link->send(bin);
}

void SeedAccessor::seed_link_on_connect(SeedLink& link) {
  scheduler.add_task(this, [this]() {
    send_auth(token);
  });
}

void SeedAccessor::seed_link_on_disconnect(SeedLink& l) {
  if (link.get() == &l) {
    scheduler.add_task(this, [this]() {
      disconnect();
    });
  }
}

void SeedAccessor::seed_link_on_error(SeedLink& l) {
  if (link.get() == &l) {
    scheduler.add_task(this, [this]() {
      disconnect();
    });
  }
}

void SeedAccessor::seed_link_on_recv(SeedLink& link, const std::string& data) {
  proto::SeedPacket packet_pb;
  if (!packet_pb.ParseFromString(data)) {
    /// @todo error
    assert(false);
  }

  scheduler.add_task(this, [this, packet_pb] {
    log_debug("receive packet from seed").map("packet", packet_pb.DebugString());

    switch (packet_pb.Payload_case()) {
      case proto::SeedPacket::kError: {
        recv_error(packet_pb.error());
      } break;

      case proto::SeedPacket::kAuthResponse: {
        recv_auth_response(packet_pb.auth_response());
      } break;

      case proto::SeedPacket::kPing: {
        recv_ping();
      } break;

      case proto::SeedPacket::kHint: {
        recv_hint(packet_pb.hint());
      } break;

      case proto::SeedPacket::kRequireRandom: {
        recv_require_random();
      } break;

      case proto::SeedPacket::kRelayPacket: {
        recv_relay_packet(packet_pb.relay_packet());
      } break;

      default: {
        log_warn("receive unsupported packet from seed").map("packet", packet_pb.DebugString());
      } break;
    }
  });
}

void SeedAccessor::recv_error(const proto::Error& packet) {
  log_error("receive error from seed").map_int("code", packet.code()).map("message", packet.message());
  disconnect();
}

void SeedAccessor::recv_auth_response(const proto::SeedAuthResponse& packet) {
  assert(auth_status == AuthStatus::NONE);

  if (packet.success()) {
    std::istringstream is(packet.config());
    picojson::value v;
    std::string err = picojson::parse(v, is);
    if (!err.empty()) {
      /// @todo error
      assert(false);
    }

    auth_status = AuthStatus::SUCCESS;
    delegate.seed_accessor_on_recv_config(v.get<picojson::object>());
    delegate.seed_accessor_on_change_state();
  } else {
    auth_status = AuthStatus::FAILURE;
    disconnect();
  }
}

void SeedAccessor::recv_ping() {
  send_ping();
}

void SeedAccessor::recv_hint(const proto::SeedHint& packet) {
  SeedHint::Type hint_old = hint;
  hint                    = packet.hint();

  if (hint != hint_old) {
    delegate.seed_accessor_on_change_state();
  }
}

void SeedAccessor::recv_require_random() {
  delegate.seed_accessor_on_recv_require_random();
}

void SeedAccessor::recv_relay_packet(const proto::SeedRelayPacket& seed_packet) {
  std::unique_ptr<Packet> packet = std::make_unique<Packet>(
      NodeID::from_pb(seed_packet.dst_nid()), NodeID::from_pb(seed_packet.src_nid()), seed_packet.id(),
      seed_packet.hop_count(),
      std::make_shared<Packet::Content>(std::make_unique<const proto::PacketContent>(seed_packet.content())),
      seed_packet.mode());
  delegate.seed_accessor_on_recv_packet(std::move(packet));
}

void SeedAccessor::send_auth(const std::string& token) {
  proto::SeedPacket p;
  proto::SeedAuth* auth = p.mutable_auth();
  auth->set_version(PROTOCOL_VERSION);
  local_nid.to_pb(auth->mutable_nid());
  auth->set_token(token);
  auth->set_hint(hint);

  std::string bin;
  p.SerializeToString(&bin);
  link->send(bin);
}

void SeedAccessor::send_ping() {
  proto::SeedPacket p;
  p.set_ping(true);

  std::string bin;
  p.SerializeToString(&bin);
  link->send(bin);
}
}  // namespace colonio
