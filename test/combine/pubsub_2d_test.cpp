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

#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "test_utils/all.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

TEST(PubSub2DTest, multi_node) {
  const std::string URL           = "http://localhost:8080/test";
  const std::string TOKEN         = "";
  const std::string PUBSUB2D_NAME = "ps2";

  AsyncHelper helper;
  TestSeed seed;
  seed.set_coord_system_sphere();
  seed.add_module_pubsub_2d(PUBSUB2D_NAME, 256);
  seed.run();

  ColonioNode node1("node1");
  ColonioNode node2("node2");

  // connect node1;
  printf("connect node1\n");
  node1.connect(URL, TOKEN);
  PubSub2D& ps1 = node1.access_pubsub2d(PUBSUB2D_NAME);
  ps1.on("key1", [&helper](const Value& v) {
    helper.mark("11");
    helper.mark(v.get<std::string>());
  });
  ps1.on("key2", [&helper](const Value& v) {
    helper.mark("12");
    helper.mark(v.get<std::string>());
  });

  // connect node2;
  printf("connect node2\n");
  node2.connect(URL, TOKEN);
  PubSub2D& ps2 = node2.access_pubsub2d(PUBSUB2D_NAME);
  ps2.on("key1", [&helper](const Value& v) {
    helper.mark("21");
    helper.mark(v.get<std::string>());
  });
  ps2.on("key2", [&helper](const Value& v) {
    helper.mark("22");
    helper.mark(v.get<std::string>());
  });

  node1.set_position(100, 50);
  node2.set_position(50, 50);

  ps1.publish("key1", 50, 50, 10, Value("a"));  // 21a
  ps2.publish("key2", 50, 50, 10, Value("b"));  // none

  node2.set_position(-20, 10);
  ps1.publish("key1", 50, 50, 10, Value("c"));   // none
  ps1.publish("key1", -20, 10, 10, Value("d"));  // 21d
  ps1.publish("key2", -20, 10, 10, Value("e"));  // 22e

  // disconnect
  printf("disconnect\n");
  node1.disconnect();
  node2.disconnect();

  EXPECT_THAT(helper.get_route(), MatchesRegex("^21a21d22e$"));
}
