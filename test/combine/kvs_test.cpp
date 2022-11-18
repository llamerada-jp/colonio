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

#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "colonio/colonio.hpp"
#include "test_utils/all.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

TEST(MapTest, set_get_single) {
  const std::string URL   = "http://localhost:8080/test";
  const std::string TOKEN = "";
  const std::string KEY   = "key";
  const std::string VALUE = "test value";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config = make_config_with_name("node");
  std::unique_ptr<Colonio> node(Colonio::new_instance(config));

  // connect node
  printf("connect node\n");
  node->connect(URL, TOKEN);
  // get(key) : not exist
  printf("get a not existed value.\n");
  try {
    node->kvs_get(KEY);
    ADD_FAILURE();

  } catch (const Error& e) {
    EXPECT_EQ(e.code, ErrorCode::KVS_NOT_FOUND);
    helper.mark("a");
  }
  // set(key, val)
  printf("set a value.\n");
  node->kvs_set(KEY, Value(VALUE));

  // get(key)
  printf("get a existed value.\n");
  Value v = node->kvs_get(KEY);

  EXPECT_EQ(v.get<std::string>(), VALUE);

  node->disconnect();
  EXPECT_THAT(helper.get_route(), MatchesRegex("^a$"));
}

TEST(MapTest, set_get_async) {
  const std::string URL   = "http://localhost:8080/test";
  const std::string TOKEN = "";
  const std::string KEY   = "key";
  const std::string VALUE = "test value";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config = make_config_with_name("node");
  std::unique_ptr<Colonio> node(Colonio::new_instance(config));

  // connect node
  printf("connect node1\n");
  node->connect(
      URL, TOKEN,
      [&](colonio::Colonio& _) {
        // get(key) : not exist
        printf("get a not existed value.\n");
        node->kvs_get(
            KEY,
            [](colonio::Colonio& _, const colonio::Value& value) {
              ADD_FAILURE();
            },
            [&](colonio::Colonio& _, const colonio::Error& err) {
              EXPECT_EQ(err.code, ErrorCode::KVS_NOT_FOUND);
              // set(key, val)
              printf("set a value.\n");
              node->kvs_set(
                  KEY, Value(VALUE), 0,
                  [&](colonio::Colonio& _) {
                    // get(key)
                    printf("get a existed value.\n");
                    node->kvs_get(
                        KEY,
                        [&VALUE, &helper](colonio::Colonio& _, const colonio::Value& value) {
                          EXPECT_EQ(value.get<std::string>(), VALUE);
                          helper.pass_signal("connect");
                        },
                        [](colonio::Colonio& _, const colonio::Error& err) {
                          std::cerr << err.message << std::endl;
                          ADD_FAILURE();
                        });
                  },
                  [](colonio::Colonio& _, const colonio::Error& err) {
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
  node->disconnect();
}

TEST(MapTest, set_get_multi) {
  const std::string URL    = "http://localhost:8080/test";
  const std::string TOKEN  = "";
  const std::string KEY1   = "key1";
  const std::string KEY2   = "key2";
  const std::string VALUE1 = "test value 1";
  const std::string VALUE2 = "test value 2";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config1 = make_config_with_name("node1");
  std::unique_ptr<Colonio> node1(Colonio::new_instance(config1));
  auto config2 = make_config_with_name("node2");
  std::unique_ptr<Colonio> node2(Colonio::new_instance(config2));

  // connect node1
  printf("connect node1\n");
  node1->connect(URL, TOKEN);
  // connect node2
  printf("connect node2\n");
  node2->connect(URL, TOKEN);

  // get(key) @ node1
  printf("get a not existed value.\n");
  try {
    node1->kvs_get(KEY1);
    ADD_FAILURE();

  } catch (const Error& e) {
    EXPECT_EQ(e.code, ErrorCode::KVS_NOT_FOUND);
    helper.mark("a");
  }

  // get(key) @ node2
  printf("get a not existed value.\n");
  try {
    node2->kvs_get(KEY1);
    ADD_FAILURE();

  } catch (const Error& e) {
    EXPECT_EQ(e.code, ErrorCode::KVS_NOT_FOUND);
    helper.mark("b");
  }

  // set(key, val) @ node1
  printf("set a value.\n");
  node1->kvs_set(KEY1, Value(VALUE1));

  // get(key) @ node2
  printf("get a existed value.\n");
  Value v1 = node2->kvs_get(KEY1);
  EXPECT_EQ(v1.get<std::string>(), VALUE1);

  // get(key) @ node1
  printf("get a existed value.\n");
  Value v2 = node1->kvs_get(KEY1);
  EXPECT_EQ(v2.get<std::string>(), VALUE1);

  // overwrite value
  printf("overwrite value.\n");
  node1->kvs_set(KEY1, Value(VALUE2));
  bool pass = false;
  for (int i = 0; i < 10; i++) {
    Value v2 = node2->kvs_get(KEY1);
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
    node1->kvs_set(KEY2, Value(VALUE1), Colonio::KVS_PROHIBIT_OVERWRITE);
  } catch (const Error& e) {
    printf("%d %s\n", static_cast<int>(e.code), e.message.c_str());
    ADD_FAILURE();
  }

  // overwrite with exist check
  printf("overwrite value with exist check.\n");
  try {
    node2->kvs_set(KEY2, Value(VALUE2), Colonio::KVS_PROHIBIT_OVERWRITE);
    ADD_FAILURE();

  } catch (const Error& e) {
    EXPECT_EQ(e.code, ErrorCode::KVS_PROHIBIT_OVERWRITE);
    helper.mark("c");
  }

  std::map<std::string, std::string> stored;
  auto data1 = node1->kvs_get_local_data();
  for (auto& datum : *data1) {
    EXPECT_EQ(stored.find(datum.first), stored.end());
    stored.insert(std::make_pair(datum.first, datum.second.get<std::string>()));
  }
  auto data2 = node2->kvs_get_local_data();
  for (auto& datum : *data2) {
    EXPECT_EQ(stored.find(datum.first), stored.end());
    stored.insert(std::make_pair(datum.first, datum.second.get<std::string>()));
  }

  EXPECT_EQ(stored.size(), static_cast<std::size_t>(2));
  EXPECT_EQ(stored[KEY1], VALUE2);
  EXPECT_EQ(stored[KEY2], VALUE1);

  printf("disconnect.\n");
  node2->disconnect();
  node1->disconnect();

  EXPECT_THAT(helper.get_route(), MatchesRegex("^abc$"));
}
