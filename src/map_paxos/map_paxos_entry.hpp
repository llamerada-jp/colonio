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

#include <picojson.h>

#include "colonio/map.hpp"
#include "core/api_entry.hpp"

namespace colonio {
class APIEntryBundler;
class APIEntryDelegate;
class APIModuleBundler;
class Context;
class MapPaxos;
class Value;

class MapPaxosEntry : public APIEntry {
 public:
  static void make_entry(
      Context& context, APIEntryBundler& api_bundler, APIEntryDelegate& entry_delegate,
      APIModuleBundler& module_bundler, const picojson::object& config);

 private:
  std::unique_ptr<MapPaxos> mp;

  MapPaxosEntry(
      Context& context, APIEntryDelegate& delegate, APIChannel::Type channel, std::unique_ptr<MapPaxos> module);

  void api_entry_on_recv_call(const api::Call& call) override;
  void api_get(uint32_t id, const Value& key);
  void api_set(uint32_t id, const Value& key, const Value& value, MapOption::Type opt);
};

}  // namespace colonio
