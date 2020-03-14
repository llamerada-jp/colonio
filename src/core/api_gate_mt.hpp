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

#include <condition_variable>
#include <memory>
#include <deque>
#include <thread>

#include "api_gate.hpp"
#include "controller.hpp"

namespace colonio {
class APIGateMultiThread : public APIGateBase, public ControllerDelegate {
 public:
  APIGateMultiThread();

  std::unique_ptr<api::Reply> call_sync(APIChannel::Type channel, const api::Call& call) override;
  void init() override;
  void quit() override;
  void set_event_hook(APIChannel::Type channel, std::function<void(const api::Event&)> on_event) override;

 private:
  std::unique_ptr<std::thread> th_event;
  std::unique_ptr<std::thread> th_controller;
  bool flg_end;
  std::mutex mtx_end;

  Controller controller;

  std::deque<std::unique_ptr<api::Call>> que_call;
  std::map<uint32_t, std::unique_ptr<api::Reply>> map_reply;
  std::unique_ptr<std::deque<std::unique_ptr<api::Event>>> que_event;
  std::map<APIChannel::Type, std::function<void(const api::Event&)>> map_event;
  std::mutex mtx_call;
  std::mutex mtx_event;
  std::mutex mtx_reply;
  std::condition_variable cond_reply;
  std::condition_variable cond_event;
  std::condition_variable cond_controller;

  std::chrono::steady_clock::time_point tp;

  void controller_on_event(Controller& sm, std::unique_ptr<api::Event> event) override;
  void controller_on_reply(Controller& sm, std::unique_ptr<api::Reply> reply) override;
  void controller_on_require_invoke(Controller& sm, unsigned int msec) override;

  void logger_on_output(Logger& logger, LogLevel::Type level, const std::string& message) override;

  void loop_event();
  void loop_controller();
  bool has_end();
  void push_event(std::unique_ptr<api::Event> event);
  void reply_failure(uint32_t id, Exception::Code code, const std::string& message);
};

typedef APIGateMultiThread APIGate;
}  // namespace colonio
