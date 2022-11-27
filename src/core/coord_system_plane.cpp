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
#include "coord_system_plane.hpp"

#include <cmath>
#include <limits>

#include "random.hpp"
#include "utils.hpp"

namespace colonio {
CoordSystemPlane::CoordSystemPlane(Random& random, const picojson::object& config) :
    CoordSystem(
        Utils::get_json<double>(config, "xMin"), Utils::get_json<double>(config, "yMin"),
        Utils::get_json<double>(config, "xMax"), Utils::get_json<double>(config, "yMax"),
        (Utils::get_json<double>(config, "xMax") - Utils::get_json<double>(config, "xMin")) / UINT32_MAX),
    local_position(Coordinate(random.generate_double(MIN_X, MAX_X), random.generate_double(MIN_Y, MAX_Y))) {
}

double CoordSystemPlane::get_distance(const Coordinate& p1, const Coordinate& p2) const {
  return std::sqrt(std::pow(p1.x - p2.x, 2) + std::pow(p1.y - p2.y, 2));
}

Coordinate CoordSystemPlane::get_local_position() const {
  return local_position;
}

void CoordSystemPlane::set_local_position(const Coordinate& position) {
  if (position.x < MIN_X || MAX_X <= position.x) {
    colonio_throw_error(
        ErrorCode::SYSTEM_CONFLICT_WITH_SETTING, "The specified X coordinate is out of range (x:%f, min:%f, max:%f)",
        position.x, MIN_X, MAX_X);
  }
  if (position.y < MIN_Y || MAX_Y <= position.y) {
    colonio_throw_error(
        ErrorCode::SYSTEM_CONFLICT_WITH_SETTING, "The specified Y coordinate is out of range (y:%f, min:%f, max:%f)",
        position.y, MIN_Y, MAX_Y);
  }
  local_position = position;
}

Coordinate CoordSystemPlane::shift_for_routing_2d(const Coordinate& base, const Coordinate& position) const {
  return Coordinate(position.x - base.x, position.y - base.y);
}
}  // namespace colonio
