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

#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "colonio/colonio_libuv.hpp"
#include "test_utils/async_helper.hpp"
#include "test_utils/test_seed.hpp"

using namespace colonio;
using namespace colonio_helper;
using ::testing::MatchesRegex;

TEST(ConnectTest, connect_single) {
  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  ColonioLibuv node(helper.get_libuv_instance());

  node.connect(
      "http://localhost:8080/test", "",
      [&](Colonio& c) {
        helper.mark("a");
        c.disconnect();
      },
      [&](Colonio& c) {
        FAIL();
        c.disconnect();
      });

  // node.run();
  helper.run();
  EXPECT_THAT(helper.get_route(), MatchesRegex("^a$"));
}

TEST(ConnectTest, connect_multi) {
  const std::string URL      = "http://localhost:8080/test";
  const std::string TOKEN    = "";
  const std::string MAP_NAME = "map";
  const std::string KEY_NAME = "key";
  const std::string VALUE    = "test value";

  AsyncHelper helper;
  TestSeed seed;
  seed.add_module_map_paxos(MAP_NAME, 256);
  seed.run();

  ColonioLibuv node1("node1", helper.get_libuv_instance());
  ColonioLibuv node2("node2", helper.get_libuv_instance());

  // connect node1
  printf("connect node1\n");
  node1.connect(
      URL, TOKEN,
      [&](Colonio& c1) {
        // connect node2
        printf("connect node2\n");
        node2.connect(
            URL, TOKEN,
            [&](Colonio& c2) {
              helper.mark("a");
              EXPECT_NE(&node1, &node2);
              EXPECT_NE(&c1, &c2);
              EXPECT_STRNE(node1.get_local_nid(), node2.get_local_nid());
              EXPECT_EQ(&node1, &c1);
              EXPECT_EQ(&node2, &c2);
              c2.disconnect();
              c1.disconnect();
            },
            [&](Colonio& c2) {
              ADD_FAILURE();
              c2.disconnect();
              c1.disconnect();
            });
      },
      [&](Colonio& c1) {
        ADD_FAILURE();
        c1.disconnect();
      });

  // node.run();
  helper.run();
  EXPECT_THAT(helper.get_route(), MatchesRegex("^a$"));
}
