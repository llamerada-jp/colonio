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

#include "colonio_module.hpp"

#include "colonio/colonio.hpp"
#include "colonio_module_protocol.pb.h"
#include "scheduler.hpp"
#include "utils.hpp"
#include "value_impl.hpp"

namespace colonio {

ColonioModule::CommandSend::CommandSend(PacketMode::Type mode, ColonioModule& parent_) :
    Command(CommandID::Colonio::SEND_PACKET, mode), parent(parent_) {
}

void ColonioModule::CommandSend::on_error(const std::string& message) {
  // TODO:implement error
  Error e = colonio_error(ErrorCode::UNDEFINED, message);
  parent.scheduler.add_user_task(&parent, [cb = cb_failure, e] {
    cb(e);
  });
}

void ColonioModule::CommandSend::on_success(std::unique_ptr<const Packet> packet) {
  parent.scheduler.add_user_task(&parent, [cb = cb_success] {
    cb();
  });
}

ColonioModule::ColonioModule(ModuleParam& param) : ModuleBase(param, Channel::COLONIO) {
}

ColonioModule::~ColonioModule() {
  scheduler.remove_task(this);
}

void ColonioModule::send(
    const std::string& dst_nid, const Value& value, uint32_t opt, std::function<void()>&& on_success,
    std::function<void(const Error&)>&& on_failure) {
  scheduler.add_controller_task(this, [=] {
    ColonioModuleProtocol::SendPacket param;
    ValueImpl::to_pb(param.mutable_value(), value);
    std::shared_ptr<const std::string> param_bin = serialize_pb(param);

    PacketMode::Type mode = 0;
    if ((opt & Colonio::SEND_ACCEPT_NEARBY) == 0) {
      mode |= PacketMode::EXPLICIT;
    }

    if ((opt & Colonio::CONFIRM_RECEIVER_RESULT) != 0) {
      std::unique_ptr<CommandSend> command = std::make_unique<CommandSend>(mode, *this);
      command->cb_success                  = on_success;
      command->cb_failure                  = on_failure;

      send_packet(std::move(command), NodeID::from_str(dst_nid), param_bin);

    } else {
      send_packet(NodeID::from_str(dst_nid), mode, CommandID::Colonio::SEND_PACKET, param_bin);
      scheduler.add_user_task(this, [on_success] {
        on_success();
      });
    }
  });
}

void ColonioModule::on(std::function<void(const Value&)>&& receiver_) {
  receiver = receiver_;
}

void ColonioModule::off() {
  receiver = nullptr;
}

void ColonioModule::module_process_command(std::unique_ptr<const Packet> packet) {
  switch (packet->command_id) {
    case CommandID::Colonio::SEND_PACKET:
      recv_packet_send_packet(std::move(packet));
      break;

    default:
      assert(false);
  }
}

void ColonioModule::recv_packet_send_packet(std::unique_ptr<const Packet> packet) {
  ColonioModuleProtocol::SendPacket content;
  packet->parse_content(&content);
  Value v = ValueImpl::from_pb(content.value());

  send_success(*packet, std::shared_ptr<std::string>());

  if (receiver) {
    scheduler.add_user_task(this, [this, v] {
      receiver(v);
    });
  }
}
}  //  namespace colonio
