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

#include <ctime>
#include <deque>
#include <functional>
#include <list>
#include <memory>
#include <mutex>

namespace colonio {
class Logger;
class Scheduler;

class SchedulerDelegate {
 public:
  virtual ~SchedulerDelegate();
  virtual void scheduler_on_require_invoke(Scheduler& sched, unsigned int msec) = 0;
};

class Scheduler {
 public:
  Scheduler(SchedulerDelegate& delegate_, Logger& logger_);
  virtual ~Scheduler();

  void add_interval_task(void* src, std::function<void()> func, unsigned int msec);
  void add_timeout_task(void* src, std::function<void()> func, unsigned int msec);
  unsigned int invoke();
  bool is_having_task(void* src);
  void remove_task(void* src);

 private:
  SchedulerDelegate& delegate;
  Logger& logger;
  struct Task {
    void* src;
    std::function<void()> func;
    unsigned int interval;
    int64_t next;
  };
  std::list<std::shared_ptr<Task>> tasks;
  std::deque<std::shared_ptr<Task>> running_tasks;
  std::mutex mtx;

  Scheduler(const Scheduler&);
  void operator=(const Scheduler&);
};

}  // namespace colonio
