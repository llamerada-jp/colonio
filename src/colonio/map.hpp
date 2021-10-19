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

#include <colonio/value.hpp>
#include <functional>

namespace colonio {
class Error;

/**
 * @brief Under developing.
 *
 */
class Map {
 public:
  // options
  static const uint32_t ERROR_WITHOUT_EXIST = 0x1;  // del, unlock
  static const uint32_t ERROR_WITH_EXIST    = 0x2;  // set (haven't done enough testing)
  static const uint32_t TRY_LOCK            = 0x4;  // lock (unsupported yet)

  virtual ~Map();

  virtual void each_local_value(std::function<void(Map&, const Value&, const Value&)>&& func) = 0;
  virtual Value get(const Value& key)                                                         = 0;
  virtual void get(
      const Value& key, std::function<void(Map&, const Value&)>&& on_success,
      std::function<void(Map&, const Error&)>&& on_failure)                   = 0;
  virtual void set(const Value& key, const Value& value, uint32_t opt = 0x00) = 0;
  virtual void set(
      const Value& key, const Value& value, uint32_t opt, std::function<void(Map&)>&& on_success,
      std::function<void(Map&, const Error&)>&& on_failure) = 0;
};
}  // namespace colonio
