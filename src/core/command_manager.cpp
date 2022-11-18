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

#include "command_manager.hpp"

#include <cassert>
#include <functional>
#include <memory>

#include "colonio.pb.h"
#include "command.hpp"
#include "logger.hpp"
#include "random.hpp"
#include "scheduler.hpp"

namespace colonio {

CommandManagerDelegate::~CommandManagerDelegate() {
}

CommandManager::CommandManager(Logger& l, Random& r, Scheduler& s, const NodeID& nid, CommandManagerDelegate& d) :
    logger(l), random(r), scheduler(s), local_nid(nid), delegate(d) {
  scheduler.repeat_task(this, std::bind(&CommandManager::on_each_sec, this), 1000);
}

CommandManager::~CommandManager() {
  scheduler.remove(this);
}

bool CommandManager::cancel_packet(const Packet& packet) {
  return records.erase(packet.id) > 0;
}

void CommandManager::chunk_out(proto::PacketContent::ContentCase content_case) {
  auto it = records.begin();
  while (it != records.end()) {
    if (it->second.content_case == content_case) {
      it = records.erase(it);
    } else {
      it++;
    }
  }
}

void CommandManager::receive_packet(std::unique_ptr<const Packet> packet) {
  assert(scheduler.is_controller_thread());

  if ((packet->mode & PacketMode::RESPONSE) != 0) {
    auto record = records.find(packet->id);
    if (record == records.end()) {
      log_warn("invalid packet id").map("packet", *packet);
      return;
    }
    std::shared_ptr<Command> cmd = record->second.command;
    records.erase(record);
    const proto::PacketContent& content = packet->content->as_proto();

    if (content.has_error()) {
      const proto::Error& err = content.error();
      cmd->on_error(static_cast<ErrorCode>(err.code()), err.message());
      return;
    }

    cmd->on_response(*packet);
    return;
  }

  proto::PacketContent::ContentCase content_case = packet->content->as_proto().Content_case();
  auto handler                                   = handlers.find(content_case);
  if (handler == handlers.end()) {
    log_warn("invalid packet order").map("packet", *packet);
    return;
  }
  handler->second(*packet);
}

void CommandManager::relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet) {
  delegate.command_manager_do_relay_packet(dst_nid, std::move(packet));
}

void CommandManager::send_error(const Packet& res_for, ErrorCode error_code, const std::string& message) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::Error* err                             = content->mutable_error();
  err->set_code(static_cast<uint32_t>(error_code));
  err->set_message(message);

  PacketMode::Type packet_mode = PacketMode::RESPONSE | PacketMode::EXPLICIT | PacketMode::ONE_WAY;
  if (res_for.mode & PacketMode::RELAY_SEED) {
    packet_mode |= PacketMode::RELAY_SEED;
  }

  std::unique_ptr<const Packet> packet =
      std::make_unique<const Packet>(res_for.src_nid, local_nid, res_for.id, std::move(content), packet_mode);

  delegate.command_manager_do_send_packet(std::move(packet));
}

void CommandManager::send_packet(
    const NodeID& dst_nid, PacketMode::Type mode, std::unique_ptr<const proto::PacketContent> content,
    std::function<void(const Packet&)>& on_response,
    std::function<void(ErrorCode, const std::string& message)>& on_error) {
  auto a = std::make_shared<Packet::Content>(std::move(content));
  auto b = std::make_shared<CommandWrapper>(mode, on_response, on_error);
  send_packet(dst_nid, a, b);
}

