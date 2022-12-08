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

TEST(MessagingTest, callExpect) {
  const std::string URL   = "http://localhost:8080/test";
  const std::string TOKEN = "";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config = make_config_with_name("node");
  std::unique_ptr<Colonio> node(Colonio::new_instance(config));

  printf("connect node\n");
  node->connect(URL, TOKEN);

  printf("node: %s\n", node->get_local_nid().c_str());

  node->messaging_set_handler(
      "expect", [&](Colonio& c, const Colonio::MessagingRequest& message,
                    std::shared_ptr<Colonio::MessagingResponseWriter> writer) {
        printf("receive expect\n");
        EXPECT_EQ(message.options, static_cast<const unsigned int>(0));
        EXPECT_STREQ(message.message.get<std::string>().c_str(), "data expect");
        writer->write(Value("result expect"));
      });

  node->messaging_post(
      node->get_local_nid(), "expect", Value("data expect"), 0,
      [&](Colonio&, const Value& result) {
        printf("receive reply from expect\n");
        EXPECT_STREQ(result.get<std::string>().c_str(), "result expect");
        helper.pass_signal("a");
      },
      [&](Colonio&, const Error&) {
        ADD_FAILURE();
      });

  helper.wait_signal("a");

  printf("disconnect node\n");
  node->disconnect();
}

TEST(MessagingTest, callNearby) {
  const std::string URL   = "http://localhost:8080/test";
  const std::string TOKEN = "";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config = make_config_with_name("node");
  std::unique_ptr<Colonio> node(Colonio::new_instance(config));

  printf("connect node\n");
  node->connect(URL, TOKEN);

  printf("node: %s\n", node->get_local_nid().c_str());

  node->messaging_set_handler(
      "nearby", [&](Colonio& c, const Colonio::MessagingRequest& message,
                    std::shared_ptr<Colonio::MessagingResponseWriter> writer) {
        printf("receive nearby\n");
        EXPECT_EQ(message.options, Colonio::MESSAGING_ACCEPT_NEARBY);
        EXPECT_STREQ(message.message.get<std::string>().c_str(), "data nearby");
        writer->write(Value("result nearby"));
      });

  // todo: avoid block without MESSAGING_IGNORE_RESPONSE option.
  node->messaging_post(
      "00000000000000000000000000000000", "nearby", Value("data nearby"), Colonio::MESSAGING_ACCEPT_NEARBY,
      [&](Colonio&, const Value& result) {
        printf("receive reply from nearby\n");
        EXPECT_STREQ(result.get<std::string>().c_str(), "result nearby");
        helper.pass_signal("a");
      },
      [&](Colonio&, const Error&) {
        ADD_FAILURE();
      });

  helper.wait_signal("a");

  printf("disconnect node\n");
  node->disconnect();
}

