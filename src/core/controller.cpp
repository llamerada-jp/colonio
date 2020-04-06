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

#include "controller.hpp"

namespace colonio {
ControllerDelegate::~ControllerDelegate() {
}

Controller::Controller(ControllerDelegate& delegate_) :
    delegate(delegate_), logger(*this), scheduler(*this), context(logger, scheduler) {
  colonio_impl = std::make_shared<ColonioImpl>(context, *this, bundler);
  bundler.registrate(colonio_impl);
#ifndef NDEBUG
  context.hook_on_debug_event([this](DebugEvent::Type event, const picojson::value& json) {
    std::unique_ptr<api::Event> ev = std::make_unique<api::Event>();
    ev->set_channel(APIChannel::COLONIO);
    api::colonio::DebugEvent* debug_event = ev->mutable_colonio_debug();
    debug_event->set_event(static_cast<uint32_t>(event));
    debug_event->set_json(json.serialize());

    delegate.controller_on_event(*this, std::move(ev));
  });
#endif
}

Controller::~Controller() {
}

void Controller::call(const api::Call& call) {
  bundler.call(call);
}

unsigned int Controller::invoke() {
  return context.scheduler.invoke();
}

void Controller::api_send_event(APIBase& api_base, std::unique_ptr<api::Event> event) {
  delegate.controller_on_event(*this, std::move(event));
}
void Controller::api_send_reply(APIBase& api_base, std::unique_ptr<api::Reply> reply) {
  delegate.controller_on_reply(*this, std::move(reply));
}

void Controller::logger_on_output(Logger& logger, LogLevel level, const std::string& message) {
  // Send log message as event.
  std::unique_ptr<api::Event> event = std::make_unique<api::Event>();
  event->set_channel(APIChannel::COLONIO);
  api::colonio::LogEvent* log_event = event->mutable_colonio_log();
  log_event->set_level(static_cast<uint32_t>(level));
  log_event->set_message(message);

  delegate.controller_on_event(*this, std::move(event));
}

void Controller::scheduler_on_require_invoke(Scheduler& sched, unsigned int msec) {
  delegate.controller_on_require_invoke(*this, msec);
}

}  // namespace colonio
