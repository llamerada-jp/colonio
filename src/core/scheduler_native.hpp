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

#include <condition_variable>
#include <deque>
#include <memory>
#include <mutex>
#include <thread>

#include "scheduler.hpp"

namespace colonio {
class SchedulerNative : public Scheduler {
 public:
  SchedulerNative(Logger& logger);
  virtual ~SchedulerNative();

  void repeat_task(void* src, std::function<void()>&& func, unsigned int interval) override;
  void add_task(void* src, std::function<void()>&& func, unsigned int after = 0) override;
  bool exists(void* src) override;
  bool is_controller_thread() const override;
  void remove(void* src, bool remove_current = true) override;

 private:
  std::unique_ptr<std::thread> th;
  std::thread::id tid;
  std::deque<Task> running;
  std::deque<Task> tasks;
  void* remove_waiting;
  bool remove_current;
  std::mutex mtx;
  std::condition_variable cv;
  bool fin;

  void sub_routine();
};
}  // namespace colonio