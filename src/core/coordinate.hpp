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

#include <picojson.h>

namespace colonio {
namespace proto {
class Coordinate;
}  // namespace proto

class Coordinate {
 public:
  double x;
  double y;

  static Coordinate from_pb(const proto::Coordinate& pb);

  Coordinate();
  Coordinate(double x_, double y_);
  bool operator<(const Coordinate& r) const;
  bool operator!=(const Coordinate& r) const;

  bool is_enable();
  void to_pb(proto::Coordinate* pb) const;
  picojson::value to_json() const;
};
}  // namespace colonio
