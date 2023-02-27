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

#include "coordinate.hpp"

#include <cassert>
#include <cmath>
#include <limits>

#include "colonio.pb.h"
#include "utils.hpp"

namespace colonio {
Coordinate::Coordinate() :
    x(std::numeric_limits<double>::signaling_NaN()), y(std::numeric_limits<double>::signaling_NaN()) {
}

Coordinate::Coordinate(double x_, double y_) : x(x_), y(y_) {
}

Coordinate Coordinate::from_pb(const proto::Coordinate& pb) {
  return Coordinate(pb.x(), pb.y());
}

bool Coordinate::operator<(const Coordinate& r) const {
  assert(!std::isnan(x) && !std::isnan(y) && !std::isnan(r.x) && !std::isnan(r.y));

  if (x != r.x) {
    return x < r.x;

  } else {
    return y < r.y;
  }
}

bool Coordinate::operator!=(const Coordinate& r) const {
  assert(!std::isnan(x) && !std::isnan(y) && !std::isnan(r.x) && !std::isnan(r.y));

  return x != r.x || y != r.y;
}

bool Coordinate::is_enable() {
  if (!std::isnan(x) && !std::isnan(y)) {
    return true;

  } else {
    return false;
  }
}

void Coordinate::to_pb(proto::Coordinate* pb) const {
  pb->set_x(x);
  pb->set_y(y);
}

picojson::value Coordinate::to_json() const {
  assert(Utils::is_safe_value(x) && Utils::is_safe_value(y));
  picojson::object obj;
  obj.insert(std::make_pair("x", picojson::value(x)));
  obj.insert(std::make_pair("y", picojson::value(y)));
  return picojson::value(obj);
}
}  // namespace colonio
