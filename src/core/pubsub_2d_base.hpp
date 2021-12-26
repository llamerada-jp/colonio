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

#include <functional>

#include "module_2d.hpp"

namespace colonio {
class Error;
class Value;

class Pubsub2DBase : public Module2D {
 public:
  virtual void publish(
      const std::string& name, double x, double y, double r, const Value& value, uint32_t opt,
      std::function<void()>&& on_success, std::function<void(const Error&)>&& on_failure) = 0;

  virtual void on(const std::string& name, std::function<void(const Value&)>&& subscriber) = 0;
  virtual void off(const std::string& name)                                                = 0;

 protected:
  Pubsub2DBase(
      ModuleParam& param, Module2DDelegate& module_2d_delegate, const CoordSystem& coord_system, Channel::Type channel);
};
}  // namespace colonio
