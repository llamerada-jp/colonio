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
#include "colonio/colonio.hpp"

#include "colonio_impl.hpp"

namespace colonio {

const uint32_t Colonio::EXPLICIT_EVENT_THREAD;
const uint32_t Colonio::EXPLICIT_CONTROLLER_THREAD;

const uint32_t Colonio::CALL_ACCEPT_NEARBY;
const uint32_t Colonio::CALL_IGNORE_REPLY;

Colonio* Colonio::new_instance(std::function<void(Colonio&, const std::string&)>&& log_receiver, uint32_t opt) {
  if (log_receiver == nullptr) {
    return new ColonioImpl([](Colonio&, const ::std::string&) {}, opt);
  } else {
    return new ColonioImpl(log_receiver, opt);
  }
}

Colonio::Colonio() {
}

Colonio::~Colonio() {
}
}  // namespace colonio
