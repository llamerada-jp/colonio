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
      [&](colonio::Colonio& c) {
        helper.pass_signal("connect");
      },
      [&](colonio::Colonio& c, const colonio::Error& err) {
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

TEST(ConnectTest, send) {
  const std::string URL      = "http://localhost:8080/test";
  const std::string TOKEN    = "";
  const std::string MAP_NAME = "map";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  std::string name1 = "node1";
  std::string name2 = "node2";
  std::unique_ptr<Colonio> node1(Colonio::new_instance([&](colonio::Colonio&, const std::string& message) {
    std::cout << name1 << ":" << message << std::endl;
  }));
  std::unique_ptr<Colonio> node2(Colonio::new_instance([&](colonio::Colonio&, const std::string& message) {
    std::cout << name2 << ":" << message << std::endl;
  }));

  printf("connect node1\n");
  node1->connect(URL, TOKEN);

  printf("connect node2\n");
  node2->connect(URL, TOKEN);

  printf("node1 : %s\n", node1->get_local_nid().c_str());
  printf("node2 : %s\n", node2->get_local_nid().c_str());

  node1->on([&](colonio::Colonio& c, const colonio::Value& value) {
    printf("receive dummy1\n");
    EXPECT_STREQ(value.get<std::string>().c_str(), "dummy1");
    helper.mark("1");
    helper.pass_signal("1");
    c.send(
        node2->get_local_nid(), colonio::Value("dummy2"), colonio::Colonio::CONFIRM_RECEIVER_RESULT,
        [&](colonio::Colonio&) {
          helper.mark("2");
          helper.pass_signal("2");
        },
        [&](colonio::Colonio&, const colonio::Error&) {
          ADD_FAILURE();
        });
  });

  node2->on([&](colonio::Colonio& c, const colonio::Value& value) {
    printf("receive dummy2\n");
    EXPECT_STREQ(value.get<std::string>().c_str(), "dummy2");
    helper.mark("3");
    helper.pass_signal("3");
  });

  NodeID target = NodeID::center_mod(NodeID::from_str(node1->get_local_nid()), NodeID::from_str(node2->get_local_nid()));
  helper.wait_signal("1", [&] {
    printf("send dummy1\n");
    node1->send(target.to_str(), colonio::Value("dummy1"), Colonio::SEND_ACCEPT_NEARBY);
    printf("send dummy0\n");
    node2->send(NodeID(0, 0).to_str(), colonio::Value("dummy0"), 0);
    sleep(1);
  });

  helper.wait_signal("2");
  helper.wait_signal("3");

  EXPECT_THAT(helper.get_route(), MatchesRegex("^123|132$"));

  printf("disconnect node2\n");
  node2->disconnect();

  printf("disconnect node1\n");
  node1->disconnect();
}