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
#include "pubsub_2d_impl.hpp"

#include <utility>

#include "colonio/error.hpp"
#include "pipe.hpp"
#include "pubsub_2d_base.hpp"

namespace colonio {
Pubsub2DImpl::Pubsub2DImpl(Pubsub2DBase& base_) : base(base_) {
}

void Pubsub2DImpl::publish(const std::string& name, double x, double y, double r, const Value& value, uint32_t opt) {
  Pipe<int> pipe;

  base.publish(
      name, x, y, r, value, opt,
      [&pipe] {
        pipe.push(1);
      },
      [&pipe](const Error& error) {
        pipe.pushError(error);
      });

  pipe.pop_with_throw();
}

void Pubsub2DImpl::publish(
    const std::string& name, double x, double y, double r, const Value& value, uint32_t opt,
    std::function<void(Pubsub2D&)>&& on_success, std::function<void(Pubsub2D&, const Error&)>&& on_failure) {
  base.publish(
      name, x, y, r, value, opt,
      [this, on_success] {
        on_success(*this);
      },
      [this, on_failure](const Error& e) {
        on_failure(*this, e);
      });
}

void Pubsub2DImpl::on(const std::string& name, std::function<void(Pubsub2D&, const Value&)>&& subscriber) {
  base.on(name, [this, subscriber](const Value& value) {
    subscriber(*this, value);
  });
}

void Pubsub2DImpl::off(const std::string& name) {
  base.off(name);
}
}  // namespace colonio
