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

#include <string>

namespace colonio {
class Context;
class SeedLinkBase;

class SeedLinkDelegate {
 public:
  virtual ~SeedLinkDelegate();
  virtual void seed_link_on_connect(SeedLinkBase& link)                       = 0;
  virtual void seed_link_on_disconnect(SeedLinkBase& link)                    = 0;
  virtual void seed_link_on_error(SeedLinkBase& link)                         = 0;
  virtual void seed_link_on_recv(SeedLinkBase& link, const std::string& data) = 0;
};

class SeedLinkBase {
 public:
  SeedLinkDelegate& delegate;

  SeedLinkBase(SeedLinkDelegate& delegate_, Context& context_);
  virtual ~SeedLinkBase();

  virtual void connect(const std::string& url) = 0;
  virtual void disconnect()                    = 0;
  virtual void send(const std::string& data)   = 0;

 protected:
  Context& context;
};
}  // namespace colonio

#ifndef EMSCRIPTEN
#  include "seed_link_websocket_native.hpp"
#else
#  include "seed_link_websocket_wasm.hpp"
#endif
