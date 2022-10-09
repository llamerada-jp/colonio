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
#include <memory>
#include <string>

#include "definition.hpp"
#include "packet.hpp"

namespace colonio {

class Command {
 public:
  const PacketMode::Type mode;

  Command(PacketMode::Type mode_);
  virtual ~Command();

  virtual void on_response(const Packet& packet) = 0;
  virtual void on_error(ErrorCode code, const std::string& message);
};

class CommandWrapper : public Command {
 public:
  CommandWrapper(PacketMode::Type mode, std::function<void(const Packet& packet)>& on_res);
  CommandWrapper(
      PacketMode::Type mode, std::function<void(const Packet& packet)>& on_res,
      std::function<void(ErrorCode code, const std::string& message)>& on_err);

  void on_response(const Packet& packet) override;
  void on_error(ErrorCode code, const std::string& message) override;

 private:
  std::function<void(const Packet& packet)> cb_on_response;
  std::function<void(ErrorCode code, const std::string& message)> cb_on_error;
};
}  // namespace colonio