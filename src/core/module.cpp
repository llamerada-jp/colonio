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

#include "module_protocol.pb.h"

#include "context.hpp"
#include "convert.hpp"
#include "module.hpp"
#include "utils.hpp"

namespace colonio {
/**
 * Simple destructor for vtable.
 */
ModuleDelegate::~ModuleDelegate() {
}

/**
 * Constructor with a module that parent module of this instance.
 * @param module Moule type.
 */
Module::Module(Context& context_, ModuleDelegate& delegate_, ModuleChannel::Type channel_) :
    channel(channel_),
    context(context_),
    delegate(delegate_) {
  context.scheduler.add_interval_task(this, std::bind(&Module::on_persec, this), 1000);
}

Module::~Module() {
  context.scheduler.remove_task(this);
}

std::unique_ptr<const Packet> Module::copy_packet_for_reply(const Packet& src) {
  return std::make_unique<const Packet>(src);
}

void Module::module_on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status) {
  // for override.
}

void Module::module_on_persec(LinkStatus::Type seed_status, LinkStatus::Type node_status) {
  // @todo deplicate.
}

/**
 * Receive packet and call capable reply or error command.
 * If pacekt is another type, call delegate method for general process.
 */
void Module::on_recv_packet(std::unique_ptr<const Packet> packet) {
  assert(packet->channel == channel);

  if (packet->command_id == CommandID::SUCCESS) {
    std::unique_ptr<Command> command;
    {
      std::lock_guard<std::mutex> guard(mutex_containers);
      auto container = containers.find(packet->id);
      if (container != containers.end()) {
        command = std::move(container->second.command);
        containers.erase(container);
      }
    }
    if (command) {
      command->on_success(std::move(packet));
    }

  } else if (packet->command_id == CommandID::FAILURE) {
    std::unique_ptr<Command> command;
    {
      std::lock_guard<std::mutex> guard(mutex_containers);
      auto container = containers.find(packet->id);
      if (container != containers.end()) {
        command = std::move(container->second.command);
        containers.erase(container);
      }
    }
    if (command) {
      command->on_failure(std::move(packet));
    }

  } else if (packet->command_id == CommandID::ERROR) {
    std::unique_ptr<Command> command;
    {
      std::lock_guard<std::mutex> guard(mutex_containers);
      auto container = containers.find(packet->id);
      if (container != containers.end()) {
        command = std::move(container->second.command);
        containers.erase(container);
      }
    }
    if (command) {
      ModuleProtocol::Error content;
      packet->parse_content(&content);
      command->on_error(content.message());
    }

  } else {
    module_process_command(std::move(packet));
  }
}

void Module::reset() {
  std::lock_guard<std::mutex> guard(mutex_containers);
  containers.clear();
}

bool Module::cancel_packet(uint32_t id) {
  auto it = containers.find(id);

  if (it == containers.end()) {
    return false;

  } else {
    containers.erase(it);
    return true;
  }
}

void Module::relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet) {
  delegate.module_do_relay_packet(*this, dst_nid, std::move(packet));
}

/**
 * Send a pacet that having reply and/or error process.
 * @param command Packet definition, reply and/or error process.
 * @param dst_nid Destination node-id.
 * @param content Packet content.
 */
void Module::send_packet(std::unique_ptr<Command> command, const NodeID& dst_nid,
                         std::shared_ptr<const std::string> content) {
  uint32_t packet_id = Context::get_rnd_32();
  std::unique_ptr<const Packet> packet;
  {
    std::lock_guard<std::mutex> guard(mutex_containers);
    while (packet_id == PACKET_ID_NONE || containers.find(packet_id) != containers.end()) {
      packet_id = Context::get_rnd_32();
    }

    std::tuple<CommandID::Type, PacketMode::Type> t = command->get_define();
    CommandID::Type command_id = std::get<0>(t);
    PacketMode::Type mode = std::get<1>(t);

    assert(mode & PacketMode::ONE_WAY || dst_nid != NodeID::NEXT);

    packet = std::make_unique<const Packet>(Packet {
        dst_nid,
        context.my_nid,
        packet_id,
        content,
        mode, channel, command_id
    });

    containers.insert(
        std::make_pair(packet_id, Container({
              dst_nid,
              context.my_nid,
              packet_id,
              mode,
              channel,
              command_id,
              content,
              0, Utils::get_current_msec(), std::move(command) })));
  }
  delegate.module_do_send_packet(*this, std::move(packet));
}

/**
 * Send a simple one-way packet.
 * @param command Packet command.
 * @param is_explicit True if set explicit flag to packet.
 * @param pid Target process-id.
 * @param dst_nid Destination node-id.
 * @param content Packet content.
 */
