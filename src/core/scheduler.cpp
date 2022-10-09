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
#include "scheduler.hpp"

#ifndef EMSCRIPTEN
#  include "scheduler_native.hpp"
#else
#  include "scheduler_wasm.hpp"
#endif
#include "utils.hpp"

namespace colonio {
Scheduler::Scheduler(Logger& logger_) : logger(logger_) {
}

Scheduler::~Scheduler() {
}

Scheduler* Scheduler::new_instance(Logger& logger) {
#ifndef EMSCRIPTEN
  return new SchedulerNative(logger);
#else
  return new SchedulerWasm(logger);
#endif
}

int64_t Scheduler::get_next_timing(std::deque<Task>& src) {
  const int64_t CURRENT_MSEC = Utils::get_current_msec();
  int64_t next               = CURRENT_MSEC + 10 * 1000;  // max 10 sec

  for (auto task : src) {
    if (task.next < next) {
      next = task.next;
    }
  }

  return next;
}

void Scheduler::pick_runnable_tasks(std::deque<Task>* dst, std::deque<Task>* src) {
  const int64_t CURRENT_MSEC = Utils::get_current_msec();

  auto task = src->begin();
  while (task != src->end()) {
    if (CURRENT_MSEC < task->next) {
      task++;
      continue;
    }

    dst->push_back(*task);
    if (task->interval == 0) {
      task = src->erase(task);

    } else {
      while (task->next <= CURRENT_MSEC) {
        task->next += task->interval;
      }
      task++;
    }
  }
}

void Scheduler::remove_deque_tasks(void* src, std::deque<Task>* dq, bool remove_head) {
  if (src == nullptr) {
    return;
  }

  bool is_first = true;
  auto it       = dq->begin();
  while (it != dq->end()) {
    if (!remove_head && is_first && it->src == src) {
      is_first = false;
      it++;

    } else if (it->src == src) {
      it = dq->erase(it);

    } else {
      it++;
    }
  }
}
}  // namespace colonio