TEST(MessagingTest, send) {
  const std::string URL   = "http://localhost:8080/test";
  const std::string TOKEN = "";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config1 = make_config_with_name("node1");
  std::unique_ptr<Colonio> node1(Colonio::new_instance(config1));
  auto config2 = make_config_with_name("node2");
  std::unique_ptr<Colonio> node2(Colonio::new_instance(config2));

  printf("connect node1\n");
  node1->connect(URL, TOKEN);

  printf("connect node2\n");
  node2->connect(URL, TOKEN);

  printf("node1 : %s\n", node1->get_local_nid().c_str());
  printf("node2 : %s\n", node2->get_local_nid().c_str());

  node1->messaging_set_handler(
      "call1", [&](Colonio& c, const Colonio::MessagingRequest& message,
                   std::shared_ptr<Colonio::MessagingResponseWriter> writer) {
        printf("receive dummy1\n");
        EXPECT_EQ(message.options, Colonio::MESSAGING_ACCEPT_NEARBY);
        EXPECT_STREQ(message.message.get<std::string>().c_str(), "dummy1");
        helper.mark("1");
        helper.pass_signal("1");

        printf("send result1\n");
        writer->write(Value("result1"));
      });

  node1->messaging_set_handler(
      "call3", [&](Colonio& c, const Colonio::MessagingRequest& message,
                   std::shared_ptr<Colonio::MessagingResponseWriter> writer) {
        printf("receive dummy3\n");
        helper.mark("3");
        helper.pass_signal("3");

        writer->write(Value());
      });

  node2->messaging_set_handler(
      "call2", [&](Colonio& c, const Colonio::MessagingRequest& message,
                   std::shared_ptr<Colonio::MessagingResponseWriter> writer) {
        printf("receive dummy2\n");
        EXPECT_EQ(message.options, Colonio::MESSAGING_IGNORE_RESPONSE);
        EXPECT_STREQ(message.message.get<std::string>().c_str(), "dummy2");
        EXPECT_EQ(writer.get(), nullptr);
        helper.mark("2");
        helper.pass_signal("2");

        printf("send dummy2\n");
        // todo: avoid block without MESSAGING_IGNORE_RESPONSE option.
        c.messaging_post(
            node1->get_local_nid(), "call3", Value(), 0,
            [&](Colonio&, const Value&) {
              helper.mark("4");
              helper.pass_signal("4");
            },
            [&](Colonio&, const Error&) {
              ADD_FAILURE();
            });
      });

  NodeID target =
      NodeID::center_mod(NodeID::from_str(node1->get_local_nid()), NodeID::from_str(node2->get_local_nid()));
  helper.wait_signal("1", [&] {
    printf("send dummy1\n");
    Value result1 = node1->messaging_post(target.to_str(), "call1", Value("dummy1"), Colonio::MESSAGING_ACCEPT_NEARBY);
    EXPECT_STREQ(result1.get<std::string>().c_str(), "result1");
    printf("send dummy0\n");
    node2->messaging_post(
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
    node1->messaging_post(
        node2->get_local_nid(), "call2", Value("dummy2"), Colonio::MESSAGING_IGNORE_RESPONSE,
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

TEST(MessagingTest, callMessageChain) {
  const std::string URL   = "http://localhost:8080/test";
  const std::string TOKEN = "";

  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  auto config             = make_config_with_name("node");
  config.max_user_threads = 3;
  std::unique_ptr<Colonio> node(Colonio::new_instance(config));

  printf("connect node\n");
  node->connect(URL, TOKEN);

  node->messaging_set_handler(
      "hop", [&](Colonio& c, const Colonio::MessagingRequest& message,
                 std::shared_ptr<Colonio::MessagingResponseWriter> writer) {
        std::string message_str = message.message.get<std::string>();

        writer->write(node->messaging_post(node->get_local_nid(), "step", Value(message_str + " hop"), 0));
      });

  node->messaging_set_handler(
      "step", [&](Colonio& c, const Colonio::MessagingRequest& message,
                  std::shared_ptr<Colonio::MessagingResponseWriter> writer) {
        std::string message_str = message.message.get<std::string>();

        writer->write(node->messaging_post(node->get_local_nid(), "jump", Value(message_str + " step"), 0));
      });

  node->messaging_set_handler(
      "jump", [&](Colonio& c, const Colonio::MessagingRequest& message,
                  std::shared_ptr<Colonio::MessagingResponseWriter> writer) {
        std::string message_str = message.message.get<std::string>();

        writer->write(Value(message_str + " jump!"));
      });

  node->messaging_post(
      node->get_local_nid(), "hop", Value("ready?"), 0,
      [&](Colonio&, const Value& result) {
        EXPECT_STREQ(result.get<std::string>().c_str(), "ready? hop step jump!");
        helper.pass_signal("a");
      },
      [&](Colonio&, const Error&) {
        ADD_FAILURE();
      });

  helper.wait_signal("a");

  printf("disconnect node\n");
  node->disconnect();
}
