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
#include "logger.hpp"
#include "packet.hpp"
#include "scheduler.hpp"
#include "utils.hpp"
#include "value_impl.hpp"

namespace colonio {

ColonioModule::CommandCall::CommandCall(PacketMode::Type mode, ColonioModule& parent_) :
    Command(CommandID::Colonio::CALL, mode), parent(parent_) {
}

void ColonioModule::CommandCall::on_error(ErrorCode code, const std::string& message) {
  Error e = colonio_error(code, message);
  parent.scheduler.add_user_task(&parent, [cb = cb_failure, e] {
    cb(e);
  });
}

void ColonioModule::CommandCall::on_success(std::unique_ptr<const Packet> packet) {
  ColonioModuleProtocol::CallSuccess content;
  packet->parse_content(&content);
  Value result = ValueImpl::from_pb(content.result());
  std::shared_ptr<const Packet> p(packet.release());
  parent.scheduler.add_user_task(&parent, [result, cb = cb_success] {
    cb(result);
  });
}

ColonioModule::ColonioModule(ModuleParam& param) : ModuleBase(param, Channel::COLONIO) {
}

ColonioModule::~ColonioModule() {
  scheduler.remove_task(this);
}

void ColonioModule::call_by_nid(
    const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt,
    std::function<void(const Value&)>&& on_success, std::function<void(const Error&)>&& on_failure) {
  scheduler.add_controller_task(this, [=] {
    ColonioModuleProtocol::Call param;
    param.set_opt(opt);
    param.set_name(name);
    ValueImpl::to_pb(param.mutable_value(), value);
    std::shared_ptr<const std::string> param_bin = serialize_pb(param);

    PacketMode::Type mode = 0;
    if ((opt & Colonio::CALL_ACCEPT_NEARBY) == 0) {
      mode |= PacketMode::EXPLICIT;
    }

    if ((opt & Colonio::CALL_IGNORE_REPLY) != 0) {
      send_packet(NodeID::from_str(dst_nid), mode, CommandID::Colonio::CALL, param_bin);
      scheduler.add_user_task(this, [on_success] {
        on_success(Value());
      });

    } else {
      std::unique_ptr<CommandCall> command = std::make_unique<CommandCall>(mode, *this);
      command->name                        = name;
      command->cb_success                  = on_success;
      command->cb_failure                  = on_failure;

      send_packet(std::move(command), NodeID::from_str(dst_nid), param_bin);
    }
  });
}

void ColonioModule::on_call(const std::string& name, std::function<Value(const Colonio::CallParameter&)>&& func) {
  on_call_funcs[name] = func;
}

void ColonioModule::off_call(const std::string& name) {
  on_call_funcs.erase(name);
}

void ColonioModule::module_process_command(std::unique_ptr<const Packet> packet) {
  switch (packet->command_id) {
    case CommandID::Colonio::CALL:
      recv_packet_call(std::move(packet));
      break;

    default:
      assert(false);
  }
}

void ColonioModule::recv_packet_call(std::unique_ptr<const Packet> packet) {
  ColonioModuleProtocol::Call content;
  packet->parse_content(&content);
  std::shared_ptr<Colonio::CallParameter> parameter(
      new Colonio::CallParameter({content.name(), ValueImpl::from_pb(content.value()), content.opt()}));

  auto it_func = on_call_funcs.find(parameter->name);
  if (it_func != on_call_funcs.end()) {
    auto& func = it_func->second;
    std::shared_ptr<const Packet> p(packet.release());
    scheduler.add_user_task(this, [this, p, parameter, func] {
      Value result;
      bool flg_err = false;
      try {
        result = func(*parameter);

      } catch (const Error& e) {
        flg_err = true;
        log_warn("an error occurred in user function")
            .map_u32("code", static_cast<uint32_t>(e.code))
            .map("message", e.message);
        send_error(*p, e.code, e.message);

      } catch (const std::exception& e) {
        flg_err = true;
        log_warn("an error occurred in user function").map("message", std::string(e.what()));
        send_error(*p, ErrorCode::RPC_UNDEFINED_ERROR, e.what());
      }

      if (flg_err || (parameter->options & Colonio::CALL_IGNORE_REPLY) != 0) {
        return;
      }

      scheduler.add_controller_task(this, [this, p, result] {
        ColonioModuleProtocol::CallSuccess param;
        ValueImpl::to_pb(param.mutable_result(), result);
        send_success(*p, serialize_pb(param));
      });
    });

  } else if ((parameter->options & Colonio::CALL_IGNORE_REPLY) == 0) {
    log_debug("receiver doesn't set").map("name", parameter->name);
    send_error(*packet, ErrorCode::RPC_UNDEFINED_ERROR, "receiver doesn't set");
  }
}
}  //  namespace colonio
