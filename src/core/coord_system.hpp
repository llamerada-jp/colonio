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

#include "coordinate.hpp"

namespace colonio {
class CoordSystem {
 public:
  const double MIN_X;
  const double MIN_Y;
  const double MAX_X;
  const double MAX_Y;
  const double PRECISION;

  virtual ~CoordSystem();
  CoordSystem(const CoordSystem&)            = delete;
  CoordSystem& operator=(const CoordSystem&) = delete;

  // for use delaunay triangle.
  virtual double get_distance(const Coordinate& p1, const Coordinate& p2) const                     = 0;
  virtual Coordinate get_local_position() const                                                     = 0;
  virtual void set_local_position(const Coordinate& position)                                       = 0;
  virtual Coordinate shift_for_routing_2d(const Coordinate& base, const Coordinate& position) const = 0;

 protected:
  CoordSystem(double min_x, double min_y, double max_x, double max_y, double precision);
};
}  // namespace colonio
