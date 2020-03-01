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

#include "utils.hpp"

namespace colonio {

APIGateMultiThread::APIGateMultiThread() : side_module(*this), tp(std::chrono::steady_clock::now()) {
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
    que_call.push(std::move(call));
    cond_side_module.notify_all();
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
      return nullptr;
    }
  }
}

void APIGateMultiThread::init() {
  assert(!th_event);
  assert(!th_side_module);
  {
    std::lock_guard<std::mutex> lock(mtx_end);
    flg_end = false;
  }
  th_event       = std::make_unique<std::thread>(&APIGateMultiThread::loop_event, this);
  th_side_module = std::make_unique<std::thread>(&APIGateMultiThread::loop_side_module, this);
}

void APIGateMultiThread::quit() {
  {
    std::lock_guard<std::mutex> lock(mtx_end);
    flg_end = true;
  }
  cond_side_module.notify_all();
  cond_reply.notify_all();
  cond_event.notify_all();

  if (th_event) {
    th_event->join();
  }
  if (th_side_module) {
    th_side_module->join();
  }
}

void APIGateMultiThread::set_event_hook(APIChannel::Type channel, std::function<void(const api::Event&)> on_event) {
  std::lock_guard<std::mutex> lock(mtx_event);
  assert(map_event.find(channel) == map_event.end());
  map_event.insert(std::make_pair(channel, on_event));
}

void APIGateMultiThread::side_module_on_event(SideModule& sm, std::unique_ptr<api::Event> event) {
  std::lock_guard<std::mutex> lock(mtx_event);
  que_event.push(std::move(event));
  cond_event.notify_all();
}

void APIGateMultiThread::side_module_on_reply(SideModule& sm, std::unique_ptr<api::Reply> reply) {
  std::lock_guard<std::mutex> lock(mtx_reply);
  map_reply.at(reply->id()) = std::move(reply);
  cond_reply.notify_all();
}

void APIGateMultiThread::side_module_on_require_invoke(SideModule& sm, unsigned int msec) {
  if (th_side_module->get_id() != std::this_thread::get_id()) {
    std::lock_guard<std::mutex> lock(mtx_call);
    tp = std::chrono ::steady_clock::now() + std::chrono::milliseconds(msec);
    cond_side_module.notify_all();
  }
}

void APIGateMultiThread::loop_event() {
  while (true) {
    try {
      std::unique_lock<std::mutex> lock(mtx_event);
      cond_event.wait(lock, [this]() { return !que_event.empty() || has_end(); });

      // Get event and execute.
      if (has_end()) {
        break;

      } else if (!que_event.empty()) {
        std::unique_ptr<api::Event> event = std::move(que_event.front());
        que_event.pop();
        auto it = map_event.find(event->channel());
        if (it != map_event.end()) {
          auto func = it->second;
          lock.unlock();
          func(*event);
        } else {
          assert(false);
        }
      }
    } catch (const std::exception& ex) {
      logE(side_module, 0, "exception what():%s", ex.what());
      exit(EXIT_FAILURE);
    }
  }
}

void APIGateMultiThread::loop_side_module() {
  while (!has_end()) {
    try {
      // call
      while (true) {
        std::unique_ptr<api::Call> call;
        std::unique_lock<std::mutex> lock(mtx_call);
        if (que_call.empty()) {
          if (cond_side_module.wait_until(lock, tp) == std::cv_status::timeout) {
            break;
          } else if (!que_call.empty()) {
            call = std::move(que_call.front());
            que_call.pop();
          }

        } else {
          call = std::move(que_call.front());
          que_call.pop();
        }
        lock.unlock();
        if (call) {
          side_module.call(*call);
        }
      }

      // invoke
      unsigned int tp_add = side_module.invoke();
      {
        std::lock_guard<std::mutex> lock(mtx_call);
        tp = std::chrono::steady_clock::now() + std::chrono::milliseconds(tp_add);
      }

    } catch (const FatalException& ex) {
      logE(side_module, 0, "fatal %s@%d %s", ex.file.c_str(), ex.line, ex.message.c_str());
      exit(EXIT_FAILURE);

    } catch (const Exception& ex) {
      logE(side_module, 0, "error %s@%d %s", ex.file.c_str(), ex.line, ex.message.c_str());

    } catch (const std::exception& ex) {
      logE(side_module, 0, "exception what():%s", ex.what());
      exit(EXIT_FAILURE);
    }
  }
}

bool APIGateMultiThread::has_end() {
  std::lock_guard<std::mutex> guard(mtx_end);
  return flg_end;
}
}  // namespace colonio
