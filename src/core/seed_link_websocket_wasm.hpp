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

#include "seed_link.hpp"

namespace colonio {
class SeedLinkWebsocketWasm : public SeedLinkBase {
 public:
#ifndef NDEBUG
  SeedLinkWebsocketWasm* debug_ptr;
#endif

  SeedLinkWebsocketWasm(SeedLinkDelegate& delegate_, Context& context_);
  virtual ~SeedLinkWebsocketWasm();

  // SeedlinkBase
  void connect(const std::string& url) override;
  void disconnect() override;
  void send(const std::string& data) override;
};

typedef SeedLinkWebsocketWasm SeedLink;
}  // namespace colonio
