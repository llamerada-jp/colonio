/*
 * Copyright 2017-2020 Yuji Ito <llamerada.jp@gmail.com>
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

#include "api.pb.h"
#include "colonio/colonio_exception.hpp"
#include "definition.hpp"

namespace colonio {

// Helper method.
ColonioException get_exception(const api::Reply& reply);

class APIGateBase {
 public:
  virtual ~APIGateBase();
  virtual std::unique_ptr<api::Reply> call_sync(APIChannel::Type channel, const api::Call& call)           = 0;
  virtual void init()                                                                                      = 0;
  virtual void quit()                                                                                      = 0;
  virtual void set_event_hook(APIChannel::Type channel, std::function<void(const api::Event& e)> on_event) = 0;
};
}  // namespace colonio

#ifndef EMSCRIPTEN
#  include "api_gate_mt.hpp"
#else
#  include "api_gate_wasm.hpp"
#endif
