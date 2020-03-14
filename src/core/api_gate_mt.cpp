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

#include "api_gate_mt.hpp"

#include <cassert>
#include <chrono>
#include <random>

#include "logger.hpp"
#include "utils.hpp"

namespace colonio {

APIGateMultiThread::APIGateMultiThread() :
    APIGateBase(*this),
    controller(*this),
    que_event(std::make_unique<std::deque<std::unique_ptr<api::Event>>>()),
    tp(std::chrono::steady_clock::now()) {
}

std::unique_ptr<api::Reply> APIGateMultiThread::call_sync(APIChannel::Type channel, const api::Call& c) {
  std::unique_ptr<api::Call> call = std::make_unique<api::Call>(c);
  uint32_t id;

  {
    std::lock_guard<std::mutex> lock(mtx_reply);
    do {
      id = Utils::get_rnd_32();
    } while (id != 0 && map_reply.find(id) != map_reply.end());
    map_reply.insert(std::make_pair(id, std::unique_ptr<api::Reply>(nullptr)));
  }

  call->set_id(id);
  call->set_channel(channel);

  {
    std::lock_guard<std::mutex> lock(mtx_call);
    que_call.push_back(std::move(call));
    cond_controller.notify_all();
  }

  {
    std::unique_lock<std::mutex> lock(mtx_reply);
    cond_reply.wait(lock, [this, id]() { return (map_reply.at(id) || has_end()); });

    auto r = map_reply.find(id);
    if (r->second) {
      std::unique_ptr<api::Reply> reply = std::move(r->second);
      map_reply.erase(r);
      return std::move(reply);
    } else {
      // return null when disconnect.
      return nullptr;
    }
  }
}

void APIGateMultiThread::init() {
  assert(!th_event);
  assert(!th_controller);
  {
    std::lock_guard<std::mutex> lock(mtx_end);
    flg_end = false;
  }
  th_event      = std::make_unique<std::thread>(&APIGateMultiThread::loop_event, this);
  th_controller = std::make_unique<std::thread>(&APIGateMultiThread::loop_controller, this);
}

void APIGateMultiThread::quit() {
  {
    std::lock_guard<std::mutex> lock(mtx_end);
    flg_end = true;
  }
  cond_controller.notify_all();
  cond_reply.notify_all();
  cond_event.notify_all();

  if (th_event) {
    th_event->join();
  }
  if (th_controller) {
    th_controller->join();
  }
}

void APIGateMultiThread::set_event_hook(APIChannel::Type channel, std::function<void(const api::Event&)> on_event) {
  std::lock_guard<std::mutex> lock(mtx_event);
  assert(map_event.find(channel) == map_event.end());
  map_event.insert(std::make_pair(channel, on_event));
}

void APIGateMultiThread::controller_on_event(Controller& sm, std::unique_ptr<api::Event> event) {
  push_event(std::move(event));
}

void APIGateMultiThread::controller_on_reply(Controller& sm, std::unique_ptr<api::Reply> reply) {
  std::lock_guard<std::mutex> lock(mtx_reply);
  map_reply.at(reply->id()) = std::move(reply);
  cond_reply.notify_all();
}

void APIGateMultiThread::controller_on_require_invoke(Controller& sm, unsigned int msec) {
  if (th_controller->get_id() != std::this_thread::get_id()) {
    std::lock_guard<std::mutex> lock(mtx_call);
    tp = std::chrono ::steady_clock::now() + std::chrono::milliseconds(msec);
    cond_controller.notify_all();
  }
}

void APIGateMultiThread::logger_on_output(Logger& logger, LogLevel::Type level, const std::string& message) {
  std::unique_ptr<api::Event> event = std::make_unique<api::Event>();
  event->set_channel(APIChannel::COLONIO);
  api::colonio::LogEvent* log_event = event->mutable_colonio_log();
  log_event->set_level(static_cast<uint32_t>(level));
  log_event->set_message(message);

  push_event(std::move(event));
}

void APIGateMultiThread::loop_event() {
  std::unique_ptr<std::deque<std::unique_ptr<api::Event>>> events =
      std::make_unique<std::deque<std::unique_ptr<api::Event>>>();

  while (true) {
    try {
      if (events->empty()) {
        std::unique_lock<std::mutex> lock(mtx_event);
        cond_event.wait(lock, [this]() { return !que_event->empty() || has_end(); });

        // Get event and execute.
        if (has_end()) {
          break;

        } else if (!que_event->empty()) {
          que_event.swap(events);
        }
      }

      if (events->size() > EVENT_QUEUE_LIMIT) {
        logI(*this, "the event queue is stuck");
      }

      auto it_event = events->begin();
      while (it_event != events->end()) {
        std::unique_ptr<api::Event> event = std::move(*it_event);
        it_event                          = events->erase(it_event);

        auto it_map = map_event.find(event->channel());
        if (it_map != map_event.end()) {
          it_map->second(*event);
        } else {
          logE(*this, "send event for channel not set:%d", event->channel());
          assert(false);
        }
      }

    } catch (const std::exception& ex) {
      logE(*this, "exception what():%s", ex.what());
      exit(EXIT_FAILURE);
    }
  }
}

void APIGateMultiThread::loop_controller() {
  while (!has_end()) {
    // call
    while (true) {
      std::unique_ptr<api::Call> call;
      std::unique_lock<std::mutex> lock(mtx_call);
      if (que_call.empty()) {
        if (cond_controller.wait_until(lock, tp) == std::cv_status::timeout) {
          break;
        } else if (!que_call.empty()) {
          call = std::move(que_call.front());
          que_call.pop_front();
        }

      } else {
        call = std::move(que_call.front());
        que_call.pop_front();
      }
      lock.unlock();
      if (call) {
        try {
          controller.call(*call);
        } catch (const FatalException& ex) {
          logE(controller, "fatal exception %s:%d: %s", ex.file.c_str(), ex.line, ex.message.c_str());
          reply_failure(call->id(), ex.code, ex.message);
          // TODO stop
          return;

        } catch (const InternalException& ex) {
          logE(controller, "internal exception %s:%d: %s", ex.file.c_str(), ex.line, ex.message.c_str());
          reply_failure(call->id(), ex.code, ex.message);

        } catch (const std::exception& ex) {
          logE(controller, "exception what():%s", ex.what());
          reply_failure(call->id(), Exception::Code::UNDEFINED, ex.what());
          // TODO stop
          return;
        }
      }
    }

    try {
      // invoke
      unsigned int tp_add = controller.invoke();
      {
        std::lock_guard<std::mutex> lock(mtx_call);
        tp = std::chrono::steady_clock::now() + std::chrono::milliseconds(tp_add);
      }

    } catch (const FatalException& ex) {
      logE(controller, "fatal exception %s:%d: %s", ex.file.c_str(), ex.line, ex.message.c_str());
      // TODO stop
      return;

    } catch (const InternalException& ex) {
      logE(controller, "internal exception %s:%d: %s", ex.file.c_str(), ex.line, ex.message.c_str());

    } catch (const std::exception& ex) {
      logE(controller, "exception what():%s", ex.what());
      // TODO stop
      return;
    }
  }
}

bool APIGateMultiThread::has_end() {
  std::lock_guard<std::mutex> guard(mtx_end);
  return flg_end;
}

void APIGateMultiThread::push_event(std::unique_ptr<api::Event> event) {
  std::lock_guard<std::mutex> lock(mtx_event);
  que_event->push_back(std::move(event));
  cond_event.notify_all();
}

void APIGateMultiThread::reply_failure(uint32_t id, Exception::Code code, const std::string& message) {
  std::lock_guard<std::mutex> lock(mtx_reply);

  std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
  reply->set_id(id);

  api::Failure* param = reply->mutable_failure();
  param->set_code(static_cast<uint32_t>(code));
  param->set_message(message);

  map_reply.at(id) = std::move(reply);
  cond_reply.notify_all();
}
}  // namespace colonio
