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

  // get(key) : not exist
  printf("get a not existed value.\n");
  try {
    map.get(Value(KEY_NAME));
    ADD_FAILURE();

  } catch (const Exception& e) {
    EXPECT_EQ(e.code, ErrorCode::NOT_EXIST_KEY);
    helper.mark("a");
  }
  // set(key, val)
  printf("set a value.\n");
  map.set(Value(KEY_NAME), Value(VALUE));

  // get(key)
  printf("get a existed value.\n");
  Value v = map.get(Value(KEY_NAME));

  EXPECT_EQ(v.get<std::string>(), VALUE);

  node.disconnect();
  EXPECT_THAT(helper.get_route(), MatchesRegex("^a$"));
}

TEST(MapTest, set_get_async) {
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
  node.connect(
      URL, TOKEN,
      [&node, &MAP_NAME, &KEY_NAME, &VALUE, &helper](colonio::Colonio& _) {
        Map& map = node.access_map(MAP_NAME);

        // get(key) : not exist
        printf("get a not existed value.\n");
        map.get(
            Value(KEY_NAME), [](colonio::Map& _, const colonio::Value& value) { ADD_FAILURE(); },
            [&KEY_NAME, &VALUE, &helper](colonio::Map& map, const colonio::Error& err) {
              EXPECT_EQ(err.code, ErrorCode::NOT_EXIST_KEY);
              // set(key, val)
              printf("set a value.\n");
              map.set(
                  Value(KEY_NAME), Value(VALUE), 0,
                  [&KEY_NAME, &VALUE, &helper](colonio::Map& map) {
                    // get(key)
                    printf("get a existed value.\n");
                    map.get(
                        Value(KEY_NAME),
                        [&VALUE, &helper](colonio::Map& _, const colonio::Value& value) {
                          EXPECT_EQ(value.get<std::string>(), VALUE);
                          helper.pass_signal("connect");
                        },
                        [](colonio::Map& _, const colonio::Error& err) {
                          std::cerr << err.message << std::endl;
                          ADD_FAILURE();
                        });
                  },
                  [](colonio::Map& _, const colonio::Error& err) {
                    std::cerr << err.message << std::endl;
                    ADD_FAILURE();
                  });
            });
      },
      [](colonio::Colonio& _1, const colonio::Error& err) {
        std::cerr << err.message << std::endl;
        ADD_FAILURE();
      });

  helper.wait_signal("connect");
  node.disconnect();
}

TEST(MapTest, set_get_multi) {
  const std::string URL       = "http://localhost:8080/test";
  const std::string TOKEN     = "";
  const std::string MAP_NAME  = "map";
  const std::string KEY_NAME1 = "key1";
  const std::string KEY_NAME2 = "key2";
  const std::string VALUE1    = "test value 1";
  const std::string VALUE2    = "test value 2";

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
  try {
    map1.get(Value(KEY_NAME1));
    ADD_FAILURE();

  } catch (const Exception& e) {
    EXPECT_EQ(e.code, ErrorCode::NOT_EXIST_KEY);
    helper.mark("a");
  }

  // get(key) @ node2
  printf("get a not existed value.\n");
  try {
    map2.get(Value(KEY_NAME1));
    ADD_FAILURE();

  } catch (const Exception& e) {
    EXPECT_EQ(e.code, ErrorCode::NOT_EXIST_KEY);
    helper.mark("b");
  }

  // set(key, val) @ node1
  printf("set a value.\n");
  map1.set(Value(KEY_NAME1), Value(VALUE1));

  // get(key) @ node2
  printf("get a existed value.\n");
  Value v1 = map2.get(Value(KEY_NAME1));
  EXPECT_EQ(v1.get<std::string>(), VALUE1);

  // get(key) @ node1
  printf("get a existed value.\n");
  Value v2 = map1.get(Value(KEY_NAME1));
  EXPECT_EQ(v2.get<std::string>(), VALUE1);

  // overwrite value
  printf("overwrite value.\n");
  map1.set(Value(KEY_NAME1), Value(VALUE2));
  bool pass = false;
  for (int i = 0; i < 10; i++) {
    Value v2 = map2.get(Value(KEY_NAME1));
    if (v2.get<std::string>() == VALUE2) {
      pass = true;
      break;
    } else {
      sleep(1);
    }
  }
  EXPECT_TRUE(pass);

  // set value with exist check
  printf("set value with exist check.\n");
  try {
    map1.set(Value(KEY_NAME2), Value(VALUE1), Map::ERROR_WITH_EXIST);
  } catch (const Exception& e) {
    printf("%d %s\n", e.code, e.message.c_str());
    ADD_FAILURE();
  }

  // overwrite with exist check
  printf("overwrite value with exist check.\n");
  try {
    map2.set(Value(KEY_NAME2), Value(VALUE2), Map::ERROR_WITH_EXIST);
    ADD_FAILURE();

  } catch (const Exception& e) {
    EXPECT_EQ(e.code, ErrorCode::EXIST_KEY);
    helper.mark("c");
  }

  printf("disconnect.\n");
  node2.disconnect();
  node1.disconnect();

  EXPECT_THAT(helper.get_route(), MatchesRegex("^abc$"));
}
