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

#include "colonio/pubsub_2d.hpp"

namespace colonio {
class Pubsub2DBase;

class Pubsub2DImpl : public Pubsub2D {
 public:
  explicit Pubsub2DImpl(Pubsub2DBase& base_);

  void publish(const std::string& name, double x, double y, double r, const Value& value, uint32_t opt) override;
  void publish(
      const std::string& name, double x, double y, double r, const Value& value, uint32_t opt,
      std::function<void(Pubsub2D&)>&& on_success, std::function<void(Pubsub2D&, const Error&)>&& on_failure) override;

  void on(const std::string& name, std::function<void(Pubsub2D&, const Value&)>&& subscriber) override;
  void off(const std::string& name) override;

 private:
  Pubsub2DBase& base;
};
}  // namespace colonio
