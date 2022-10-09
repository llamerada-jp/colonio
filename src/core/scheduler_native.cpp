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
#include "scheduler_native.hpp"

#include <cassert>
#include <cmath>

#include "logger.hpp"
#include "utils.hpp"

namespace colonio {
SchedulerNative::SchedulerNative(Logger& logger) :
    Scheduler(logger), remove_waiting(nullptr), remove_current(false), fin(false) {
  th = std::make_unique<std::thread>(std::bind(&SchedulerNative::sub_routine, this));
}

SchedulerNative::~SchedulerNative() {
  {
    std::lock_guard<std::mutex> lock(mtx);
    fin = true;
    cv.notify_all();
  }

  if (th) {
    th->join();
  }
}

void SchedulerNative::repeat_task(void* src, std::function<void()>&& func, unsigned int interval) {
  assert(interval != 0);
  std::lock_guard<std::mutex> lock(mtx);
  const int64_t CURRENT_MSEC = Utils::get_current_msec();

  tasks.push_back(Task{src, func, interval, static_cast<int64_t>(std::floor(CURRENT_MSEC / interval)) * interval});
  if (!is_controller_thread()) {
    cv.notify_all();
  }
}

void SchedulerNative::add_task(void* src, std::function<void()>&& func, unsigned int after) {
  if (after == 0 && is_controller_thread() && running.size() != 0) {
    running.push_back(Task{src, func, 0, 0});

  } else {
    std::lock_guard<std::mutex> lock(mtx);

    const int64_t CURRENT_MSEC = Utils::get_current_msec();
    tasks.push_back(Task{src, func, 0, CURRENT_MSEC + after});
    if (!is_controller_thread()) {
      cv.notify_all();
    }
  }
}

bool SchedulerNative::exists(void* src) {
  assert(is_controller_thread());
  for (auto& task : running) {
    if (task.src == src) {
      return true;
    }
  }

  std::lock_guard<std::mutex> lock(mtx);
  for (auto& task : tasks) {
    if (task.src == src) {
      return true;
    }
  }

  return false;
}

bool SchedulerNative::is_controller_thread() const {
  return std::this_thread::get_id() == tid;
}

void SchedulerNative::remove(void* src, bool remove_current) {
  if (is_controller_thread()) {
    assert(!remove_current || running.front().src != src);
    remove_deque_tasks(src, &tasks, true);
    remove_deque_tasks(src, &running, remove_current);

  } else {
    std::unique_lock<std::mutex> guard(mtx);
    cv.wait(guard, [this] {
      return remove_waiting == nullptr;
    });
    remove_waiting = src;
    remove_current = remove_current;
    cv.notify_all();
    cv.wait(guard, [this, src] {
      return remove_waiting != src;
    });
  }
}

void SchedulerNative::sub_routine() {
  tid = std::this_thread::get_id();

  while (true) {
    {
      assert(running.empty());
      std::unique_lock<std::mutex> lock(mtx);

      int64_t next    = get_next_timing(tasks);
      int64_t CURRENT = Utils::get_current_msec();
      if (next > CURRENT) {
        std::chrono::milliseconds interval(next - CURRENT);
        cv.wait_for(lock, interval, [this, next] {
          int64_t update_next = get_next_timing(tasks);
          return update_next != next || remove_waiting != nullptr || fin;
        });
      }

      if (fin) {
        return;
      }

      pick_runnable_tasks(&running, &tasks);

      if (remove_waiting != nullptr) {
        remove_deque_tasks(remove_waiting, &tasks, true);
        remove_deque_tasks(remove_waiting, &running, remove_current);
        remove_waiting = nullptr;
        cv.notify_all();
      }
    }

    while (!running.empty()) {
      Task& task = running.front();
      try {
        task.func();

      } catch (Error& e) {
        if (e.fatal) {
          log_error(e.what()).map_int("code", static_cast<int>(e.code)).map_u32("line", e.line).map("file", e.file);
          return;

        } else {
          log_warn(e.what()).map_int("code", static_cast<int>(e.code)).map_u32("line", e.line).map("file", e.file);
        }
      }
      running.pop_front();
    }
  }
}
}  // namespace colonio