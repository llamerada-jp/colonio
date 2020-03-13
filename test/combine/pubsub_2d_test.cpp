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

#include <cmath>

#include "test_utils/all.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

double d2r(double d) {
  return M_PI * d / 180.0;
}

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

  try {
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

    printf("set position 1 and 2\n");
    node1.set_position(d2r(100), d2r(50));
    node2.set_position(d2r(50), d2r(50));
    sleep(3);  // wait for deffusing new position

    printf("publish a\n");
    ps1.publish("key1", d2r(50), d2r(50), 10, Value("a"));  // 21a
    printf("publish b\n");
    ps2.publish("key2", d2r(50), d2r(50), 10, Value("b"));  // none

    printf("set position 2\n");
    node2.set_position(d2r(-20), d2r(10));
    sleep(3);  // wait for deffusing new position

    printf("publish c\n");
    ps1.publish("key1", d2r(50), d2r(50), 10, Value("c"));  // none
    printf("publish d\n");
    ps1.publish("key1", d2r(-20), d2r(10), 10, Value("d"));  // 21d
    printf("publish e\n");
    ps1.publish("key2", d2r(-20), d2r(10), 10, Value("e"));  // 22e

  } catch (colonio::Exception& ex) {
    printf("exception code:%d: %s\n", static_cast<uint32_t>(ex.code), ex.message.c_str());
    ADD_FAILURE();
  }

  // disconnect
  printf("disconnect\n");
  node1.disconnect();
  node2.disconnect();

  EXPECT_THAT(helper.get_route(), MatchesRegex("^21a21d22e$"));
}
