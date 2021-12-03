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
#include "coord_system_sphere.hpp"

#include <cmath>
#include <limits>

#include "random.hpp"
#include "utils.hpp"

namespace colonio {
CoordSystemSphere::CoordSystemSphere(Random& random, const picojson::object& config) :
    CoordSystem(M_PI * -1.0, M_PI * -0.5, M_PI * 1.0, M_PI * 0.5, 0.0001 / 60.0),
    conf_radius(0),
    local_position(
        (2.0 * M_PI * static_cast<double>(random.generate_u32()) / static_cast<double>(UINT32_MAX)) - M_PI * 1.0,
        (1.0 * M_PI * static_cast<double>(random.generate_u32()) / static_cast<double>(UINT32_MAX)) - M_PI * 0.5) {
  conf_radius = Utils::get_json<double>(config, "radius");
}

double CoordSystemSphere::get_distance(const Coordinate& p1, const Coordinate& p2) const {
  // spherical trigonometry (球面三角法)
  double avr_x = (p1.x - p2.x) / 2;
  double avr_y = (p1.y - p2.y) / 2;
  return conf_radius * 2 *
         std::asin(
             std::sqrt(std::pow(std::sin(avr_y), 2) + std::cos(p1.y) * std::cos(p2.y) * std::pow(std::sin(avr_x), 2)));
}

Coordinate CoordSystemSphere::get_local_position() const {
  return local_position;
}

void CoordSystemSphere::set_local_position(const Coordinate& position) {
  if (position.x < MIN_X || MAX_X <= position.x) {
    colonio_throw_error(
        ErrorCode::CONFLICT_WITH_SETTING, "The specified X coordinate is out of range (x:%f, min:%f, max:%f)",
        position.x, MIN_X, MAX_X);
  }
  if (position.y < MIN_Y || MAX_Y <= position.y) {
    colonio_throw_error(
        ErrorCode::CONFLICT_WITH_SETTING, "The specified Y coordinate is out of range (y:%f, min:%f, max:%f)",
        position.y, MIN_Y, MAX_Y);
  }
  local_position = position;
}

Coordinate CoordSystemSphere::shift_for_routing_2d(const Coordinate& base, const Coordinate& position) const {
  double rad = std::atan2(
      std::sin(position.x - base.x),
      std::cos(base.y) * std::tan(position.y) - std::sin(base.y) * std::cos(position.x - base.x));
  double d = get_distance(base, position);
  Coordinate ans(d * std::cos(rad), d * std::sin(rad));

  return ans;
}
}  // namespace colonio
