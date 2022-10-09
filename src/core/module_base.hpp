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

#include <memory>
#include <mutex>

#include "definition.hpp"
#include "node_id.hpp"

namespace colonio {
class Command;
class Logger;
class ModuleBase;
class ModuleDelegate;
class Packet;
class Random;
class Scheduler;

struct ModuleParam {
  ModuleDelegate& delegate;
  Logger& logger;
  Random& random;
  Scheduler& scheduler;
  const NodeID& local_nid;
};

class ModuleDelegate {
 public:
  virtual ~ModuleDelegate();
  virtual void module_do_send_packet(ModuleBase& module, std::unique_ptr<const Packet> packet) = 0;
  virtual void module_do_relay_packet(
      ModuleBase& module, const NodeID& dst_nid, std::unique_ptr<const Packet> packet) = 0;
};

class ModuleBase {
 public:
  const Channel::Type channel;

  ModuleBase(const ModuleBase&) = delete;
  virtual ~ModuleBase();
  ModuleBase& operator=(const ModuleBase&) = delete;

  static std::unique_ptr<const Packet> copy_packet_for_reply(const Packet& src);

  virtual void module_on_change_accessor_state(LinkState::Type seed_state, LinkState::Type node_state);

  void on_recv_packet(std::unique_ptr<const Packet> packet);
  void reset();

 protected:
  Logger& logger;
  Random& random;
  Scheduler& scheduler;
  const NodeID& local_nid;

  ModuleBase(ModuleParam& param, Channel::Type channel_);

  virtual void module_process_command(std::unique_ptr<const Packet> packet) = 0;

  bool cancel_packet(uint32_t id);
  void relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet);
  void send_packet(std::unique_ptr<Command> command, const NodeID& dst_nid, std::shared_ptr<const std::string> content);
  void send_packet(
      const NodeID& dst_nid, PacketMode::Type mode, CommandID::Type command_id,
      std::shared_ptr<const std::string> content);
  void send_error(const Packet& reply_for, ErrorCode error_code, const std::string& message);
  void send_failure(const Packet& reply_for, std::shared_ptr<const std::string> content);
  void send_success(const Packet& reply_for, std::shared_ptr<const std::string> content);

 private:
  struct Container {
    NodeID dst_nid;
    NodeID src_nid;
    uint32_t packet_id;
    PacketMode::Type mode;
    // Channel::Type channel;
    CommandID::Type command_id;
    std::shared_ptr<const std::string> content;

    uint32_t retry_count;
    int64_t send_time;
    std::unique_ptr<Command> command;
  };

  ModuleDelegate& delegate;

  std::map<uint32_t, Container> containers;

  void on_persec();
};
}  // namespace colonio
