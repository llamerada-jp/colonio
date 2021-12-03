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

#include "map_impl.hpp"

#include "colonio/error.hpp"
#include "map_base.hpp"
#include "pipe.hpp"
#include "value_impl.hpp"

namespace colonio {

MapImpl::MapImpl(MapBase& base_) : base(base_) {
}

void MapImpl::foreach_local_value(std::function<void(Map&, const Value&, const Value&)>&& func) {
  base.foreach_local_value([this, &func](const Value& key, const Value& value) {
    func(*this, key, value);
  });
}

Value MapImpl::get(const Value& key) {
  Pipe<Value> pipe;

  base.get(
      key,
      [&pipe](const Value& value) {
        pipe.push(value);
      },
      [&pipe](const Error& error) {
        pipe.pushError(error);
      });

  return *pipe.pop_with_throw();
}

void MapImpl::get(
    const Value& key, std::function<void(Map&, const Value&)>&& on_success,
    std::function<void(Map&, const Error&)>&& on_failure) {
  base.get(
      key,
      [this, on_success](const Value& value) {
        on_success(*this, value);
      },
      [this, on_failure](const Error& error) {
        on_failure(*this, error);
      });
}

void MapImpl::set(const Value& key, const Value& value, uint32_t opt) {
  Pipe<int> pipe;

  base.set(
      key, value, opt,
      [&pipe]() {
        pipe.push(1);
      },
      [&pipe](const Error& error) {
        pipe.pushError(error);
      });

  pipe.pop_with_throw();
}

void MapImpl::set(
    const Value& key, const Value& value, uint32_t opt, std::function<void(Map&)>&& on_success,
    std::function<void(Map&, const Error&)>&& on_failure) {
  base.set(
      key, value, opt,
      [this, on_success]() {
        on_success(*this);
      },
      [this, on_failure](const Error& error) {
        on_failure(*this, error);
      });
}
}  // namespace colonio
