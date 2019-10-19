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
#include <picojson.h>

#include <cstdint>
#include <string>

#include "colonio/value.hpp"
#include "node_id.hpp"
#include "protocol.pb.h"

namespace colonio {
class ValueImpl {
 public:
  union Storage {
    bool bool_v;
    int64_t int64_v;
    double double_v;
    std::string* string_v;
  };

  Value::Type type;
  Storage storage;

  ValueImpl();
  explicit ValueImpl(bool v);
  explicit ValueImpl(int64_t v);
  explicit ValueImpl(double v);
  explicit ValueImpl(const std::string& v);
  explicit ValueImpl(const char* v);
  ValueImpl(const ValueImpl& src);
  virtual ~ValueImpl();

  static void to_pb(Protocol::Value* pb, const Value& value);
  static Value from_pb(const Protocol::Value& pb);

  static NodeID to_hash(const Value& value, const std::string& solt);
  static std::string to_str(const Value& value);

  bool operator<(const ValueImpl& b) const;
};
}  // namespace colonio
