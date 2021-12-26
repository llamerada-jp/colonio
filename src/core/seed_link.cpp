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

#include "seed_link.hpp"

#ifndef EMSCRIPTEN
#  include "seed_link_websocket_native.hpp"
#else
#  include "seed_link_websocket_wasm.hpp"
#endif

namespace colonio {
SeedLinkParam::SeedLinkParam(SeedLinkDelegate& delegate_, Logger& logger_) : delegate(delegate_), logger(logger_) {
}

SeedLinkDelegate::~SeedLinkDelegate() {
}

SeedLink* SeedLink::new_instance(SeedLinkParam& param) {
#ifndef EMSCRIPTEN
  return new SeedLinkWebsocketNative(param);
#else
  return new SeedLinkWebsocketWasm(param);
#endif
}

SeedLink::SeedLink(SeedLinkParam& param) : delegate(param.delegate), logger(param.logger) {
}

SeedLink::~SeedLink() {
}
}  // namespace colonio
