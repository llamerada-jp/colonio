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

#include <chrono>
#include <condition_variable>
#include <set>
#include <sstream>
#include <string>
#include <thread>

class AsyncHelper {
 public:
  std::mutex mtx;

  std::stringstream marks;
  std::set<std::string> signals;
  std::condition_variable cond_signals;

  AsyncHelper() {
  }

  virtual ~AsyncHelper() {
  }

  void mark(const std::string& m) {
    std::lock_guard<std::mutex> lock(mtx);
    marks << m;
  }

  void clear_signal() {
    std::lock_guard<std::mutex> lock(mtx);
    signals.clear();
  }

  std::string get_route() {
    std::lock_guard<std::mutex> lock(mtx);
    return marks.str();
  }

  void pass_signal(const std::string& key) {
    std::lock_guard<std::mutex> lock(mtx);
    signals.insert(key);
    cond_signals.notify_all();
  }

  void wait_signal(const std::string& key) {
    std::unique_lock<std::mutex> lock(mtx);
    cond_signals.wait(lock, [this, &key]() {
      return signals.find(key) != signals.end();
    });
  }

  void wait_signal(const std::string& key, std::function<void()>&& func) {
    std::unique_lock<std::mutex> lock(mtx);
    while (!cond_signals.wait_for(lock, std::chrono::seconds(3), [this, &key]() {
      return signals.find(key) != signals.end();
    })) {
      func();
    }
  }

  bool check_signal(const std::string& key) {
    std::lock_guard<std::mutex> lock(mtx);
    return signals.find(key) != signals.end();
  }
};
