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

#include <cmath>
#include <tuple>

#include "colonio/colonio.hpp"
#include "test_utils/all.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

double d2r(double d) {
  return M_PI * d / 180.0;
}

TEST(SpreadTest, async) {
  const std::string URL   = "http://localhost:8080/test";
  const std::string TOKEN = "";

  AsyncHelper helper;
  TestSeed seed;
  seed.set_coord_system_sphere();
  seed.run();

  auto config1 = make_config_with_name("node1");
  std::unique_ptr<Colonio> node1(Colonio::new_instance(config1));
  auto config2 = make_config_with_name("node2");
  std::unique_ptr<Colonio> node2(Colonio::new_instance(config2));

  printf("connect node1\n");
  node1->connect(
      URL, TOKEN,
      [&URL, &TOKEN, &node2, &helper](Colonio& _) {
        printf("connect node2\n");
        node2->connect(
            URL, TOKEN,
            [&helper](Colonio& _) {
              helper.pass_signal("connect");
            },
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
  node1->spread_set_handler("key", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
    helper.mark("1" + r.message.get<std::string>());
    helper.pass_signal("on1");
  });
  node2->spread_set_handler("key", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
    helper.mark("2" + r.message.get<std::string>());
    helper.pass_signal("on2");
  });

  {
    double x, y;
    printf("set position node1\n");
    std::tie(x, y) = node1->set_position(d2r(50), d2r(50));
    EXPECT_FLOAT_EQ(d2r(50), x);
    EXPECT_FLOAT_EQ(d2r(50), y);
  }

  {
    double x, y;
    printf("set position node2\n");
    std::tie(x, y) = node2->set_position(d2r(50), d2r(50));
    EXPECT_FLOAT_EQ(d2r(50), x);
    EXPECT_FLOAT_EQ(d2r(50), y);
  }

  printf("wait publishing\n");
  helper.wait_signal("on1", [&node2] {
    printf("publish position node2\n");
    node2->spread_post(
        d2r(50), d2r(50), 10, "key", Value("b"), 0, [](Colonio& _) {},
        [](Colonio& _, const Error& err) {
          std::cout << err.message << std::endl;
          ADD_FAILURE();
        });
  });
  helper.wait_signal("on2", [&node1] {
    printf("publish position node1\n");
    node1->spread_post(
        d2r(50), d2r(50), 10, "key", Value("a"), 0, [](Colonio& _) {},
        [](Colonio& _, const Error& err) {
          std::cout << err.message << std::endl;
          ADD_FAILURE();
        });
  });

  printf("disconnect\n");
  node1->disconnect();
  node2->disconnect();

  EXPECT_THAT(helper.get_route(), MatchesRegex("^1b2a|2a1b$"));
}

TEST(SpreadTest, multi_node) {
  const std::string URL   = "http://localhost:8080/test";
  const std::string TOKEN = "";

  AsyncHelper helper;
  TestSeed seed;
  seed.set_coord_system_sphere();
  seed.run();

  auto config1 = make_config_with_name("node1");
  std::unique_ptr<Colonio> node1(Colonio::new_instance(config1));
  auto config2 = make_config_with_name("node2");
  std::unique_ptr<Colonio> node2(Colonio::new_instance(config2));

  try {
    // connect node1;
    printf("connect node1\n");
    node1->connect(URL, TOKEN);
    node1->spread_set_handler("key1", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
      helper.mark("11");
      helper.mark(r.message.get<std::string>());
      helper.pass_signal("on11");
    });
    node1->spread_set_handler("key2", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
      helper.mark("12");
      helper.mark(r.message.get<std::string>());
      helper.pass_signal("on12");
    });

    // connect node2;
    printf("connect node2\n");
    node2->connect(URL, TOKEN);
    node2->spread_set_handler("key1", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
      helper.mark("21");
      helper.mark(r.message.get<std::string>());
      helper.pass_signal("on21");
    });
    node2->spread_set_handler("key2", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
      helper.mark("22");
      helper.mark(r.message.get<std::string>());
      helper.pass_signal("on22");
    });

    printf("set position 1 and 2\n");
    node1->set_position(d2r(100), d2r(50));
    node2->set_position(d2r(50), d2r(50));

    helper.wait_signal("on21", [&node1] {
      printf("post a\n");
      node1->spread_post(d2r(50), d2r(50), 10, "key1", Value("a"));  // 21a
    });
    printf("post b\n");
    node2->spread_post(d2r(50), d2r(50), 10, "key2", Value("b"));  // none

    helper.clear_signal();
    printf("set position 2\n");
    node2->set_position(d2r(-20), d2r(10));

    printf("publish c\n");
    node1->spread_post(d2r(50), d2r(50), 10, "key1", Value("c"));  // none
    helper.wait_signal("on21", [&node1] {
      printf("publish d\n");
      node1->spread_post(d2r(-20), d2r(10), 10, "key1", Value("d"));  // 21d
    });
    helper.wait_signal("on22", [&node1] {
      printf("publish e\n");
      node1->spread_post(d2r(-20), d2r(10), 10, "key2", Value("e"));  // 22e
    });

  } catch (colonio::Error& err) {
    printf("exception code:%u: %s\n", static_cast<uint32_t>(err.code), err.message.c_str());
    ADD_FAILURE();
  }

  // disconnect
  printf("disconnect\n");
  node1->disconnect();
  node2->disconnect();

  EXPECT_THAT(helper.get_route(), MatchesRegex("^21a21d22e$"));
}

TEST(SpreadTest, plane) {
  const std::string URL   = "http://localhost:8080/test";
  const std::string TOKEN = "";

  printf("setup seed\n");
  AsyncHelper helper;
  TestSeed seed;
  seed.set_coord_system_plane();
  seed.run();

  auto config1 = make_config_with_name("node1");
  std::unique_ptr<Colonio> node1(Colonio::new_instance(config1));
  auto config2 = make_config_with_name("node2");
  std::unique_ptr<Colonio> node2(Colonio::new_instance(config2));

  // connect node1;
  printf("connect node1\n");
  node1->connect(URL, TOKEN);
  printf("connect node1 fin\n");
  node1->spread_set_handler("key1", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
    helper.mark("11");
    helper.mark(r.message.get<std::string>());
  });
  node1->spread_set_handler("key2", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
    helper.mark("12");
    helper.mark(r.message.get<std::string>());
  });

  // connect node2;
  printf("connect node2\n");
  node2->connect(URL, TOKEN);
  printf("connect node2 fin\n");
  node2->spread_set_handler("key1", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
    helper.mark("21");
    helper.mark(r.message.get<std::string>());
  });
  node2->spread_set_handler("key2", [&helper](Colonio&, const Colonio::SpreadRequest& r) {
    helper.mark("22");
    helper.mark(r.message.get<std::string>());
  });

  double x1, y1;
  std::tie(x1, y1) = node1->set_position(-0.5, 0.5);
  EXPECT_FLOAT_EQ(x1, -0.5);
  EXPECT_FLOAT_EQ(y1, 0.5);

  double x2, y2;
  std::tie(x2, y2) = node2->set_position(0.5, -0.5);
  EXPECT_FLOAT_EQ(x2, 0.5);
  EXPECT_FLOAT_EQ(y2, -0.5);

  // disconnect
  printf("disconnect\n");
  node1->disconnect();
  node2->disconnect();
}
