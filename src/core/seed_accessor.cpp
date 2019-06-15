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
#include <istream>
#include <map>
#include <memory>
#include <sstream>
#include <string>
#include <vector>

#include "context.hpp"
#include "convert.hpp"
#include "logger.hpp"
#include "seed_accessor.hpp"
#include "utils.hpp"

namespace colonio {
SeedAccessorDelegate::~SeedAccessorDelegate() {
}

SeedAccessor::SeedAccessor(Context& context_, SeedAccessorDelegate& delegate_,
                           const std::string& url_, const std::string& token_) :
    context(context_),
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
  context.scheduler.remove_task(this);  
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
  link = std::make_unique<SeedLink>(*this, context);
  
  if (last_connect_time + interval < current_msec) {
    last_connect_time = current_msec;
    link->connect(url);

  } else {
    context.scheduler.add_timeout_task(this, [this]() {
        int64_t current_msec = Utils::get_current_msec();
        last_connect_time = current_msec;
        link->connect(url);
        delegate.seed_accessor_on_change_status(*this, get_status());
      }, last_connect_time + interval - current_msec);
  }
  
  delegate.seed_accessor_on_change_status(*this, get_status());
}

/**
 * Disconnect from the server.
 */
void SeedAccessor::disconnect() {
  if (get_status() != LinkStatus::OFFLINE) {
    link.reset();

    delegate.seed_accessor_on_change_status(*this, get_status());
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
  if (get_status() == LinkStatus::ONLINE &&
      (hint & SeedHint::ONLYONE) != 0) {
    return true;

  } else {
    return false;
  }
}

void SeedAccessor::relay_packet(std::unique_ptr<const Packet> packet) {
  if (link) {
    logd("Relay to seed. %s", Utils::dump_packet(*packet).c_str());

    picojson::object json;
    json.insert(std::make_pair("dst_nid", packet->dst_nid.to_json()));
    json.insert(std::make_pair("src_nid", packet->src_nid.to_json()));
    json.insert(std::make_pair("id", Convert::int2json(packet->id)));
    json.insert(std::make_pair("mode", picojson::value(static_cast<double>(packet->mode))));
    json.insert(std::make_pair("channel", picojson::value(static_cast<double>(packet->channel))));
    json.insert(std::make_pair("command_id", picojson::value(static_cast<double>(packet->command_id))));
    json.insert(std::make_pair("content", picojson::value(picojson::value(packet->content).serialize())));

    link->send(picojson::value(json).serialize());

  } else {
    logd("Reject relaying packet to seed. %s", Utils::dump_packet(*packet).c_str());
  }
}

void SeedAccessor::seed_link_on_connect(SeedLinkBase& link) {
  send_auth(token);
}

void SeedAccessor::seed_link_on_disconnect(SeedLinkBase& l) {
  context.scheduler.add_timeout_task(this, [this, &l]() {
      if (link.get() == &l) {
        disconnect();
      }
    }, 0);
}

void SeedAccessor::seed_link_on_error(SeedLinkBase& l) {
  context.scheduler.add_timeout_task(this, [this, &l]() {
      if (link.get() == &l) {
        disconnect();
      }
    }, 0);
}

void SeedAccessor::seed_link_on_recv(SeedLinkBase& link, const std::string& data) {
  std::istringstream is(data);
  picojson::value v;
  std::string err = picojson::parse(v, is);
  if (!err.empty()) {
    /// @todo error
    assert(false);
  }

  picojson::object& json = v.get<picojson::object>();

  picojson::value content;
  std::string content_str = json.at("content").get<std::string>();
  if (content_str.size() != 0) {
    std::istringstream is(content_str);
    std::string err = picojson::parse(content, is);
    if (!err.empty()) {
      // TODO(llamerada.jp@gmail.com): Output warning log.
      assert(false);
    }
  }

  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(Packet {
    NodeID::from_json(json.at("dst_nid")),
    NodeID::from_json(json.at("src_nid")),
    Convert::json2int<uint32_t>(json.at("id")),
    static_cast<PacketMode::Type>(json.at("mode").get<double>()),
    static_cast<ModuleChannel::Type>(json.at("channel").get<double>()),
    static_cast<CommandID::Type>(json.at("command_id").get<double>()),
    content_str == "" ? picojson::object() : content.get<picojson::object>()
      });

  if (packet->src_nid == NodeID::SEED && packet->channel == ModuleChannel::SEED &&
      packet->id == 0) {
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
}

void SeedAccessor::recv_auth_success(const Packet& packet) {
  assert(auth_status == AuthStatus::NONE);
  auth_status = AuthStatus::SUCCESS;
  delegate.seed_accessor_on_recv_config(*this, packet.content.at("config").get<picojson::object>());
  delegate.seed_accessor_on_change_status(*this, get_status());
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
  hint = Convert::json2int<SeedHint::Type>(packet.content.at("hint"));

  if (hint != hint_old) {
    delegate.seed_accessor_on_change_status(*this, get_status());
  }
}

void SeedAccessor::recv_ping(const Packet& packet) {
  send_ping();
}

void SeedAccessor::recv_require_random(const Packet& packet) {
  delegate.seed_accessor_on_recv_require_random(*this);
}

void SeedAccessor::send_auth(const std::string& token) {
  picojson::object params;
  params.insert(std::make_pair("version", picojson::value(PROTOCOL_VERSION)));
  params.insert(std::make_pair("token", picojson::value(token)));
  params.insert(std::make_pair("hint", Convert::int2json(hint)));

  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(Packet {
    NodeID::SEED,
    context.my_nid,
    0,
    PacketMode::NONE,
    ModuleChannel::SEED,
    CommandID::Seed::AUTH,
    params
  });

  relay_packet(std::move(packet));
}

void SeedAccessor::send_ping() {
  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(Packet {
    NodeID::SEED,
    context.my_nid,
    0,
    PacketMode::NONE,
    ModuleChannel::SEED,
    CommandID::Seed::PING,
    picojson::object()
  });

  relay_packet(std::move(packet));
}
}  // namespace colonio
