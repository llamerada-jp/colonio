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
#pragma once

#ifdef __clang__
#  include <picojson.h>
#else
#  pragma GCC diagnostic push
#  pragma GCC diagnostic ignored "-Wmaybe-uninitialized"
#  include <picojson.h>
#  pragma GCC diagnostic pop
#endif

#include <deque>
#include <memory>
#include <string>

#include "colonio.pb.h"
#include "definition.hpp"
#include "seed_link.hpp"

namespace colonio {
class NodeID;
class Packet;
class SeedAccessor;

class SeedAccessorDelegate {
 public:
  virtual ~SeedAccessorDelegate();
  virtual void seed_accessor_on_change_state()                                    = 0;
  virtual void seed_accessor_on_error(const std::string& message)                 = 0;
  virtual void seed_accessor_on_recv_config(const picojson::object& config)       = 0;
  virtual void seed_accessor_on_recv_packet(std::unique_ptr<const Packet> packet) = 0;
  virtual void seed_accessor_on_require_random()                                  = 0;
};

namespace AuthStatus {
typedef int Type;
static const Type NONE    = 0;
static const Type SUCCESS = 1;
static const Type FAILURE = 2;
}  // namespace AuthStatus

/**
 * Use server as seed of peer to peer connection.
 */
class SeedAccessor {
 public:
  SeedAccessor(
      Logger& l, Scheduler& s, const NodeID& n, SeedAccessorDelegate& d, const std::string& u, const std::string& t,
      unsigned int timeout, bool v);
  virtual ~SeedAccessor();
  SeedAccessor(const SeedAccessor&) = delete;
  SeedAccessor& operator=(const SeedAccessor&) = delete;

  void disconnect();
  void enable_polling(bool on);
  void tell_online_state(bool flag);
  AuthStatus::Type get_auth_status() const;
  LinkState::Type get_link_state() const;
  bool is_only_one();
  bool last_request_had_error();
  void relay_packet(std::unique_ptr<const Packet> packet);

 private:
  Logger& logger;
  Scheduler& scheduler;
  const NodeID& local_nid;
  SeedAccessorDelegate& delegate;

  const std::string token;
  const unsigned int SESSION_TIMEOUT;

  /** Connection to the server */
  std::unique_ptr<SeedLink> link;
  // session id. it is empty if session disabled
  std::string session;
  // true if polling is enabled
  bool polling_flag;
  // true if waiting response for authenticate request
  bool running_auth;
  // describing state of P2P connection. true if the node having online node link at least one
  bool is_online;
  // true if waiting response for polling request
  bool running_poll;
  // true if last request had error, ex: network error, request returns 4xx, 5xx code
  bool has_error;
  // timestamp of last send packet without close request
  int64_t last_send_time;

  // waiting queue of packets to relay to seed
  std::deque<std::unique_ptr<const Packet>> waiting;

  AuthStatus::Type auth_status;
  // true if the only this node is connecting to to the seed
  bool only_one_flag;

  void trigger();
  void send_authenticate();
  void send_relay();
  void send_poll();
  void send_close();
  void decode_hint(SeedHint::Type hint);
  void decode_http_code(int code);
};
}  // namespace colonio
