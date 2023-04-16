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
#include "core/node_id.hpp"
#include "test_utils/all.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

const std::string SEED_URL("https://localhost:8080/test");

TEST(ConnectTest, connect_single) {
  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config = make_config_with_name("node");
  std::unique_ptr<Colonio> node(Colonio::new_instance(config));

  node->connect(SEED_URL, "");
  node->disconnect();
}

TEST(ConnectTest, connect_async) {
  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config = make_config_with_name("node");
  std::unique_ptr<Colonio> node(Colonio::new_instance(config));

  node->connect(
      SEED_URL, "",
      [&](Colonio& c) {
        helper.pass_signal("connect");
      },
      [&](Colonio& c, const Error& err) {
        ADD_FAILURE();
      });

  helper.wait_signal("connect");
  node->disconnect();
}

TEST(ConnectTest, connect_error) {
  AsyncHelper helper;

  auto config = make_config_with_name("node");
  std::unique_ptr<Colonio> node(Colonio::new_instance(config));

  ASSERT_THROW(node->connect("https://localhost:8080/not_exist", ""), Error);
  ASSERT_THROW(node->disconnect(), Error);

  node->connect(
      "https://localhost:8080/not_exist", "",
      [&](Colonio& c) {
        ADD_FAILURE();
      },
      [&](Colonio& c, const Error& err) {
        helper.pass_signal("connect failure");
      });
  helper.wait_signal("connect failure");

  node->disconnect(
      [&](Colonio& c) {
        ADD_FAILURE();
      },
      [&](Colonio& c, const Error& err) {
        helper.pass_signal("disconnect failure");
      });
  helper.wait_signal("disconnect failure");
}

TEST(ConnectTest, connect_multi) {
  const std::string URL      = SEED_URL;
  const std::string TOKEN    = "";
  const std::string MAP_NAME = "map";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config1 = make_config_with_name("node1");
  std::unique_ptr<Colonio> node1(Colonio::new_instance(config1));
  auto config2 = make_config_with_name("node2");
  std::unique_ptr<Colonio> node2(Colonio::new_instance(config2));

  // before connect
  EXPECT_FALSE(node1->is_connected());
  EXPECT_STREQ(node1->get_local_nid().c_str(), "");

  // connect node1
  printf("connect node1\n");
  node1->connect(URL, TOKEN);
  EXPECT_TRUE(node1->is_connected());

  // connect node2
  printf("connect node2\n");
  node2->connect(URL, TOKEN);

  EXPECT_NE(&node1, &node2);
  EXPECT_STRNE(node1->get_local_nid().c_str(), node2->get_local_nid().c_str());

  // disconnect node 2
  printf("disconnect node2\n");
  node2->disconnect();

  // disconnect node 1
  printf("disconnect node1\n");
  node1->disconnect();
}