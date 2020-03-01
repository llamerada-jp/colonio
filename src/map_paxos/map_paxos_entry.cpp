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
#include "map_paxos_entry.hpp"

#include "core/utils.hpp"

namespace colonio {
void MapPaxosEntry::make_entry(
    Context& context, APIEntryBundler& api_bundler, APIModuleBundler& module_bundler, APIEntryDelegate& entry_delegate,
    const picojson::object& config) {
  APIChannel::Type channel = static_cast<APIChannel::Type>(Utils::get_json<double>(config, "channel"));
  unsigned int retry_max   = Utils::get_json<double>(config, "retryMax", MAP_PAXOS_RETRY_MAX);
}
}  // namespace colonio
