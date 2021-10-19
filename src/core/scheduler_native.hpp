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
#include <map>
#include <memory>
#include <mutex>
#include <thread>

#include "scheduler.hpp"

namespace colonio {
class SchedulerNative : public Scheduler {
 public:
  SchedulerNative(Logger& logger, uint32_t opt);
  virtual ~SchedulerNative();

  void add_controller_loop(void* src, std::function<void()>&& func, unsigned int interval) override;
  void add_controller_task(void* src, std::function<void()>&& func, unsigned int after = 0) override;
  void add_user_task(void* src, std::function<void()>&& func) override;
  bool has_task(void* src) override;
  void remove_task(void* src, bool remove_current = true) override;

  void start_controller_routine() override;
  void start_user_routine() override;

 private:
  std::unique_ptr<std::thread> controller_thread;
  std::thread::id controller_tid;
  std::deque<Task> controller_running;
  std::deque<Task> controller_tasks;
  void* controller_remove_waiting;
  bool controller_remove_current;
  std::mutex controller_mtx;
  std::condition_variable controller_cond;

  std::unique_ptr<std::thread> user_thread;
  std::thread::id user_tid;
  std::deque<Task> user_tasks;
  void* user_remove_waiting;
  std::mutex user_mtx;
  std::condition_variable user_cond;

  bool flg_end;
};
}  // namespace colonio