/*
 * Copyright 2017-2020 Yuji Ito <llamerada.jp@gmail.com>
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

#include <deque>
#include <memory>

#include "api_gate.hpp"
#include "controller.hpp"

namespace colonio {
class APIGateWASM : public APIGateBase, public ControllerDelegate {
 public:
  APIGateWASM();

  void call_async(
      APIChannel::Type channel, const api::Call& call, std::function<void(const api::Reply&)> on_reply) override;
  std::unique_ptr<api::Reply> call_sync(APIChannel::Type channel, const api::Call& call) override;
  void init() override;
  void quit() override;
  void set_event_hook(APIChannel::Type channel, std::function<void(const api::Event&)> on_event) override;

  void call(uint32_t id);
  void invoke();

 private:
  Controller controller;
  Random random;

  std::map<uint32_t, std::function<void(const api::Reply&)>> map_reply;
  std::map<APIChannel::Type, std::function<void(const api::Event&)>> map_event;

  std::map<uint32_t, std::function<void()>> map_call;

  void controller_on_event(Controller& sm, std::unique_ptr<api::Event> event) override;
  void controller_on_reply(Controller& sm, std::unique_ptr<api::Reply> reply) override;
  void controller_on_require_invoke(Controller& sm, unsigned int msec) override;

  void logger_on_output(Logger& logger, const std::string& json) override;

  void call_event(std::unique_ptr<api::Event> event);
  void call_reply(std::unique_ptr<api::Reply> reply, std::function<void(const api::Reply&)> on_reply);
  void push_call(std::function<void()> func);
  void reply_failure(uint32_t id, ErrorCode code, const std::string& message);
};

typedef APIGateWASM APIGate;
}  // namespace colonio
