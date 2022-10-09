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
#pragma once

#include <deque>
#include <functional>

namespace colonio {
class Logger;

class Scheduler {
 public:
  static Scheduler* new_instance(Logger& logger);

  virtual ~Scheduler();

  virtual void repeat_task(void* src, std::function<void()>&& func, unsigned int interval) = 0;
  virtual void add_task(void* src, std::function<void()>&& func, unsigned int delay = 0)   = 0;
  virtual bool exists(void* src)                                                           = 0;
  virtual bool is_controller_thread() const                                                = 0;
  virtual void remove(void* src, bool remove_current = true)                               = 0;

 protected:
  struct Task {
    void* src;
    std::function<void()> func;
    unsigned int interval;
    int64_t next;
  };

  Logger& logger;

  explicit Scheduler(Logger& logger_);
  Scheduler(const Scheduler&) = delete;

  int64_t get_next_timing(std::deque<Task>& src);
  void pick_runnable_tasks(std::deque<Task>* dst, std::deque<Task>* src);
  void remove_deque_tasks(void* src, std::deque<Task>* dq, bool remove_head);
};
}  // namespace colonio
