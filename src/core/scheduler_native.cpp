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

#include "colonio/colonio.hpp"
#include "utils.hpp"

namespace colonio {
SchedulerNative::SchedulerNative(Logger& logger, uint32_t opt) :
    Scheduler(logger), controller_remove_waiting(nullptr), user_remove_waiting(nullptr), flg_end(false) {
  if ((opt & Colonio::EXPLICIT_CONTROLLER_THREAD) == 0) {
    controller_thread = std::make_unique<std::thread>(&SchedulerNative::start_controller_routine, this);
  }
  if ((opt & Colonio::EXPLICIT_EVENT_THREAD) == 0) {
    user_thread = std::make_unique<std::thread>(&SchedulerNative::start_user_routine, this);
  }
}

SchedulerNative::~SchedulerNative() {
  {
    std::lock_guard<std::mutex> lock1(controller_mtx);
    std::lock_guard<std::mutex> lock2(user_mtx);
    flg_end = true;
    controller_cond.notify_all();
    user_cond.notify_all();
  }

  if (controller_thread) {
    controller_thread->join();
  }
  if (user_thread) {
    user_thread->join();
  }
}

void SchedulerNative::add_controller_loop(void* src, std::function<void()>&& func, unsigned int interval) {
  assert(interval != 0);
  std::lock_guard<std::mutex> guard(controller_mtx);
  const int64_t CURRENT_MSEC = Utils::get_current_msec();

  controller_tasks.push_back(
      Task{src, func, interval, static_cast<int64_t>(std::floor(CURRENT_MSEC / interval)) * interval});
  if (!is_controller_thread()) {
    controller_cond.notify_all();
  }
}

void SchedulerNative::add_controller_task(void* src, std::function<void()>&& func, unsigned int after) {
  if (after == 0 && is_controller_thread() && controller_running.size() != 0) {
    controller_running.push_back(Task{src, func, 0, 0});

  } else {
    std::lock_guard<std::mutex> guard(controller_mtx);

    const int64_t CURRENT_MSEC = Utils::get_current_msec();
    controller_tasks.push_back(Task{src, func, 0, CURRENT_MSEC + after});
    if (!is_controller_thread()) {
      controller_cond.notify_all();
    }
  }
}

void SchedulerNative::add_user_task(void* src, std::function<void()>&& func) {
  std::lock_guard<std::mutex> guard(user_mtx);
  user_tasks.push_back(Task{src, func, 0, 0});
  if (!is_user_thread()) {
    user_cond.notify_all();
  }
}

bool SchedulerNative::has_task(void* src) {
  assert(is_controller_thread());
  for (auto& task : controller_running) {
    if (task.src == src) {
      return true;
    }
  }

  {
    std::lock_guard<std::mutex> guard(controller_mtx);
    for (auto& task : controller_tasks) {
      if (task.src == src) {
        return true;
      }
    }
  }

  {
    std::lock_guard<std::mutex> guard(user_mtx);
    for (auto& task : user_tasks) {
      if (task.src == src) {
        return true;
      }
    }
  }

  return false;
}

bool SchedulerNative::is_controller_thread() const {
  return std::this_thread::get_id() == controller_tid;
}

bool SchedulerNative::is_user_thread() const {
  return std::this_thread::get_id() == user_tid;
}

void SchedulerNative::remove_task(void* src, bool remove_current) {
  assert(!is_user_thread());

  if (is_controller_thread()) {
    assert(!remove_current || controller_running.front().src != src);
    remove_deque_tasks(src, &controller_tasks, true);
    remove_deque_tasks(src, &controller_running, remove_current);

  } else {
    std::unique_lock<std::mutex> guard(controller_mtx);
    controller_cond.wait(guard, [this] {
      return controller_remove_waiting == nullptr;
    });
    controller_remove_waiting = src;
    controller_remove_current = remove_current;
    controller_cond.notify_all();
    controller_cond.wait(guard, [this, src] {
      return controller_remove_waiting != src;
    });
  }

  {
    std::unique_lock<std::mutex> guard(user_mtx);
    user_cond.wait(guard, [this] {
      return user_remove_waiting == nullptr;
    });
    user_remove_waiting = src;
    user_cond.notify_all();
    user_cond.wait(guard, [this, src] {
      return user_remove_waiting != src;
    });
  }
}

void SchedulerNative::start_controller_routine() {
  controller_tid = std::this_thread::get_id();

  while (true) {
    {
      assert(controller_running.empty());
      std::unique_lock<std::mutex> guard(controller_mtx);

      int64_t next    = get_next_timeing(controller_tasks);
      int64_t CURRENT = Utils::get_current_msec();
      if (next > CURRENT) {
        std::chrono::milliseconds interval(next - CURRENT);
        controller_cond.wait_for(guard, interval, [this, next] {
          int64_t update_next = get_next_timeing(controller_tasks);
          return update_next != next || controller_remove_waiting != nullptr || flg_end;
        });
      }

      if (flg_end) {
        return;
      }

      pick_runnable_tasks(&controller_running, &controller_tasks);

      if (controller_remove_waiting != nullptr) {
        remove_deque_tasks(controller_remove_waiting, &controller_tasks, true);
        remove_deque_tasks(controller_remove_waiting, &controller_running, controller_remove_current);
        controller_remove_waiting = nullptr;
        controller_cond.notify_all();
      }
    }

    while (!controller_running.empty()) {
      Task& task = controller_running.front();
      task.func();
      controller_running.pop_front();
    }
  }
}

void SchedulerNative::start_user_routine() {
  user_tid = std::this_thread::get_id();
  std::deque<Task> running;

  while (true) {
    bool notify = false;
    {
      std::unique_lock<std::mutex> guard(user_mtx);
      user_cond.wait(guard, [this] {
        return !user_tasks.empty() || user_remove_waiting != nullptr || flg_end;
      });

      if (flg_end) {
        return;
      }

      if (user_remove_waiting != nullptr) {
        remove_deque_tasks(user_remove_waiting, &user_tasks, true);
        user_remove_waiting = nullptr;
        notify              = true;
      }

      if (!user_tasks.empty()) {
        assert(running.empty());
        user_tasks.swap(running);
        notify = true;
      }

      if (notify) {
        user_cond.notify_all();
      }
    }

    for (auto& task : running) {
      task.func();
    }

    running.clear();
  }
}
}  // namespace colonio