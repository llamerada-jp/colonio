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

#include "command.hpp"
#include "module_base.hpp"

namespace colonio {
class Colonio;
class Error;
class Value;

class ColonioModule : public ModuleBase {
 public:
  ColonioModule(ModuleParam& param);
  virtual ~ColonioModule();

  void send(
      const std::string& dst_nid, const Value& value, uint32_t opt, std::function<void()>&& on_success,
      std::function<void(const Error&)>&& on_failure);
  void on(std::function<void(const Value&)>&& receiver_);
  void off();

 private:
  class CommandSend : public Command {
   public:
    ColonioModule& parent;
    std::function<void()> cb_success;
    std::function<void(const Error&)> cb_failure;

    CommandSend(PacketMode::Type mode, ColonioModule& parent_);

    void on_error(const std::string& message) override;
    void on_success(std::unique_ptr<const Packet> packet) override;
  };

  std::function<void(const Value&)> receiver;

  void module_process_command(std::unique_ptr<const Packet> packet) override;

  void recv_packet_send_packet(std::unique_ptr<const Packet> packet);
};
}  // namespace colonio