void Module::send_packet(const NodeID& dst_nid, PacketMode::Type mode,
                         CommandID::Type command_id, std::shared_ptr<const std::string> content) {
  uint32_t packet_id = Context::get_rnd_32();
  {
    std::lock_guard<std::mutex> guard(mutex_containers);
    while (packet_id == PACKET_ID_NONE || containers.find(packet_id) != containers.end()) {
      packet_id = Context::get_rnd_32();
    }
  }

  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(Packet {
    dst_nid,
    context.my_nid,
    packet_id,
    content,
    static_cast<PacketMode::Type>(PacketMode::ONE_WAY | mode),
    channel,
    command_id
  });

  delegate.module_do_send_packet(*this, std::move(packet));
}


/**
 * Send a error packet for the received packet.
 * @param error_for Received packet.
 */
void Module::send_error(const Packet& reply_for, const std::string& message) {
  ModuleProtocol::Error content;
  content.set_message(message);
  std::shared_ptr<const std::string> content_bin = serialize_pb(content);

  PacketMode::Type packet_mode = PacketMode::REPLY | PacketMode::EXPLICIT | PacketMode::ONE_WAY;
  if (reply_for.mode & PacketMode::RELAY_SEED) {
    packet_mode |= PacketMode::RELAY_SEED;
  }

  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(Packet {
    reply_for.src_nid,
    context.my_nid,
    reply_for.id,
    content_bin,
    packet_mode,
    reply_for.channel,
    CommandID::ERROR
  });

  delegate.module_do_send_packet(*this, std::move(packet));
}

/**
 * Send a failure reply packet for the received packet.
 * @param reply_for Received packet.
 * @param content Packet content.
 */
void Module::send_failure(const Packet& reply_for, std::shared_ptr<const std::string> content) {
  PacketMode::Type packet_mode = PacketMode::REPLY | PacketMode::EXPLICIT | PacketMode::ONE_WAY;
  if (reply_for.mode & PacketMode::RELAY_SEED) {
    packet_mode |= PacketMode::RELAY_SEED;
  }

  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(Packet {
    reply_for.src_nid,
    context.my_nid,
    reply_for.id,
    content,
    packet_mode,
    reply_for.channel,
    CommandID::FAILURE
  });

  delegate.module_do_send_packet(*this, std::move(packet));
}

/**
 * Send a success reply packet for the received packet.
 * @param reply_for Received packet.
 * @param content Packet content.
 */
void Module::send_success(const Packet& reply_for, std::shared_ptr<const std::string> content) {
  PacketMode::Type packet_mode = PacketMode::REPLY | PacketMode::EXPLICIT | PacketMode::ONE_WAY;
  if (reply_for.mode & PacketMode::RELAY_SEED) {
    packet_mode |= PacketMode::RELAY_SEED;
  }

  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(Packet {
    reply_for.src_nid,
    context.my_nid,
    reply_for.id,
    content,
    packet_mode,
    reply_for.channel,
    CommandID::SUCCESS
  });

  delegate.module_do_send_packet(*this, std::move(packet));
}

void Module::on_persec() {
  std::set<std::unique_ptr<Command>> on_errors;
  std::set<std::unique_ptr<Packet>> retry_packets;

  {
    std::lock_guard<std::mutex> guard(mutex_containers);
    auto it = containers.begin();

    while (it != containers.end()) {
      Container& container = it->second;
      if (Utils::get_current_msec() - container.send_time > PACKET_RETRY_INTERVAL) {
        if (container.retry_count > PACKET_RETRY_COUNT_MAX) {
          // error
          logd("Command timeout. (id=%s)", Convert::int2str(container.packet_id).c_str());
          on_errors.insert(std::move(container.command));
          it = containers.erase(it);
          continue;

        } else {
          if ((container.mode & PacketMode::NO_RETRY) == PacketMode::NONE) {
            // retry
            retry_packets.insert(std::make_unique<Packet>(Packet {
                  container.dst_nid,
                  container.src_nid,
                  container.packet_id,
                  container.content,
                  container.mode,
                  container.channel,
                  container.command_id
            }));
          }
          
          container.retry_count++;
          container.send_time = Utils::get_current_msec();
        }
      }
      it++;
    }
  }

  for (auto& it : on_errors) {
    it->on_error("timeout");
  }

  for (auto& it : retry_packets) {
    delegate.module_do_send_packet(*this, std::make_unique<const Packet>(*it));
  }
}
}  // namespace colonio