void CommandManager::send_packet(
    const NodeID& dst_nid, std::shared_ptr<Packet::Content> content, std::shared_ptr<Command> command) {
  assert(scheduler.is_controller_thread());

  uint32_t packet_id = random.generate_u32();
  std::unique_ptr<const Packet> packet;
  while (packet_id == PACKET_ID_NONE || records.find(packet_id) != records.end()) {
    packet_id = random.generate_u32();
  }

  PacketMode::Type mode = command->mode;
  assert(mode & PacketMode::ONE_WAY || dst_nid != NodeID::NEXT);

  packet = std::make_unique<const Packet>(dst_nid, local_nid, packet_id, 0, content, mode);

  records.insert(std::make_pair(
      packet_id, SentRecord({
                     dst_nid,
                     // local_nid,
                     packet_id,
                     content,
                     command,
                     content->as_proto().Content_case(),
                     0,
                     Utils::get_current_msec(),
                 })));

  delegate.command_manager_do_send_packet(std::move(packet));
}

void CommandManager::send_packet(
    const NodeID& dst_nid, std::unique_ptr<proto::PacketContent> content, std::shared_ptr<Command> command) {
  auto a = std::make_shared<Packet::Content>(std::move(content));
  send_packet(dst_nid, a, command);
}

void CommandManager::send_packet_one_way(
    const NodeID& dst_nid, PacketMode::Type mode, std::unique_ptr<const proto::PacketContent> content) {
  uint32_t packet_id = random.generate_u32();
  std::unique_ptr<const Packet> packet;
  while (packet_id == PACKET_ID_NONE || records.find(packet_id) != records.end()) {
    packet_id = random.generate_u32();
  }

  packet = std::make_unique<const Packet>(
      dst_nid, local_nid, packet_id, std::move(content), static_cast<PacketMode::Type>(PacketMode::ONE_WAY | mode));

  delegate.command_manager_do_send_packet(std::move(packet));
}

void CommandManager::send_response(const Packet& res_for, std::unique_ptr<const proto::PacketContent> content) {
  send_response(res_for.src_nid, res_for.id, (res_for.mode & PacketMode::RELAY_SEED) != 0, std::move(content));
}

void CommandManager::send_response(
    const NodeID& dst_nid, uint32_t id, bool relay_seed, std::unique_ptr<const proto::PacketContent> content) {
  PacketMode::Type mode = PacketMode::RESPONSE | PacketMode::EXPLICIT | PacketMode::ONE_WAY;
  if (relay_seed) {
    mode |= PacketMode::RELAY_SEED;
  }

  std::unique_ptr<const Packet> packet =
      std::make_unique<const Packet>(dst_nid, local_nid, id, std::move(content), mode);

  delegate.command_manager_do_send_packet(std::move(packet));
}

void CommandManager::set_handler(
    proto::PacketContent::ContentCase content_case, std::function<void(const Packet&)> handler) {
  assert(handlers.find(content_case) == handlers.end());
  handlers.insert(std::make_pair(content_case, handler));
}

void CommandManager::on_each_sec() {
  assert(scheduler.is_controller_thread());
  std::set<std::shared_ptr<Command>> on_errors;
  std::set<std::unique_ptr<Packet>> retry_packets;

  auto it = records.begin();

  while (it != records.end()) {
    SentRecord& record = it->second;
    if (Utils::get_current_msec() - record.send_time <= PACKET_RETRY_INTERVAL) {
      it++;
      continue;
    }

    // raise error if timeout
    if (record.retry_count > PACKET_RETRY_COUNT_MAX) {
      log_debug("command timeout").map_u32("id", record.packet_id);
      on_errors.insert(record.command);
      it = records.erase(it);
      continue;
    }

    // retry to send the packet
    if ((record.command->mode & PacketMode::NO_RETRY) == PacketMode::NONE) {
      retry_packets.insert(std::make_unique<Packet>(
          record.dst_nid, local_nid, record.packet_id, 0, record.content, record.command->mode));
    }

    // update retry count and timestamp
    record.retry_count++;
    record.send_time = Utils::get_current_msec();
    it++;
  }

  for (auto& it : on_errors) {
    it->on_error(ErrorCode::PACKET_TIMEOUT, "command timeout");
  }

  for (auto& it : retry_packets) {
    delegate.command_manager_do_send_packet(std::make_unique<const Packet>(*it));
  }
}
}  // namespace colonio
