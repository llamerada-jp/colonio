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

// Implement log output method to check.
class ColonioTest : public ColonioLibuv {
 public:
  ColonioTest(const std::string& node_name_, uv_loop_t* loop) : ColonioLibuv(loop), node_name(node_name_) {
  }

 protected:
  std::string node_name;

  void on_output_log(LogLevel::Type level, const std::string& message) override {
    // printf("%s %s\n", node_name.c_str(), message.c_str());
  }
};

// TEST(AccessTest, set_get) {
int main() {
  const std::string URL      = "http://localhost:8080/test";
  const std::string TOKEN    = "";
  const std::string MAP_NAME = "map";
  const std::string KEY_NAME = "key";
  const std::string VALUE    = "test value";

  AsyncHelper helper;
  TestSeed seed;
  seed.add_module_map_paxos(MAP_NAME, 256);
  seed.run();

  ColonioTest node1("node1", helper.get_libuv_instance());
  ColonioTest node2("node2", helper.get_libuv_instance());

  // connect node1
  printf("connect node1\n");
  node1.connect(
      URL, TOKEN,
      [&](Colonio& c1) {
        Map& map1 = c1.access_map(MAP_NAME);
        // connect node2
        printf("connect node2\n");
        node2.connect(
            URL, TOKEN,
            [&](Colonio& c2) {
              Map& map2 = c2.access_map(MAP_NAME);

              // get(key) @ node1
              printf("get a not existed value.\n");
              map1.get(
                  Value(KEY_NAME),
                  [&](const Value& v) {
                    FAIL();
                    c1.disconnect();
                    c2.disconnect();
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
                                c1.disconnect();
                                c2.disconnect();
                              },
                              [&](MapFailureReason reason) {
                                ADD_FAILURE();
                                c1.disconnect();
                                c2.disconnect();
                              });
                        },
                        [&](MapFailureReason reason) {
                          ADD_FAILURE();
                          c1.disconnect();
                          c2.disconnect();
                        });
                  });
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
