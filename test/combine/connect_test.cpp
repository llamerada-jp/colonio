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

TEST(ConnectTest, sendSingle) {
  const std::string URL      = "http://localhost:8080/test";
  const std::string TOKEN    = "";
  const std::string MAP_NAME = "map";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  std::string name = "node";
  std::unique_ptr<Colonio> node(Colonio::new_instance([&](Colonio&, const std::string& message) {
    std::cout << name << ":" << message << std::endl;
  }));

  printf("connect node\n");
  node->connect(URL, TOKEN);

  printf("node: %s\n", node->get_local_nid().c_str());

  node->on_call("nearby", [&](Colonio& c, const Colonio::CallParameter& parameter) {
    printf("receive nearby\n");
    EXPECT_EQ(parameter.options, Colonio::CALL_ACCEPT_NEARBY);
    EXPECT_STREQ(parameter.name.c_str(), "nearby");
    EXPECT_STREQ(parameter.value.get<std::string>().c_str(), "data nearby");
    return Value("result nearby");
  });

  node->on_call("expect", [&](Colonio& c, const Colonio::CallParameter& parameter) {
    printf("receive expect\n");
    EXPECT_EQ(parameter.options, 0);
    EXPECT_STREQ(parameter.name.c_str(), "expect");
    EXPECT_STREQ(parameter.value.get<std::string>().c_str(), "data expect");
    return Value("result expect");
  });

  // todo: avoid block without CALL_IGNORE_REPLY option.
  node->call_by_nid(
      "00000000000000000000000000000000", "nearby", Value("data nearby"), Colonio::CALL_ACCEPT_NEARBY,
      [&](Colonio&, const Value& result) {
        printf("receive reply from nearby\n");
        EXPECT_STREQ(result.get<std::string>().c_str(), "result nearby");
        helper.pass_signal("a");
      },
      [&](Colonio&, const Error&) {
        ADD_FAILURE();
      });

  node->call_by_nid(
      node->get_local_nid(), "expect", Value("data expect"), 0,
      [&](Colonio&, const Value& result) {
        printf("receive reply from expect\n");
        EXPECT_STREQ(result.get<std::string>().c_str(), "result expect");
        helper.pass_signal("b");
      },
      [&](Colonio&, const Error&) {
        ADD_FAILURE();
      });

  helper.wait_signal("a");
  helper.wait_signal("b");

  printf("disconnect node\n");
  node->disconnect();
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
  std::unique_ptr<Colonio> node1(Colonio::new_instance([&](Colonio&, const std::string& message) {
    std::cout << name1 << ":" << message << std::endl;
  }));
  std::unique_ptr<Colonio> node2(Colonio::new_instance([&](Colonio&, const std::string& message) {
    std::cout << name2 << ":" << message << std::endl;
  }));

  printf("connect node1\n");
  node1->connect(URL, TOKEN);

  printf("connect node2\n");
  node2->connect(URL, TOKEN);

  printf("node1 : %s\n", node1->get_local_nid().c_str());
  printf("node2 : %s\n", node2->get_local_nid().c_str());

  node1->on_call("call1", [&](Colonio& c, const Colonio::CallParameter& parameter) {
    printf("receive dummy1\n");
    EXPECT_EQ(parameter.options, Colonio::CALL_ACCEPT_NEARBY);
    EXPECT_STREQ(parameter.name.c_str(), "call1");
    EXPECT_STREQ(parameter.value.get<std::string>().c_str(), "dummy1");
    helper.mark("1");
    helper.pass_signal("1");

    printf("send result1\n");
    return Value("result1");
  });

  node1->on_call("call3", [&](Colonio& c, const Colonio::CallParameter& parameter) {
    printf("receive dummy3\n");
    helper.mark("3");
    helper.pass_signal("3");

    return Value();
  });

  node2->on_call("call2", [&](Colonio& c, const Colonio::CallParameter& parameter) {
    printf("receive dummy2\n");
    EXPECT_EQ(parameter.options, Colonio::CALL_IGNORE_REPLY);
    EXPECT_STREQ(parameter.name.c_str(), "call2");
    EXPECT_STREQ(parameter.value.get<std::string>().c_str(), "dummy2");
    helper.mark("2");
    helper.pass_signal("2");

    printf("send dummy2\n");
    // todo: avoid block without CALL_IGNORE_REPLY option.
    c.call_by_nid(
        node1->get_local_nid(), "call3", Value(), 0,
        [&](Colonio&, const Value&) {
          helper.mark("4");
          helper.pass_signal("4");
        },
        [&](Colonio&, const Error&) {
          ADD_FAILURE();
        });

    printf("send result2\n");
    return Value("result2");
  });

  NodeID target =
      NodeID::center_mod(NodeID::from_str(node1->get_local_nid()), NodeID::from_str(node2->get_local_nid()));
  helper.wait_signal("1", [&] {
    printf("send dummy1\n");
    Value result1 = node1->call_by_nid(target.to_str(), "call1", Value("dummy1"), Colonio::CALL_ACCEPT_NEARBY);
    EXPECT_STREQ(result1.get<std::string>().c_str(), "result1");
    printf("send dummy0\n");
    node2->call_by_nid(
        NodeID(0, 0).to_str(), "dummy", Value("dummy0"), 0,
        [&](Colonio&, const Value&) {
          ADD_FAILURE();
        },
        [&](Colonio&, const Error&) {
          helper.pass_signal("1e");
        });
    sleep(1);
  });

  helper.wait_signal("2", [&] {
    printf("send dummy2\n");
    node1->call_by_nid(
        node2->get_local_nid(), "call2", Value("dummy2"), Colonio::CALL_IGNORE_REPLY,
        [&](Colonio&, const Value& result) {
          EXPECT_EQ(result.get_type(), Value::NULL_T);
          helper.pass_signal("2s");
        },
        [&](Colonio&, const Error&) {
          ADD_FAILURE();
        });
  });

  helper.wait_signal("1e");
  helper.wait_signal("2s");
  helper.wait_signal("3");
  helper.wait_signal("4");

  EXPECT_THAT(helper.get_route(), MatchesRegex("^1234$"));

  printf("disconnect node2\n");
  node2->disconnect();

  printf("disconnect node1\n");
  node1->disconnect();
}