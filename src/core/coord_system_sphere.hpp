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

#include "coord_system.hpp"

namespace colonio {
class CoordSystemSphere : public CoordSystem {
 public:
  CoordSystemSphere(const picojson::object& config);

  // CoordSystem
  double get_distance(const Coordinate& p1, const Coordinate& p2) override;
  Coordinate get_my_position() override;
  void set_my_position(const Coordinate& position) override;
  Coordinate shift_for_routing_2d(const Coordinate& base, const Coordinate& position) override;

 private:
  double conf_radius;

  Coordinate my_position;
};
}  // namespace colonio
