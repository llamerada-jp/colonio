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

#include <functional>
#include <map>
#include <memory>

#include "colonio.pb.h"
#include "colonio/colonio.hpp"
#include "command.hpp"
#include "definition.hpp"
#include "node_id.hpp"
#include "packet.hpp"

namespace colonio {
class Logger;
class Random;
class Scheduler;

class CommandManagerDelegate {
 public:
  virtual ~CommandManagerDelegate();
  virtual void command_manager_do_send_packet(std::unique_ptr<const Packet> packet)                         = 0;
  virtual void command_manager_do_relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet) = 0;
};

class CommandManager {
 public:
  CommandManager(Logger& l, Random& r, Scheduler& s, const NodeID& nid, CommandManagerDelegate& d);
  virtual ~CommandManager();

  bool cancel_packet(const Packet& packet);
  void chunk_out(proto::PacketContent::ContentCase content_case);
  void receive_packet(std::unique_ptr<const Packet> packet);
  void relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet);
  void send_error(const Packet& res_for, ErrorCode error_code, const std::string& message);
  void send_packet(
      const NodeID& dst_nid, PacketMode::Type mode, std::unique_ptr<const proto::PacketContent> content,
      std::function<void(const Packet&)>& on_response,
      std::function<void(ErrorCode, const std::string& message)>& on_error);
  void send_packet(const NodeID& dst_nid, std::shared_ptr<Packet::Content> content, std::shared_ptr<Command> command);
  void send_packet(
      const NodeID& dst_nid, std::unique_ptr<proto::PacketContent> content, std::shared_ptr<Command> command);
  void send_packet_one_way(
      const NodeID& dst_nid, PacketMode::Type mode, std::unique_ptr<const proto::PacketContent> content);
  void send_response(const Packet& res_for, std::unique_ptr<const proto::PacketContent> content);
  void send_response(
      const NodeID& dst_nid, uint32_t id, bool relay_seed, std::unique_ptr<const proto::PacketContent> content);
  void set_handler(proto::PacketContent::ContentCase content_case, std::function<void(const Packet&)> handler);

 private:
  struct SentRecord {
    NodeID dst_nid;
    // NodeID src_nid;
    uint32_t packet_id;
    std::shared_ptr<Packet::Content> content;
    std::shared_ptr<Command> command;
    proto::PacketContent::ContentCase content_case;
    uint32_t retry_count;
    int64_t send_time;
  };

  Logger& logger;
  Random& random;
  Scheduler& scheduler;
  const NodeID& local_nid;

  CommandManagerDelegate& delegate;

  std::map<proto::PacketContent::ContentCase, std::function<void(const Packet&)>> handlers;
  std::map<uint32_t, SentRecord> records;

  void on_each_sec();
};
}  // namespace colonio
