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

#include "colonio/colonio.hpp"
#include "command.hpp"
#include "module_base.hpp"

namespace colonio {
class ColonioModule : public ModuleBase {
 public:
  explicit ColonioModule(ModuleParam& param);
  virtual ~ColonioModule();

  void call_by_nid(
      const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt,
      std::function<void(const Value&)>&& on_success, std::function<void(const Error&)>&& on_failure);
  void on_call(const std::string& name, std::function<Value(const Colonio::CallParameter&)>&& func);
  void off_call(const std::string& name);

 private:
  class CommandCall : public Command {
   public:
    ColonioModule& parent;
    std::string name;
    std::function<void(const Value&)> cb_success;
    std::function<void(const Error&)> cb_failure;

    CommandCall(PacketMode::Type mode, ColonioModule& parent_);

    void on_error(ErrorCode code, const std::string& message) override;
    void on_success(std::unique_ptr<const Packet> packet) override;
  };

  std::map<std::string, std::function<Value(const Colonio::CallParameter&)>> on_call_funcs;

  void module_process_command(std::unique_ptr<const Packet> packet) override;

  void recv_packet_call(std::unique_ptr<const Packet> packet);
};
}  // namespace colonio
