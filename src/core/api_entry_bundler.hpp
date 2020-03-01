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

#include <map>
#include <memory>

#include "api.pb.h"
#include "definition.hpp"

namespace colonio {
class APIEntry;

class APIEntryBundler {
 public:
  void call(const api::Call& call);
  void registrate(std::shared_ptr<APIEntry> entry);

 private:
  std::map<APIChannel::Type, std::shared_ptr<APIEntry>> entries;
};
}  // namespace colonio
