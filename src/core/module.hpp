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
#pragma once

#include <memory>

#include "command.hpp"
#include "definition.hpp"

namespace colonio {
class Context;
class Module;

class ModuleDelegate {
 public:
  virtual ~ModuleDelegate();
  virtual void module_do_send_packet(Module& module, std::unique_ptr<const Packet> packet) = 0;
  virtual void module_do_relay_packet(Module& module, const NodeID& dst_nid,
                                      std::unique_ptr<const Packet> packet) = 0;
};

class Module {
 public:
  const ModuleChannel::Type channel;

  virtual ~Module();

  static std::unique_ptr<const Packet> copy_packet_for_reply(const Packet& src);

  virtual void module_on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status);
  // @todo deplicate
  virtual void module_on_persec(LinkStatus::Type seed_status, LinkStatus::Type node_status);

  void on_recv_packet(std::unique_ptr<const Packet> packet);
  void reset();

 protected:
  Context& context;

  Module(Context& context_, ModuleDelegate& delegate_, ModuleChannel::Type channel_);

  virtual void module_process_command(std::unique_ptr<const Packet> packet) = 0;

  bool cancel_packet(uint32_t id);
  void relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet);
  void send_packet(std::unique_ptr<Command> command, const NodeID& dst_nid,
                   const picojson::object& content);
  void send_packet(const NodeID& dst_nid, PacketMode::Type mode,
                   CommandID::Type command_id, const picojson::object& content);
  void send_error(const Packet& reply_for, const std::string& message);
  void send_failure(const Packet& reply_for, const picojson::object& content);
  void send_success(const Packet& reply_for, const picojson::object& content);

 private:
  struct Container {
    NodeID dst_nid;
    NodeID src_nid;
    uint32_t packet_id;
    PacketMode::Type mode;
    ModuleChannel::Type channel;
    CommandID::Type command_id;
    picojson::object content;

    uint32_t retry_count;
    int64_t send_time;
    std::unique_ptr<Command> command;
  };

  ModuleDelegate& delegate;

  std::map<uint32_t, Container> containers;
  std::mutex mutex_containers;

  void on_persec();
};
}  // namespace colonio
