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
#include "coord_system_sphere.hpp"

#include <cassert>
#include <cmath>
#include <limits>

#include "context.hpp"
#include "utils.hpp"

namespace colonio {
CoordSystemSphere::CoordSystemSphere(const picojson::object& config) :
    CoordSystem(M_PI * -1.0, M_PI * -0.5, M_PI * 1.0, M_PI * 0.5, 0.0001 / 60.0),
    conf_radius(0),
    my_position(
        (2.0 * M_PI * static_cast<double>(Context::get_rnd_32()) / static_cast<double>(UINT32_MAX)) - M_PI * 1.0,
        (1.0 * M_PI * static_cast<double>(Context::get_rnd_32()) / static_cast<double>(UINT32_MAX)) - M_PI * 0.5) {
  conf_radius = Utils::get_json<double>(config, "radius");
}

double CoordSystemSphere::get_distance(const Coordinate& p1, const Coordinate& p2) {
  // spherical trigonometry (球面三角法)
  double avr_x = (p1.x - p2.x) / 2;
  double avr_y = (p1.y - p2.y) / 2;
  return conf_radius * 2 *
         std::asin(
             std::sqrt(std::pow(std::sin(avr_y), 2) + std::cos(p1.y) * std::cos(p2.y) * std::pow(std::sin(avr_x), 2)));
}

Coordinate CoordSystemSphere::get_my_position() {
  return my_position;
}

void CoordSystemSphere::set_my_position(const Coordinate& position) {
  assert(MIN_X <= position.x && position.x < MAX_X);
  assert(MIN_Y <= position.y && position.y < MAX_Y);
  my_position = position;
}

Coordinate CoordSystemSphere::shift_for_routing_2d(const Coordinate& base, const Coordinate& position) {
  Coordinate ans(position.x - base.x, position.y - base.y);
  ans.x = Utils::pmod(ans.x + M_PI, M_PI * 2.0) - M_PI;
  ans.y = Utils::pmod(ans.y + M_PI * 0.5, M_PI) - M_PI * 0.5;

  assert(MIN_X <= ans.x && ans.x < MAX_X && MIN_Y <= ans.y && ans.y < MAX_Y);

  return ans;
}
}  // namespace colonio
