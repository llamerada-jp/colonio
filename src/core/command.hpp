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
#include <string>
#include <tuple>

#include "definition.hpp"
#include "packet.hpp"

namespace colonio {

class Command {
 public:
  virtual ~Command();

  std::tuple<CommandID::Type, PacketMode::Type> get_define();

  virtual void on_error(ErrorCode code, const std::string& message);
  virtual void on_failure(std::unique_ptr<const Packet> packet);
  virtual void on_success(std::unique_ptr<const Packet> packet) = 0;

 protected:
  const CommandID::Type id;
  const PacketMode::Type mode;

  Command(CommandID::Type id_, PacketMode::Type mode_);
};
}  // namespace colonio
