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

#include <picojson.h>

#include <cassert>
#include <istream>
#include <map>
#include <memory>
#include <sstream>
#include <string>
#include <vector>

#include "colonio.pb.h"
#include "logger.hpp"
#include "packet.hpp"
#include "scheduler.hpp"
#include "utils.hpp"

namespace colonio {
SeedAccessorDelegate::~SeedAccessorDelegate() {
}

SeedAccessor::SeedAccessor(
    Logger& l, Scheduler& s, const NodeID& n, SeedAccessorDelegate& d, const std::string& u, const std::string& t,
    unsigned int timeout, bool v) :
    logger(l),
    scheduler(s),
    local_nid(n),
    delegate(d),
    token(t),
    SESSION_TIMEOUT(timeout),
    polling_flag(false),
    running_auth(false),
    is_online(false),
    running_poll(false),
    has_error(false),
    last_send_time(0),
    auth_status(AuthStatus::NONE),
    only_one_flag(false) {
  SeedLinkParam link_param(logger, u, v);
  link.reset(SeedLink::new_instance(link_param));

  scheduler.repeat_task(
      this,
      [&]() {
        trigger();
      },
      1000);
}

/**
 */
SeedAccessor::~SeedAccessor() {
  link.reset();
  scheduler.remove(this);
}

void SeedAccessor::disconnect() {
  polling_flag = false;
  waiting.clear();

  if (get_link_state() == LinkState::OFFLINE) {
    return;
  }

  send_close();
}

void SeedAccessor::enable_polling(bool on) {
  if (polling_flag == on) {
    return;
  }

  polling_flag = on;
  trigger();
}

void SeedAccessor::tell_online_state(bool flag) {
  is_online = flag;
}

AuthStatus::Type SeedAccessor::get_auth_status() const {
  return auth_status;
}

LinkState::Type SeedAccessor::get_link_state() const {
  if (!session.empty()) {
    return LinkState::ONLINE;
  }

  if (running_auth) {
    return LinkState::CONNECTING;
  }

  return LinkState::OFFLINE;
}

bool SeedAccessor::is_only_one() {
  return only_one_flag;
}

bool SeedAccessor::last_request_had_error() {
  return has_error;
}

void SeedAccessor::relay_packet(std::unique_ptr<const Packet> packet) {
  waiting.push_back(std::move(packet));
  trigger();
}

void SeedAccessor::trigger() {
  // wait to finish authentication
  if (running_auth) {
    return;
  }

  // disconnect when polling is off and waiting queue is empty
  if (!polling_flag && waiting.empty() && last_send_time != 0) {
    if (last_send_time + SESSION_TIMEOUT < Utils::get_current_msec() && !session.empty()) {
      disconnect();
    }
    return;
  }

  // try to authenticate to get session
  if (session.empty()) {
    send_authenticate();
    last_send_time = Utils::get_current_msec();
    return;
  }

  if (polling_flag && !running_poll) {
    send_poll();
    last_send_time = Utils::get_current_msec();
  }

  if (!waiting.empty()) {
    send_relay();
    last_send_time = Utils::get_current_msec();
  }
}

void SeedAccessor::send_authenticate() {
  if (running_auth || auth_status == AuthStatus::FAILURE) {
    return;
  }

  session      = "";
  running_auth = true;
  auth_status  = AuthStatus::NONE;

  proto::SeedAuthenticate p;
  p.set_version(PROTOCOL_VERSION);
  local_nid.to_pb(p.mutable_nid());
  if (!token.empty()) {
    p.set_token(token);
  }

  std::string bin;
  p.SerializeToString(&bin);
  link->post("/authenticate", bin, [&](int code, const std::string& res_bin) {
    scheduler.add_task(this, [&, code, res_bin]() {
      running_auth = false;

      if (code != 200) {
        if (code == 401) {
          auth_status = AuthStatus::FAILURE;
          delegate.seed_accessor_on_error("authenticate failed, should check access token");

        } else {
          decode_http_code(code);
        }
        return;
      }

      proto::SeedAuthenticateResponse res;
      if (!res.ParseFromString(res_bin)) {
        log_warn("failed to parse an authenticate response packet from seed");
        return;
      }

      std::istringstream is(res.config());
      picojson::value v;
      std::string err = picojson::parse(v, is);
      if (!err.empty()) {
        log_warn("failed to parse configuration data from seed");
        return;
      }

      auth_status = AuthStatus::SUCCESS;
      session     = res.session_id();
      decode_hint(res.hint());

      delegate.seed_accessor_on_recv_config(v.get<picojson::object>());
      delegate.seed_accessor_on_change_state();

      trigger();
    });
  });

  delegate.seed_accessor_on_change_state();
}

void SeedAccessor::send_relay() {
  proto::SeedRelay p;
  p.set_session_id(session);
  for (auto& packet : waiting) {
    auto packet_pb = p.add_packets();
    packet->dst_nid.to_pb(packet_pb->mutable_dst_nid());
    packet->src_nid.to_pb(packet_pb->mutable_src_nid());
    packet_pb->set_hop_count(packet->hop_count);
    packet_pb->set_id(packet->id);
    packet_pb->set_mode(packet->mode);
    *(packet_pb->mutable_content()) = packet->content->as_proto();
  }
  waiting.clear();

  std::string bin;
  p.SerializeToString(&bin);
  link->post("/relay", bin, [&](int code, const std::string& res_bin) {
    scheduler.add_task(this, [&, code, res_bin]() {
      if (code != 200) {
        decode_http_code(code);
        return;
      }

      proto::SeedRelayResponse res;
      if (!res.ParseFromString(res_bin)) {
        log_warn("failed to parse a relay response packet from seed");
        return;
      }

      decode_hint(res.hint());

      trigger();
    });
  });
}

void SeedAccessor::send_poll() {
  assert(polling_flag);
  assert(!running_poll);

  running_poll = true;
  proto::SeedPoll p;
  p.set_session_id(session);
  p.set_online(is_online);

  std::string bin;
  p.SerializeToString(&bin);
  link->post("/poll", bin, [&](int code, const std::string& res_bin) {
    scheduler.add_task(this, [&, code, res_bin]() {
      running_poll = false;

      // drop received packet when disabling polling
      if (!polling_flag) {
        return;
      }

      if (code != 200) {
        decode_http_code(code);
        return;
      }

      proto::SeedPollResponse res;
      if (!res.ParseFromString(res_bin)) {
        log_warn("failed to parse a poll response packet from seed");
        return;
      }

      decode_hint(res.hint());
      session = res.session_id();

      for (int i = 0; i < res.packets_size(); i++) {
        auto& packet_pb                = res.packets(i);
        std::unique_ptr<Packet> packet = std::make_unique<Packet>(
            NodeID::from_pb(packet_pb.dst_nid()), NodeID::from_pb(packet_pb.src_nid()), packet_pb.id(),
            packet_pb.hop_count(),
            std::make_shared<Packet::Content>(std::make_unique<const proto::PacketContent>(packet_pb.content())),
            packet_pb.mode());
        delegate.seed_accessor_on_recv_packet(std::move(packet));
      }

      trigger();
    });
  });
}

void SeedAccessor::send_close() {
  proto::SeedClose p;
  p.set_session_id(session);

  std::string bin;
  p.SerializeToString(&bin);
  link->post("/close", bin, [&](int code, const std::string& res_bin) {
    scheduler.add_task(this, [&]() {
      trigger();
    });
  });

  session.clear();
  delegate.seed_accessor_on_change_state();
}

void SeedAccessor::decode_hint(SeedHint::Type hint) {
  only_one_flag = (hint & SeedHint::ONLY_ONE) != 0;

  if ((hint & SeedHint::REQUIRE_RANDOM) != 0) {
    delegate.seed_accessor_on_require_random();
  }
}

void SeedAccessor::decode_http_code(int code) {
  // network error, request returns 4xx, 5xx code
  if (code == 0 || (400 <= code && code < 600)) {
    has_error = true;
  } else {
    has_error = false;
  }

  switch (code) {
    case 0:  // network error
      break;

    case 200:  // OK
      return;

    case 400:  // bad request
      assert(false);
      return;

    case 401:  // unauthorized
      session = "";
      // session may closed, retry immediately
      scheduler.add_task(this, [&]() {
        trigger();
      });
      break;

    case 405:  // method not allowed
      // should use POST request
      delegate.seed_accessor_on_error("couldn't use POST request");
      return;

    case 412:  // precondition failed
      // should use HTTP/2 or HTTP/3
      delegate.seed_accessor_on_error("couldn't use HTTP/2 or HTTP/3");
      return;

    case 500:  // internal server error
      log_warn("internal server error occurred, will retry after a short time");
      break;

    default:  // unknown error
      log_warn("unknown error occurred").map_int("code", code);
      return;
  }
}
}  // namespace colonio
