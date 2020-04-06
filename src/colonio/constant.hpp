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

#include <cstdint>

namespace colonio {

namespace DebugEvent {
enum Type {
  MAP_SET,
  LINKS,
  NEXTS,
  POSITION,
  REQUIRED_1D,
  REQUIRED_2D,
  KNOWN_1D,
  KNOWN_2D,
};
}  // namespace DebugEvent

enum class Error : uint32_t {
  UNDEFINED,
  SYSTEM_ERROR,
  OFFLINE,
  INCORRECT_DATA_FORMAT,
  CONFLICT_WITH_SETTING,
  NOT_EXIST_KEY,
  // EXIST_KEY,
  CHANGED_PROPOSER,
  COLLISION_LATE,
  NO_ONE_RECV,
};

enum class LogLevel : uint32_t {
  //
  INFO,
  WARN,
  ERROR,
  DEBUG
};

}  // namespace colonio
