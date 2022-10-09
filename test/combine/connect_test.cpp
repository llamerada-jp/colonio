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

TEST(ConnectTest, connect_single) {
  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  std::unique_ptr<Colonio> node(Colonio::new_instance(log_receiver("node")));

  node->connect("http://localhost:8080/test", "");
  node->disconnect();
}

TEST(ConnectTest, connect_async) {
  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  std::unique_ptr<Colonio> node(Colonio::new_instance(log_receiver("node")));

  node->connect(
      "http://localhost:8080/test", "",
      [&](Colonio& c) {
        helper.pass_signal("connect");
      },
      [&](Colonio& c, const Error& err) {
        ADD_FAILURE();
      });

  helper.wait_signal("connect");
  node->disconnect();
}

TEST(ConnectTest, connect_multi) {
  const std::string URL      = "http://localhost:8080/test";
  const std::string TOKEN    = "";
  const std::string MAP_NAME = "map";

  AsyncHelper helper;
  TestSeed seed;
  seed.add_module_map_paxos(MAP_NAME, 256);
  seed.run();

  std::unique_ptr<Colonio> node1(Colonio::new_instance(log_receiver("node1")));
  std::unique_ptr<Colonio> node2(Colonio::new_instance(log_receiver("node2")));

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