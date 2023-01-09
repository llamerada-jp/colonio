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
#include <string>

#include "colonio/colonio.hpp"
#include "command.hpp"
#include "definition.hpp"

namespace colonio {
class CommandManager;
class Error;
class Logger;
class Scheduler;
class Value;

class Messaging {
 public:
  Messaging(Logger& l, CommandManager& c);
  virtual ~Messaging();

  void post(
      const std::string& dst_nid, const std::string& name, const Value& message, uint32_t opt,
      std::function<void(const Value&)>&& on_response, std::function<void(const Error&)>&& on_failure);
  void set_handler(
      const std::string& name,
      std::function<void(std::shared_ptr<const MessagingRequest>, std::shared_ptr<MessagingResponseWriter>)>&& handler);
  void unset_handler(const std::string& name);

 private:
  class CommandMessage : public Command {
   public:
    Logger& logger;
    Messaging& parent;
    std::string name;
    std::function<void(const Value&)> cb_response;
    std::function<void(const Error&)> cb_failure;

    CommandMessage(PacketMode::Type mode, Messaging& parent_);

    void on_response(const Packet& packet) override;
    void on_error(ErrorCode code, const std::string& message) override;
  };

  Logger& logger;
  CommandManager& command_manager;

  std::map<
      std::string,
      std::function<void(std::shared_ptr<const MessagingRequest>, std::shared_ptr<MessagingResponseWriter>)>>
      handlers;

  void recv_messaging(const Packet& packet);
};
}  // namespace colonio
