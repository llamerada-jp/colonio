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

#include <cmath>

#include "test_utils/all.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

double d2r(double d) {
  return M_PI * d / 180.0;
}

TEST(Pubsub2DTest, pubsub_async) {
  const std::string URL            = "http://localhost:8080/test";
  const std::string TOKEN          = "";
  const std::string PUBSUB_2D_NAME = "ps2";

  AsyncHelper helper;
  TestSeed seed;
  seed.set_coord_system_sphere();
  seed.add_module_pubsub_2d(PUBSUB_2D_NAME, 256);
  seed.run();

  ColonioNode node1("node1");
  ColonioNode node2("node2");

  printf("connect node1\n");
  node1.connect(
      URL, TOKEN,
      [&URL, &TOKEN, &node2, &helper](Colonio& _) {
        printf("connect node2\n");
        node2.connect(
            URL, TOKEN, [&helper](Colonio& _) { helper.pass_signal("connect"); },
            [](Colonio&, const Error& err) {
              std::cout << err.message << std::endl;
              ADD_FAILURE();
            });
      },
      [](Colonio&, const Error& err) {
        std::cout << err.message << std::endl;
        ADD_FAILURE();
      });

  printf("wait connecting\n");
  helper.wait_signal("connect");
  Pubsub2D& ps1 = node1.access_pubsub_2d(PUBSUB_2D_NAME);
  Pubsub2D& ps2 = node2.access_pubsub_2d(PUBSUB_2D_NAME);
  ps1.on("key", [&helper](const Value& v) {
    helper.mark("1" + v.get<std::string>());
    helper.pass_signal("on1");
  });
  ps2.on("key", [&helper](const Value& v) {
    helper.mark("2" + v.get<std::string>());
    helper.pass_signal("on2");
  });

  printf("set position node1\n");
  node1.set_position(
      d2r(50), d2r(50),
      [&helper](Colonio& _, double x, double y) {
        EXPECT_FLOAT_EQ(d2r(50), x);
        EXPECT_FLOAT_EQ(d2r(50), y);
        helper.pass_signal("pos1");
      },
      [](Colonio&, const Error& err) {
        std::cout << err.message << std::endl;
        ADD_FAILURE();
      });
  printf("set position node2\n");
  node2.set_position(
      d2r(50), d2r(50),
      [&helper](Colonio& _, double x, double y) {
        EXPECT_FLOAT_EQ(d2r(50), x);
        EXPECT_FLOAT_EQ(d2r(50), y);
        helper.pass_signal("pos2");
      },
      [](Colonio&, const Error& err) {
        std::cout << err.message << std::endl;
        ADD_FAILURE();
      });
  helper.wait_signal("pos1");
  helper.wait_signal("pos2");

  printf("wait publishing\n");
  helper.wait_signal("on1", [&ps2] {
    printf("publish position node2\n");
    ps2.publish(
        "key", d2r(50), d2r(50), 10, Value("b"), 0, [](Pubsub2D& _) {},
        [](Pubsub2D& _, const Error& err) {
          std::cout << err.message << std::endl;
          ADD_FAILURE();
        });
  });
  helper.wait_signal("on2", [&ps1] {
    printf("publish position node1\n");
    ps1.publish(
        "key", d2r(50), d2r(50), 10, Value("a"), 0, [](Pubsub2D& _) {},
        [](Pubsub2D& _, const Error& err) {
          std::cout << err.message << std::endl;
          ADD_FAILURE();
        });
  });

  printf("disconnect\n");
  node1.disconnect();
  node2.disconnect();

  EXPECT_THAT(helper.get_route(), MatchesRegex("^1b2a|2a1b$"));
}

