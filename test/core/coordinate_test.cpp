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
#include "core/coordinate.hpp"

#include <gtest/gtest.h>

#include "core/colonio.pb.h"

using namespace colonio;

TEST(Coordinate, test) {
  Coordinate empty;
  EXPECT_FALSE(empty.is_enable());

  Coordinate src(40.96, 102.4);
  EXPECT_TRUE(src.is_enable());

  picojson::object js = src.to_json().get<picojson::object>();
  EXPECT_EQ(js.at("x").get<double>(), 40.96);
  EXPECT_EQ(js.at("y").get<double>(), 102.4);

  proto::Coordinate pb;
  src.to_pb(&pb);
  Coordinate dst = Coordinate::from_pb(pb);
  EXPECT_FALSE(src != dst);
  EXPECT_FALSE(src < dst);

  dst.y += 1;
  EXPECT_TRUE(src < dst);

  src.x += 1;
  EXPECT_FALSE(src < dst);
}