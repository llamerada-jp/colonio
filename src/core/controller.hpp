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

#include "api_bundler.hpp"
#include "colonio_impl.hpp"
#include "context.hpp"
#include "logger.hpp"
#include "scheduler.hpp"

namespace colonio {
class Controller;

class ControllerDelegate {
 public:
  virtual ~ControllerDelegate();
  virtual void controller_on_event(Controller& sm, std::unique_ptr<api::Event> event) = 0;
  virtual void controller_on_reply(Controller& sm, std::unique_ptr<api::Reply> reply) = 0;
  virtual void controller_on_require_invoke(Controller& sm, unsigned int msec)        = 0;
};

class Controller : public LoggerDelegate, public SchedulerDelegate, public APIDelegate {
 public:
  Logger logger;

  Controller(ControllerDelegate& delegate_);
  virtual ~Controller();

  void call(const api::Call& call);
  unsigned int invoke();

 private:
  ControllerDelegate& delegate;
  Scheduler scheduler;
  Context context;
  APIBundler bundler;
  std::shared_ptr<ColonioImpl> colonio_impl;

  void api_send_event(APIBase& api_base, std::unique_ptr<api::Event> event) override;
  void api_send_reply(APIBase& api_base, std::unique_ptr<api::Reply> reply) override;

  void logger_on_output(Logger& logger, LogLevel level, const std::string& message) override;

  void scheduler_on_require_invoke(Scheduler& sched, unsigned int msec) override;
};
}  // namespace colonio
