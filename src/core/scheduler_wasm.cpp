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
#include "scheduler_wasm.hpp"

#include <emscripten.h>

#include <cassert>

#include "utils.hpp"

extern "C" {
typedef unsigned long COLONIO_PTR_T;

extern void scheduler_release(COLONIO_PTR_T this_ptr);
extern void scheduler_request_next_routine(COLONIO_PTR_T this_ptr, int msec);

EMSCRIPTEN_KEEPALIVE int scheduler_invoke(COLONIO_PTR_T this_ptr) {
  colonio::SchedulerWasm& THIS = *reinterpret_cast<colonio::SchedulerWasm*>(this_ptr);
  return THIS.exec_tasks();
}
}  // extern "C"

namespace colonio {
SchedulerWasm::SchedulerWasm(Logger& logger, uint32_t opt) : Scheduler(logger) {
  assert(opt == 0);
}

SchedulerWasm::~SchedulerWasm() {
  scheduler_release(reinterpret_cast<COLONIO_PTR_T>(this));
}

void SchedulerWasm::add_controller_loop(void* src, std::function<void()>&& func, unsigned int interval) {
  assert(interval != 0);
  const int64_t CURRENT_MSEC = Utils::get_current_msec();
  tasks.push_back(Task{src, func, interval, static_cast<int64_t>(std::floor(CURRENT_MSEC / interval)) * interval});
  if (running.size() == 0) {
    request_next_routine();
  }
}

void SchedulerWasm::add_controller_task(void* src, std::function<void()>&& func, unsigned int after) {
  if (after == 0 && running.size() != 0) {
    running.push_back(Task{src, func, 0, 0});

  } else {
    const int64_t CURRENT_MSEC = Utils::get_current_msec();
    tasks.push_back(Task{src, func, 0, CURRENT_MSEC + after});
    if (running.size() == 0) {
      request_next_routine();
    }
  }
}

void SchedulerWasm::add_user_task(void* src, std::function<void()>&& func) {
  if (running.size() != 0) {
    running.push_back(Task{src, func, 0, 0});

  } else {
    const int64_t CURRENT_MSEC = Utils::get_current_msec();
    tasks.push_back(Task{src, func, 0, CURRENT_MSEC});
    request_next_routine();
  }
}

bool SchedulerWasm::has_task(void* src) {
  for (auto& task : running) {
    if (task.src == src) {
      return true;
    }
  }

  for (auto& task : tasks) {
    if (task.src == src) {
      return true;
    }
  }

  return false;
}

bool SchedulerWasm::is_controller_thread() const {
  return true;
}

bool SchedulerWasm::is_user_thread() const {
  return true;
}

void SchedulerWasm::remove_task(void* src, bool remove_current) {
  assert(!remove_current || running.front().src != src);
  remove_deque_tasks(src, &tasks, true);
  remove_deque_tasks(src, &running, remove_current);
}

void SchedulerWasm::start_controller_routine() {
  assert(false);
}

void SchedulerWasm::start_user_routine() {
  assert(false);
}

int SchedulerWasm::exec_tasks() {
  assert(running.size() == 0);
  pick_runnable_tasks(&running, &tasks);

  while (!running.empty()) {
    Task& task = running.front();
    task.func();
    running.pop_front();
  }

  int64_t next = get_next_timeing(tasks);
  next -= Utils::get_current_msec();
  if (next < 0) {
    next = 0;
  }
  return next;
}

void SchedulerWasm::request_next_routine() {
  int64_t next = get_next_timeing(tasks);
  next -= Utils::get_current_msec();
  if (next < 0) {
    next = 0;
  }
  scheduler_request_next_routine(reinterpret_cast<COLONIO_PTR_T>(this), next);
}
}  // namespace colonio