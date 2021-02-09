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

#include "api_gate_wasm.hpp"

#include <emscripten.h>

#include <cassert>
#include <chrono>
#include <random>

#include "logger.hpp"
#include "utils.hpp"

extern "C" {
typedef unsigned long COLONIO_PTR_T;
typedef unsigned long COLONIO_CALL_ID_T;

extern void api_gate_release(COLONIO_PTR_T this_ptr);
extern void api_gate_require_call_after(COLONIO_PTR_T this_ptr, COLONIO_CALL_ID_T id);
extern void api_gate_require_invoke(COLONIO_PTR_T this_ptr, int msec);

EMSCRIPTEN_KEEPALIVE void api_gate_call(COLONIO_PTR_T this_ptr, COLONIO_CALL_ID_T id);
EMSCRIPTEN_KEEPALIVE void api_gate_invoke(COLONIO_PTR_T this_ptr);
}

void api_gate_call(COLONIO_PTR_T this_ptr, COLONIO_CALL_ID_T id) {
  colonio::APIGateWASM& THIS = *reinterpret_cast<colonio::APIGateWASM*>(this_ptr);

  THIS.call(id);
}

void api_gate_invoke(COLONIO_PTR_T this_ptr) {
  colonio::APIGateWASM& THIS = *reinterpret_cast<colonio::APIGateWASM*>(this_ptr);

  THIS.invoke();
}

namespace colonio {

APIGateWASM::APIGateWASM() : controller(*this) {
}

void APIGateWASM::call_async(
    APIChannel::Type channel, const api::Call& c, std::function<void(const api::Reply&)> on_reply) {
  uint32_t id;
  do {
    id = random.generate_u32();
  } while (id != 0 && map_reply.find(id) != map_reply.end());
  map_reply.insert(std::make_pair(id, on_reply));

  api::Call call = c;
  call.set_id(id);
  call.set_channel(channel);

  try {
    controller.call(call);

  } catch (const FatalException& ex) {
    logE(controller, "fatal exception").map("file", ex.file).map_int("line", ex.line).map("message", ex.message);
    reply_failure(id, ex.code, ex.message);
    // TODO stop
    return;

  } catch (const InternalException& ex) {
    logW(controller, "internal exception").map("file", ex.file).map_int("line", ex.line).map("message", ex.message);
    reply_failure(id, ex.code, ex.message);

  } catch (const std::exception& ex) {
    logE(controller, "exception").map("message", std::string(ex.what()));
    reply_failure(id, ErrorCode::UNDEFINED, ex.what());
    // TODO stop
    return;
  }
}

std::unique_ptr<api::Reply> APIGateWASM::call_sync(APIChannel::Type channel, const api::Call& call) {
  // call_sync is not used in wasm.
  assert(false);
  return std::unique_ptr<api::Reply>();
}

void APIGateWASM::init(bool explicit_event_thread, bool explicit_controller_thread) {
  assert(explicit_event_thread == false);
  assert(explicit_controller_thread == false);

  api_gate_require_invoke(reinterpret_cast<COLONIO_PTR_T>(this), 100);
}

void APIGateWASM::quit() {
  api_gate_release(reinterpret_cast<COLONIO_PTR_T>(this));
}

void APIGateWASM::set_event_hook(APIChannel::Type channel, std::function<void(const api::Event&)> on_event) {
  assert(map_event.find(channel) == map_event.end());
  map_event.insert(std::make_pair(channel, on_event));
}

void APIGateWASM::call(uint32_t id) {
  auto it = map_call.find(id);
  if (it != map_call.end()) {
    it->second();
    map_call.erase(id);
  }
}

void APIGateWASM::invoke() {
  unsigned int msec = 0;

  try {
    msec = controller.invoke();

  } catch (const FatalException& ex) {
    logE(controller, "fatal exception").map("file", ex.file).map_int("line", ex.line).map("message", ex.message);
    // TODO stop

  } catch (const InternalException& ex) {
    logW(controller, "internal exception").map("file", ex.file).map_int("line", ex.line).map("message", ex.message);

  } catch (const std::exception& ex) {
    logE(controller, "exception").map("message", std::string(ex.what()));
    // TODO stop
  }

  api_gate_require_invoke(reinterpret_cast<COLONIO_PTR_T>(this), msec);
}

void APIGateWASM::controller_on_event(Controller& sm, std::unique_ptr<api::Event> event) {
  call_event(std::move(event));
}

void APIGateWASM::controller_on_reply(Controller& sm, std::unique_ptr<api::Reply> reply) {
  auto it = map_reply.find(reply->id());
  if (it != map_reply.end()) {
    call_reply(std::move(reply), it->second);
    map_reply.erase(it);
  } else {
    logW(*this, "id for reply does not exist").map_u32("id", reply->id());
  }
}

void APIGateWASM::controller_on_require_invoke(Controller& sm, unsigned int msec) {
  api_gate_require_invoke(reinterpret_cast<COLONIO_PTR_T>(this), msec);
}

void APIGateWASM::logger_on_output(Logger& logger, const std::string& json) {
  std::unique_ptr<api::Event> event = std::make_unique<api::Event>();
  event->set_channel(APIChannel::COLONIO);
  api::colonio::LogEvent* log_event = event->mutable_colonio_log();
  log_event->set_json(json);

  call_event(std::move(event));
}

void APIGateWASM::call_event(std::unique_ptr<api::Event> event) {
  std::shared_ptr<api::Event> shared_ev = std::move(event);
  push_call([this, shared_ev]() {
    auto it = map_event.find(shared_ev->channel());
    if (it != map_event.end()) {
      it->second(*shared_ev);
    } else {
      logE(*this, "send event for channel not set").map_u32("channel", shared_ev->channel());
      assert(false);
    }
  });
}

void APIGateWASM::call_reply(std::unique_ptr<api::Reply> reply, std::function<void(const api::Reply&)> on_reply) {
  std::shared_ptr<api::Reply> shared_reply = std::move(reply);
  push_call([shared_reply, on_reply]() { on_reply(*shared_reply); });
}

void APIGateWASM::push_call(std::function<void()> func) {
  uint32_t id;
  do {
    id = random.generate_u32();
  } while (id != 0 && map_call.find(id) != map_call.end());
  map_call.insert(std::make_pair(id, func));
  api_gate_require_call_after(reinterpret_cast<COLONIO_PTR_T>(this), static_cast<COLONIO_CALL_ID_T>(id));
}

void APIGateWASM::reply_failure(uint32_t id, ErrorCode code, const std::string& message) {
  std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
  reply->set_id(id);

  api::Failure* param = reply->mutable_failure();
  param->set_code(static_cast<uint32_t>(code));
  param->set_message(message);

  auto it = map_reply.find(id);
  if (it != map_reply.end()) {
    call_reply(std::move(reply), it->second);
    map_reply.erase(it);
  } else {
    logW(*this, "id for reply does not exist").map_u32("id", reply->id());
  }
}
}  // namespace colonio
