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

#include <colonio/value.hpp>
#include <cstdint>

namespace colonio {

namespace MapOption {
typedef uint32_t Type;
static const Type NONE                = 0x0;
static const Type ERROR_WITHOUT_EXIST = 0x1;  // del, unlook
// static const Type ERROR_WITH_EXIST    = 0x2; // set
static const Type TRY_LOCK = 0x4;  // lock
}  // namespace MapOption

class Map {
 public:
  virtual ~Map();

  virtual Value get(const Value& key)                                                           = 0;
  virtual void set(const Value& key, const Value& value, MapOption::Type opt = MapOption::NONE) = 0;
};
}  // namespace colonio
