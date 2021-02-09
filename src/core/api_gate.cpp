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
#include "api_gate.hpp"

#include <cassert>

namespace colonio {

Error get_error(const api::Reply& reply) {
  if (reply.has_failure()) {
    return Error(static_cast<ErrorCode>(reply.failure().code()), reply.failure().message());
  } else {
    return Error(ErrorCode::UNDEFINED, "unknown error");
  }
}

Exception get_exception(const api::Reply& reply) {
  if (reply.has_failure()) {
    return Exception(static_cast<ErrorCode>(reply.failure().code()), reply.failure().message());
  } else {
    return Exception(ErrorCode::UNDEFINED, "unknown error");
  }
}

APIGateBase::APIGateBase() : logger(*this) {
}

APIGateBase::~APIGateBase() {
}

void APIGateBase::start_on_event_thread() {
  // implementation error, must not use Colonio::start_on_event_thread in current api-gate case.
  assert(false);
}

void APIGateBase::start_on_controller_thread() {
  // implementation error, must not use Colonio::start_on_controller_thread in current api-gate case.
  assert(false);
}
}  // namespace colonio
