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

#include "convert.hpp"
#include "logger.hpp"
#include "module_base.hpp"
#include "packet.hpp"
#include "scheduler.hpp"
#include "seed_accessor_protocol.pb.h"
#include "utils.hpp"

namespace colonio {
SeedAccessorDelegate::~SeedAccessorDelegate() {
}

SeedAccessor::SeedAccessor(
    ModuleParam& param, SeedAccessorDelegate& delegate_, const std::string& url_, const std::string& token_) :
    logger(param.logger),
    scheduler(param.scheduler),
    local_nid(param.local_nid),
    delegate(delegate_),
    url(url_),
    token(token_),
    last_connect_time(0),
    auth_status(AuthStatus::NONE),
    hint(SeedHint::NONE) {
}

/**
 */
SeedAccessor::~SeedAccessor() {
  scheduler.remove_task(this);
  disconnect();
}

/**
 * Try to connect to the seed server over HTTP(S).
 */
void SeedAccessor::connect(unsigned int interval) {
  assert(get_status() == LinkStatus::OFFLINE);
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
    scheduler.add_controller_task(
        this,
        [this]() {
          int64_t current_msec = Utils::get_current_msec();
          last_connect_time    = current_msec;
          if (link) {
            link->connect(url);
            delegate.seed_accessor_on_change_status(*this);
          }
        },
        last_connect_time + interval - current_msec);
  }

  delegate.seed_accessor_on_change_status(*this);
}

/**
 * Disconnect from the server.
 */
void SeedAccessor::disconnect() {
  logd("SeedAccessor::disconnect");
  if (get_status() != LinkStatus::OFFLINE) {
    link.reset();

    delegate.seed_accessor_on_change_status(*this);
  }
}

AuthStatus::Type SeedAccessor::get_auth_status() {
  return auth_status;
}

LinkStatus::Type SeedAccessor::get_status() {
  if (link) {
    if (auth_status == AuthStatus::SUCCESS) {
      return LinkStatus::ONLINE;

    } else {
      return LinkStatus::CONNECTING;
    }

  } else {
    return LinkStatus::OFFLINE;
  }
}

bool SeedAccessor::is_only_one() {
  if (get_status() == LinkStatus::ONLINE && (hint & SeedHint::ONLYONE) != 0) {
    return true;

  } else {
    return false;
  }
}

void SeedAccessor::relay_packet(std::unique_ptr<const Packet> packet) {
  if (link) {
    SeedAccessorProtocol::SeedAccessor packet_sa;
    packet->dst_nid.to_pb(packet_sa.mutable_dst_nid());
    packet->src_nid.to_pb(packet_sa.mutable_src_nid());
    packet_sa.set_hop_count(packet->hop_count);
    packet_sa.set_id(packet->id);
    packet_sa.set_mode(packet->mode);
    packet_sa.set_channel(packet->channel);
    packet_sa.set_command_id(packet->command_id);
    if (packet->content != nullptr) {
      packet_sa.set_content(*packet->content);
    }

    std::string packet_bin;
    packet_sa.SerializeToString(&packet_bin);
    logd("binary to seed").map_int("size", packet_bin.size());  //.map_dump("data", packet_bin);
    link->send(packet_bin);

  } else {
    logw("reject relaying packet to seed").map("packet", *packet);
  }
}

void SeedAccessor::seed_link_on_connect(SeedLink& link) {
  scheduler.add_controller_task(this, [this]() {
    send_auth(token);
  });
}

void SeedAccessor::seed_link_on_disconnect(SeedLink& l) {
  if (link.get() == &l) {
    scheduler.add_controller_task(this, [this]() {
      disconnect();
    });
  }
}

void SeedAccessor::seed_link_on_error(SeedLink& l) {
  if (link.get() == &l) {
    scheduler.add_controller_task(this, [this]() {
      disconnect();
    });
  }
}

