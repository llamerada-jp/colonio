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

TEST(MapTest, set_get_single) {
  const std::string URL      = "http://localhost:8080/test";
  const std::string TOKEN    = "";
  const std::string MAP_NAME = "map";
  const std::string KEY_NAME = "key";
  const std::string VALUE    = "test value";

  AsyncHelper helper;
  TestSeed seed;
  seed.add_module_map_paxos(MAP_NAME, 256);
  seed.run();

  ColonioNode node("node");

  // connect node
  printf("connect node1\n");
  node.connect(URL, TOKEN);
  Map& map = node.access_map(MAP_NAME);

  // get(key)
  printf("get a not existed value.\n");
  map.get(
      Value(KEY_NAME),
      [&](const Value& v) {
        FAIL();
        node.disconnect();
      },
      [&](MapFailureReason reason) {
        EXPECT_EQ(reason, MapFailureReason::NOT_EXIST_KEY);

        // set(key, val)
        printf("set a value.\n");
        map.set(
            Value(KEY_NAME), Value(VALUE),
            [&]() {
              // get(key)
              printf("get a existed value.\n");
              map.get(
                  Value(KEY_NAME),
                  [&](const Value& v) {
                    helper.mark("a");
                    EXPECT_EQ(v.get<std::string>(), VALUE);
                    node.disconnect();
                  },
                  [&](MapFailureReason reason) {
                    ADD_FAILURE();
                    node.disconnect();
                  });
            },
            [&](MapFailureReason reason) {
              ADD_FAILURE();
              node.disconnect();
            });
      });

  node.disconnect();
  EXPECT_THAT(helper.get_route(), MatchesRegex("^a$"));
}

TEST(MapTest, set_get_multi) {
  const std::string URL      = "http://localhost:8080/test";
  const std::string TOKEN    = "";
  const std::string MAP_NAME = "map";
  const std::string KEY_NAME = "key";
  const std::string VALUE    = "test value";

  AsyncHelper helper;
  TestSeed seed;
  seed.add_module_map_paxos(MAP_NAME, 256);
  seed.run();

  ColonioNode node1("node1");
  ColonioNode node2("node2");

  // connect node1
  printf("connect node1\n");
  node1.connect(URL, TOKEN);
  // connect node2
  printf("connect node2\n");
  node2.connect(URL, TOKEN);

  Map& map1 = node1.access_map(MAP_NAME);
  Map& map2 = node2.access_map(MAP_NAME);

  // get(key) @ node1
  printf("get a not existed value.\n");
  map1.get(
      Value(KEY_NAME),
      [&](const Value& v) {
        FAIL();
        node1.disconnect();
        node2.disconnect();
      },
      [&](MapFailureReason reason) {
        EXPECT_EQ(reason, MapFailureReason::NOT_EXIST_KEY);

        // set(key, val) @ node1
        printf("set a value.\n");
        map1.set(
            Value(KEY_NAME), Value(VALUE),
            [&]() {
              // get(key) @ node2
              printf("get a existed value.\n");
              map2.get(
                  Value(KEY_NAME),
                  [&](const Value& v2) {
                    helper.mark("a");
                    EXPECT_EQ(v2.get<std::string>(), VALUE);
                    node1.disconnect();
                    node2.disconnect();
                  },
                  [&](MapFailureReason reason) {
                    ADD_FAILURE();
                    node1.disconnect();
                    node2.disconnect();
                  });
            },
            [&](MapFailureReason reason) {
              ADD_FAILURE();
              node1.disconnect();
              node2.disconnect();
            });
      });

  node2.disconnect();
  node1.disconnect();

  EXPECT_THAT(helper.get_route(), MatchesRegex("^a$"));
}
