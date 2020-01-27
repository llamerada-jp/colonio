/*
 * Copyright 2017-2019 Yuji Ito <llamerada.jp@gmail.com>
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

enum class PubSub2DFailureReason : uint32_t { NONE, SYSTEM_ERROR, NOONE_RECV };

class PubSub2D {
 public:
  virtual ~PubSub2D();

  virtual void publish(
      const std::string& name, double x, double y, double r, const Value& value,
      const std::function<void()>& on_success, const std::function<void(PubSub2DFailureReason)>& on_failure) = 0;
  virtual void on(const std::string& name, const std::function<void(const Value&)>& subscriber)              = 0;
  virtual void off(const std::string& name)                                                                  = 0;
};
}  // namespace colonio