void SeedAccessor::seed_link_on_recv(SeedLink& link, const std::string& data) {
  SeedAccessorProtocol::SeedAccessor packet_pb;
  if (!packet_pb.ParseFromString(data)) {
    /// @todo error
    assert(false);
  }

  scheduler.add_controller_task(this, [this, packet_pb] {
    std::shared_ptr<const std::string> content(new std::string(packet_pb.content()));
    logd("packet size").map_int("size", packet_pb.content().size());
    std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(Packet{
        NodeID::from_pb(packet_pb.dst_nid()), NodeID::from_pb(packet_pb.src_nid()), packet_pb.hop_count(),
        packet_pb.id(), content, static_cast<PacketMode::Type>(packet_pb.mode()),
        static_cast<Channel::Type>(packet_pb.channel()), static_cast<CommandID::Type>(packet_pb.command_id())});

    if (packet->src_nid == NodeID::SEED && packet->channel == Channel::SEED_ACCESSOR && packet->id == 0) {
      switch (packet->command_id) {
        case CommandID::SUCCESS: {
          recv_auth_success(*packet);
        } break;

        case CommandID::FAILURE: {
          recv_auth_failure(*packet);
        } break;

        case CommandID::ERROR: {
          recv_auth_error(*packet);
        } break;

        case CommandID::Seed::HINT: {
          recv_hint(*packet);
        } break;

        case CommandID::Seed::PING: {
          recv_ping(*packet);
        } break;

        case CommandID::Seed::REQUIRE_RANDOM: {
          recv_require_random(*packet);
        } break;

        default:
          // @todo output warning log
          assert(false);
          break;
      }

    } else {
      delegate.seed_accessor_on_recv_packet(*this, std::move(packet));
    }
  });
}

void SeedAccessor::recv_auth_success(const Packet& packet) {
  assert(auth_status == AuthStatus::NONE);
  SeedAccessorProtocol::AuthSuccess content;
  packet.parse_content(&content);

  std::istringstream is(content.config());
  picojson::value v;
  std::string err = picojson::parse(v, is);
  if (!err.empty()) {
    /// @todo error
    assert(false);
  }

  auth_status = AuthStatus::SUCCESS;
  delegate.seed_accessor_on_recv_config(*this, v.get<picojson::object>());
  delegate.seed_accessor_on_change_status(*this);
}

void SeedAccessor::recv_auth_failure(const Packet& packet) {
  assert(auth_status == AuthStatus::NONE);
  auth_status = AuthStatus::FAILURE;
  disconnect();
}

void SeedAccessor::recv_auth_error(const Packet& packet) {
  disconnect();
}

void SeedAccessor::recv_hint(const Packet& packet) {
  SeedHint::Type hint_old = hint;
  SeedAccessorProtocol::Hint content;
  packet.parse_content(&content);
  hint = content.hint();

  if (hint != hint_old) {
    delegate.seed_accessor_on_change_status(*this);
  }
}

void SeedAccessor::recv_ping(const Packet& packet) {
  send_ping();
}

void SeedAccessor::recv_require_random(const Packet& packet) {
  delegate.seed_accessor_on_recv_require_random(*this);
}

void SeedAccessor::send_auth(const std::string& token) {
  SeedAccessorProtocol::Auth param;
  param.set_version(PROTOCOL_VERSION);
  param.set_token(token);
  param.set_hint(hint);
  std::shared_ptr<std::string> content(new std::string());
  param.SerializeToString(content.get());

  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(
      Packet{NodeID::SEED, local_nid, 0, 0, content, PacketMode::NONE, Channel::SEED_ACCESSOR, CommandID::Seed::AUTH});

  relay_packet(std::move(packet));
}

void SeedAccessor::send_ping() {
  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(
      Packet{NodeID::SEED, local_nid, 0, 0, nullptr, PacketMode::NONE, Channel::SEED_ACCESSOR, CommandID::Seed::PING});

  relay_packet(std::move(packet));
}
}  // namespace colonio
