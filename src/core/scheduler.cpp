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
#include "scheduler.hpp"

#include <cassert>
#include <iostream>

#include "utils.hpp"

namespace colonio {
SchedulerDelegate::~SchedulerDelegate() {
}

Scheduler::Scheduler(SchedulerDelegate& delegate_) : delegate(delegate_) {
}

Scheduler::~Scheduler() {
}

void Scheduler::add_interval_task(void* src, std::function<void()> func, unsigned int msec) {
  assert(msec != 0);

  std::lock_guard<std::mutex> guard(mtx);
  int64_t next = ((Utils::get_current_msec() / 1000) + 1) * 1000 + msec;
  tasks.push_back(std::make_shared<Task>(Task{src, func, msec, next}));
}

void Scheduler::add_timeout_task(void* src, std::function<void()> func, unsigned int msec) {
  std::lock_guard<std::mutex> guard(mtx);

  if (msec == 0 && running_tasks.size() != 0) {
    running_tasks.push_back(std::make_shared<Task>(Task{src, func, 0, 0}));

  } else {
    int64_t min_next = INT64_MAX;
    for (auto& it : tasks) {
      if (min_next > it->next) {
        min_next = it->next;
      }
    }

    int64_t current_msec = Utils::get_current_msec();
    tasks.push_back(std::make_shared<Task>(Task{src, func, 0, current_msec + msec}));

    if (min_next - current_msec > msec) {
      delegate.scheduler_on_require_invoke(*this, msec);
    }
  }
}

unsigned int Scheduler::invoke() {
  assert(running_tasks.size() == 0);
  {
    int64_t current_msec = Utils::get_current_msec();
    std::lock_guard<std::mutex> guard(mtx);
    auto it = tasks.begin();
    while (it != tasks.end()) {
      if ((*it)->next <= current_msec) {
        running_tasks.push_back(*it);
        if ((*it)->interval == 0) {
          it = tasks.erase(it);
        } else {
          it++;
        }
      } else {
        it++;
      }
    }
  }

  bool is_first = true;
  while (true) {
    std::shared_ptr<Task> task;
    {
      std::lock_guard<std::mutex> guard(mtx);
      if (is_first) {
        is_first = false;
      } else {
        running_tasks.pop_front();
      }
      if (running_tasks.size() > 0) {
        task = running_tasks.front();
      } else {
        break;
      }
    }

    task->func();
  }

  {
    std::lock_guard<std::mutex> guard(mtx);
    if (tasks.size() == 0) {
      return 0;
    } else {
      int64_t current_msec = Utils::get_current_msec();
      int64_t min_next     = INT64_MAX;
      for (auto& it : tasks) {
        while (it->next <= current_msec) {
          it->next += it->interval;
        }
        if (min_next > it->next) {
          min_next = it->next;
        }
      }
      return min_next - current_msec;
    }
  }
}

bool Scheduler::is_having_task(void* src) {
  std::lock_guard<std::mutex> guard(mtx);

  for (auto& it : running_tasks) {
    if (it->src == src) {
      return true;
    }
  }

  for (auto& it : tasks) {
    if (it->src == src) {
      return true;
    }
  }

  return false;
}

void Scheduler::remove_task(void* src) {
  std::lock_guard<std::mutex> guard(mtx);

  if (running_tasks.size() != 0) {
    auto it = running_tasks.begin();
    it++;
    while (it != running_tasks.end()) {
      if ((*it)->src == src) {
        it = running_tasks.erase(it);
      } else {
        it++;
      }
    }
  }

  {
    auto it = tasks.begin();
    while (it != tasks.end()) {
      if ((*it)->src == src) {
        it = tasks.erase(it);
      } else {
        it++;
      }
    }
  }
}
}  // namespace colonio
