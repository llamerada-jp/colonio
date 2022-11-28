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
#pragma once

#ifdef __clang__
#  include <picojson.h>
#else
#  pragma GCC diagnostic push
#  pragma GCC diagnostic ignored "-Wmaybe-uninitialized"
#  include <picojson.h>
#  pragma GCC diagnostic pop
#endif

#include <cstdint>
#include <string>
#include <vector>

#include "colonio/colonio.hpp"

namespace colonio {
namespace proto {
class Value;
}  // namespace proto

class ValueImpl {
 public:
  union Storage {
    bool bool_v;
    int64_t int64_v;
    double double_v;
    std::string* string_v;
    std::vector<uint8_t>* binary_v;
  };

  Value::Type type;
  Storage storage;

  ValueImpl();
  explicit ValueImpl(const ValueImpl& src);
  explicit ValueImpl(bool v);
  explicit ValueImpl(int64_t v);
  explicit ValueImpl(double v);
  explicit ValueImpl(const std::string& v);
  ValueImpl(const void* v, unsigned int siz);
  virtual ~ValueImpl();

  static void to_pb(proto::Value* pb, const Value& value);
  static Value from_pb(const proto::Value& pb);

  static std::string to_str(const Value& value);

  bool operator<(const ValueImpl& b) const;
};
}  // namespace colonio
