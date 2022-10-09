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
#include <sstream>

#include "utils.hpp"

namespace colonio {

Command::Command(PacketMode::Type mode_) : mode(mode_) {
}

Command::~Command() {
}

/**
 * It will be called when an error packet has arrived for the send packet.
 * @param packet Received error packet.
 */
void Command::on_error(ErrorCode code, const std::string& message) {
  std::stringstream ss;
  ss << "receive error packet: (" << static_cast<uint32_t>(code) << ") " << message;
  colonio_throw_error(ErrorCode::UNDEFINED, ss.str());
}

CommandWrapper::CommandWrapper(PacketMode::Type mode, std::function<void(const Packet& packet)>& on_res) :
    Command(mode), cb_on_response(on_res) {
}

CommandWrapper::CommandWrapper(
    PacketMode::Type mode, std::function<void(const Packet& packet)>& on_res,
    std::function<void(ErrorCode code, const std::string& message)>& on_err) :
    Command(mode), cb_on_response(on_res), cb_on_error(on_err) {
}

void CommandWrapper::on_response(const Packet& packet) {
  cb_on_response(packet);
}

void CommandWrapper::on_error(ErrorCode code, const std::string& message) {
  if (cb_on_error) {
    cb_on_error(code, message);
    return;
  }

  Command::on_error(code, message);
}
}  // namespace colonio