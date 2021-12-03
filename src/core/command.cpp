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

#include "command.hpp"

#include <cassert>
#include <tuple>

#include "utils.hpp"

namespace colonio {

Command::Command(CommandID::Type id_, PacketMode::Type mode_) : id(id_), mode(mode_) {
}

/**
 * Simple destructor for vtable.
 */
Command::~Command() {
}

std::tuple<CommandID::Type, PacketMode::Type> Command::get_define() {
  return std::make_tuple(id, mode);
}

/**
 * It will be called when an error packet has arrived for the send packet.
 * @param packet Received error packet.
 */
void Command::on_error(const std::string& message) {
  colonio_throw_error(ErrorCode::UNDEFINED, message);
}

/**
 * It will be called when a failure reply has arrived for the send packet.
 * @param packet Received reply packet.
 */
void Command::on_failure(std::unique_ptr<const Packet> packet) {
  assert(false);
}
}  // namespace colonio
