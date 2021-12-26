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

#include "core/coord_system_plane.hpp"

#include <gtest/gtest.h>

#include "core/random.hpp"

using namespace colonio;

TEST(CoordSystemPlaneTest, test) {
  std::string config = "{\"type\":\"plane\", \"xMin\":-1.0, \"xMax\":1.0, \"yMin\":-2.0, \"yMax\":2.0 }";
  picojson::value v;
  std::string err = picojson::parse(v, config);
  if (!err.empty()) {
    std::cerr << err << std::endl;
    FAIL();
  }
  Random random;
  CoordSystemPlane plane(random, v.get<picojson::object>());

  EXPECT_FLOAT_EQ(plane.MIN_X, -1.0);
  EXPECT_FLOAT_EQ(plane.MAX_X, 1.0);
  EXPECT_FLOAT_EQ(plane.MIN_Y, -2.0);
  EXPECT_FLOAT_EQ(plane.MAX_Y, 2.0);

  EXPECT_FLOAT_EQ(plane.get_distance(Coordinate(-0.5, 0.5), Coordinate(0.5, -0.5)), 1.41421356237);

  plane.set_local_position(Coordinate(0.5, -1.0));
  Coordinate pos1 = plane.get_local_position();
  EXPECT_FLOAT_EQ(pos1.x, 0.5);
  EXPECT_FLOAT_EQ(pos1.y, -1.0);

  Coordinate pos2 = plane.shift_for_routing_2d(Coordinate(0.5, 0.5), Coordinate(-0.5, 1.0));
  EXPECT_FLOAT_EQ(pos2.x, -1.0);
  EXPECT_FLOAT_EQ(pos2.y, 0.5);
}
