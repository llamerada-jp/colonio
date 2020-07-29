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

#include <picojson.h>

#include "coord_system.hpp"

namespace colonio {
class CoordSystemPlane : public CoordSystem {
 public:
  CoordSystemPlane(const picojson::object& config);

  // CoordSystem
  double get_distance(const Coordinate& p1, const Coordinate& p2) override;
  Coordinate get_local_position() override;
  void set_local_position(const Coordinate& position) override;
  Coordinate shift_for_routing_2d(const Coordinate& base, const Coordinate& position) override;

 private:
  Coordinate local_position;
};
}  // namespace colonio
