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
#include <functional>
#include <map>
#include <mutex>
#include <thread>

class UserThreadPoolTest;
namespace colonio {
class Logger;

class UserThreadPool {
 public:
  UserThreadPool() = delete;
  UserThreadPool(Logger&, unsigned int max_threads, unsigned int bored_limit);
  virtual ~UserThreadPool();

  void push(std::function<void()>&& func);

 private:
  class Task {
   public:
    std::function<void()> func;

    explicit Task(std::function<void()>&);
  };

  struct Thread {
   public:
    std::thread th;
    std::unique_ptr<Task> task;
    std::condition_variable cv;
    bool dead;

    explicit Thread(std::function<void()>);
  };

  const unsigned int MAX_THREADS;
  const unsigned int BORED_LIMIT;

  Logger& logger;
  std::map<std::thread::id, std::unique_ptr<Thread>> threads;
  std::condition_variable threads_cv;
  // use it like stack, use deque to realize `house_keep`
  std::deque<std::thread::id> free_threads;
  std::condition_variable free_threads_cv;
  std::mutex mtx;
  bool fin;

  void start_on_thread();
  void house_keep();

  friend class ::UserThreadPoolTest;
};
}  // namespace colonio