TEST(Pubsub2DTest, multi_node) {
  const std::string URL            = "http://localhost:8080/test";
  const std::string TOKEN          = "";
  const std::string PUBSUB_2D_NAME = "ps2";

  AsyncHelper helper;
  TestSeed seed;
  seed.set_coord_system_sphere();
  seed.add_module_pubsub_2d(PUBSUB_2D_NAME, 256);
  seed.run();

  ColonioNode node1("node1");
  ColonioNode node2("node2");

  try {
    // connect node1;
    printf("connect node1\n");
    node1.connect(URL, TOKEN);
    Pubsub2D& ps1 = node1.access_pubsub_2d(PUBSUB_2D_NAME);
    ps1.on("key1", [&helper](const Value& v) {
      helper.mark("11");
      helper.mark(v.get<std::string>());
      helper.pass_signal("on11");
    });
    ps1.on("key2", [&helper](const Value& v) {
      helper.mark("12");
      helper.mark(v.get<std::string>());
      helper.pass_signal("on12");
    });

    // connect node2;
    printf("connect node2\n");
    node2.connect(URL, TOKEN);
    Pubsub2D& ps2 = node2.access_pubsub_2d(PUBSUB_2D_NAME);
    ps2.on("key1", [&helper](const Value& v) {
      helper.mark("21");
      helper.mark(v.get<std::string>());
      helper.pass_signal("on21");
    });
    ps2.on("key2", [&helper](const Value& v) {
      helper.mark("22");
      helper.mark(v.get<std::string>());
      helper.pass_signal("on22");
    });

    printf("set position 1 and 2\n");
    node1.set_position(d2r(100), d2r(50));
    node2.set_position(d2r(50), d2r(50));

    helper.wait_signal("on21", [&ps1] {
      printf("publish a\n");
      ps1.publish("key1", d2r(50), d2r(50), 10, Value("a"));  // 21a
    });
    printf("publish b\n");
    ps2.publish("key2", d2r(50), d2r(50), 10, Value("b"));  // none

    helper.clear_signal();
    printf("set position 2\n");
    node2.set_position(d2r(-20), d2r(10));

    printf("publish c\n");
    ps1.publish("key1", d2r(50), d2r(50), 10, Value("c"));  // none
    helper.wait_signal("on21", [&ps1] {
      printf("publish d\n");
      ps1.publish("key1", d2r(-20), d2r(10), 10, Value("d"));  // 21d
    });
    helper.wait_signal("on22", [&ps1] {
      printf("publish e\n");
      ps1.publish("key2", d2r(-20), d2r(10), 10, Value("e"));  // 22e
    });

  } catch (colonio::Exception& ex) {
    printf("exception code:%u: %s\n", static_cast<uint32_t>(ex.code), ex.message.c_str());
    ADD_FAILURE();
  }

  // disconnect
  printf("disconnect\n");
  node1.disconnect();
  node2.disconnect();

  EXPECT_THAT(helper.get_route(), MatchesRegex("^21a21d22e$"));
}

TEST(Pubsub2DTest, pubsub_plane) {
  const std::string URL            = "http://localhost:8080/test";
  const std::string TOKEN          = "";
  const std::string PUBSUB_2D_NAME = "ps2";

  printf("setup seed\n");
  AsyncHelper helper;
  TestSeed seed;
  seed.set_coord_system_plane();
  seed.add_module_pubsub_2d(PUBSUB_2D_NAME, 256);
  seed.run();

  printf("create instance1\n");
  ColonioNode node1("node1");
  printf("create instance2\n");
  ColonioNode node2("node2");

  // connect node1;
  printf("connect node1\n");
  node1.connect(URL, TOKEN);
  printf("connect node1 fin\n");
  Pubsub2D& ps1 = node1.access_pubsub_2d(PUBSUB_2D_NAME);
  ps1.on("key1", [&helper](const Value& v) {
    helper.mark("11");
    helper.mark(v.get<std::string>());
  });
  ps1.on("key2", [&helper](const Value& v) {
    helper.mark("12");
    helper.mark(v.get<std::string>());
  });

  // connect node2;
  printf("connect node2\n");
  node2.connect(URL, TOKEN);
  printf("connect node2 fin\n");
  Pubsub2D& ps2 = node2.access_pubsub_2d(PUBSUB_2D_NAME);
  ps2.on("key1", [&helper](const Value& v) {
    helper.mark("21");
    helper.mark(v.get<std::string>());
  });
  ps2.on("key2", [&helper](const Value& v) {
    helper.mark("22");
    helper.mark(v.get<std::string>());
  });

  double x1, y1;
  std::tie(x1, y1) = node1.set_position(-0.5, 0.5);
  EXPECT_FLOAT_EQ(x1, -0.5);
  EXPECT_FLOAT_EQ(y1, 0.5);

  double x2, y2;
  std::tie(x2, y2) = node2.set_position(0.5, -0.5);
  EXPECT_FLOAT_EQ(x2, 0.5);
  EXPECT_FLOAT_EQ(y2, -0.5);

  // disconnect
  printf("disconnect\n");
  node1.disconnect();
  node2.disconnect();
}
