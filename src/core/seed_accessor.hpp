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

#include <map>
#include <memory>
#include <queue>
#include <string>
#include <utility>
#include <vector>

#include "definition.hpp"
#include "seed_link.hpp"

namespace colonio {
struct ModuleParam;
class NodeID;
class Packet;
class SeedAccessor;

class SeedAccessorDelegate {
 public:
  virtual ~SeedAccessorDelegate();
  virtual void seed_accessor_on_change_state(SeedAccessor& sa)                                      = 0;
  virtual void seed_accessor_on_recv_config(SeedAccessor& sa, const picojson::object& config)       = 0;
  virtual void seed_accessor_on_recv_packet(SeedAccessor& sa, std::unique_ptr<const Packet> packet) = 0;
  virtual void seed_accessor_on_recv_require_random(SeedAccessor& sa)                               = 0;
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
class SeedAccessor : public SeedLinkDelegate {
 public:
  SeedAccessor(ModuleParam& param, SeedAccessorDelegate& delegate_, const std::string& url_, const std::string& token_);
  virtual ~SeedAccessor();
  SeedAccessor(const SeedAccessor&) = delete;
  SeedAccessor& operator=(const SeedAccessor&) = delete;

  void connect(unsigned int interval = SEED_CONNECT_INTERVAL);
  void disconnect();
  AuthStatus::Type get_auth_status() const;
  LinkState::Type get_link_state() const;
  bool is_only_one();
  void relay_packet(std::unique_ptr<const Packet> packet);

 private:
  Logger& logger;
  Scheduler& scheduler;
  const NodeID& local_nid;
  SeedAccessorDelegate& delegate;

  /** Server URL. */
  const std::string url;
  const std::string token;
  /** Connection to the server */
  std::unique_ptr<SeedLink> link;
  /** Last time of tried to connect to the server. */
  int64_t last_connect_time;

  AuthStatus::Type auth_status;
  SeedHint::Type hint;

  void seed_link_on_connect(SeedLink& link) override;
  void seed_link_on_disconnect(SeedLink& link) override;
  void seed_link_on_error(SeedLink& link) override;
  void seed_link_on_recv(SeedLink& link, const std::string& data) override;

  void recv_auth_success(const Packet& packet);
  void recv_auth_failure(const Packet& packet);
  void recv_auth_error(const Packet& packet);
  void recv_hint(const Packet& packet);
  void recv_ping(const Packet& packet);
  void recv_require_random(const Packet& packet);
  void send_auth(const std::string& token);
  void send_ping();
};
}  // namespace colonio
