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
  static Scheduler* new_instance(Logger& logger, uint32_t opt);

  virtual ~Scheduler();

  virtual void add_controller_loop(void* src, std::function<void()>&& func, unsigned int interval)  = 0;
  virtual void add_controller_task(void* src, std::function<void()>&& func, unsigned int after = 0) = 0;
  virtual void add_user_task(void* src, std::function<void()>&& func)                               = 0;
  virtual bool has_task(void* src)                                                                  = 0;
  virtual bool is_controller_thread() const                                                         = 0;
  virtual bool is_user_thread() const                                                               = 0;
  virtual void remove_task(void* src, bool remove_current = true)                                   = 0;

  virtual void start_controller_routine() = 0;
  virtual void start_user_routine()       = 0;

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

  int64_t get_next_timeing(std::deque<Task>& src);
  void pick_runnable_tasks(std::deque<Task>* dst, std::deque<Task>* src);
  void remove_deque_tasks(void* src, std::deque<Task>* dq, bool remove_head);
};
}  // namespace colonio
