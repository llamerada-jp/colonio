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

#include <cassert>
#include <condition_variable>
#include <memory>
#include <mutex>
#include <thread>
#include <utility>

namespace colonio {

/**
 * @brief Pipe is a class to help with inter-thread message passing, like channel in golang.
 * You can pass one data instance in one Pipe instance, and it cannot be reused.
 * A copy of the data instance will be created internally.
 */
template<class T>
class Pipe {
 public:
  Pipe() {
  }

  /**
   * @brief Pass the value to another thread.
   * This method works asynchronously with pop method.
   * @param instance_ A value to pass to another thread.
   */
  void push(const T& instance_) {
    std::lock_guard<std::mutex> lock(mtx);
    assert(!instance);
    assert(!error);
    instance = std::make_unique<T>(instance_);
    cond.notify_all();
  }

  void pushError(const Error& error_) {
    std::lock_guard<std::mutex> lock(mtx);
    assert(!instance);
    assert(!error);
    error = std::make_unique<Error>(error_);
    cond.notify_all();
  }

  /**
   * @brief Get a copy of the pushed value.
   * This method will wait for the value to be pushed.
   * @return T& A Reference to a copy of the pushed value.
   */
  std::pair<T*, Error*> pop() {
    std::unique_lock<std::mutex> lock(mtx);
    cond.wait(lock, [this] {
      return instance || error;
    });
    return std::make_pair(instance.get(), error.get());
  }

 private:
  std::mutex mtx;
  std::condition_variable cond;
  std::unique_ptr<T> instance;
  std::unique_ptr<Error> error;

  explicit Pipe(const Pipe&);
  void operator=(const Pipe&);
};
}  // namespace colonio
