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

#include "core/api_entry.hpp"

namespace colonio {
class APIEntryBundler;
class APIEntryDelegate;
class APIModuleBundler;
class Context;

class MapPaxosEntry : public APIEntry {
 public:
  static void make_entry(
      Context& context, APIEntryBundler& api_bundler, APIModuleBundler& module_bundler,
      APIEntryDelegate& entry_delegate, const picojson::object& config);

 private:
  MapPaxosEntry(Context& context, APIEntryDelegate& delegate, APIChannel::Type channel);

  void api_entry_on_recv_call(const api::Call& call) override;
};

}  // namespace colonio
