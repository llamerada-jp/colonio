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

#include "api_entry_bundler.hpp"

#include "api_entry.hpp"
#include "utils.hpp"

namespace colonio {
void APIEntryBundler::call(const api::Call& call) {
  assert(call.id() != 0);

  auto entry = entries.find(call.channel());
  if (entry != entries.end()) {
    entry->second->api_entry_on_recv_call(call);
  } else {
    colonio_fatal("Called incorrect API entry : %d", call.channel());
  }
}

void APIEntryBundler::registrate(std::shared_ptr<APIEntry> entry) {
  assert(entry->channel != APIChannel::NONE);
  assert(entries.find(entry->channel) == entries.end());

  entries.insert(std::make_pair(entry->channel, entry));
}
}  // namespace colonio
