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

#include "core/node_id.hpp"

#include <gtest/gtest.h>

#include "core/core.pb.h"
#include "core/internal_exception.hpp"
#include "core/random.hpp"

using namespace colonio;

// funcs for test
uint64_t ID0(const NodeID& id) {
  uint64_t id0, id1;
  id.get_raw(&id0, &id1);
  return id0;
}

uint64_t ID1(const NodeID& id) {
  uint64_t id0, id1;
  id.get_raw(&id0, &id1);
  return id1;
}

TEST(NodeIDTest, str) {
  const NodeID out00;
  EXPECT_EQ(out00, NodeID::NONE);

  EXPECT_EQ(NodeID::from_str(""), NodeID::NONE);
  EXPECT_EQ(NodeID::from_str("."), NodeID::THIS);
  EXPECT_EQ(NodeID::from_str("seed"), NodeID::SEED);
  EXPECT_EQ(NodeID::from_str("next"), NodeID::NEXT);

  EXPECT_STREQ(NodeID::NONE.to_str().c_str(), "");
  EXPECT_STREQ(NodeID::THIS.to_str().c_str(), ".");
  EXPECT_STREQ(NodeID::SEED.to_str().c_str(), "seed");
  EXPECT_STREQ(NodeID::NEXT.to_str().c_str(), "next");

  const NodeID out01 = NodeID::from_str("0123456789abcdefABCDEF0123456789");
  EXPECT_EQ(ID0(out01), static_cast<uint64_t>(0x0123456789abcdef));
  EXPECT_EQ(ID1(out01), static_cast<uint64_t>(0xABCDEF0123456789));

  bool has_error = false;
  try {
    NodeID::from_str("0123456789abcdef0123456789ABCDEZ");
  } catch (InternalException& e) {
    has_error = true;
    EXPECT_STREQ(e.what(), "illegal node-id string. (string : 0123456789abcdef0123456789ABCDEZ)");
  }
  EXPECT_EQ(has_error, true);

  has_error = false;
  try {
    NodeID::from_str("Z123456789abcdef0123456789ABCDE");
  } catch (InternalException& e) {
    has_error = true;
    EXPECT_STREQ(e.what(), "illegal node-id string. (string : Z123456789abcdef0123456789ABCDE)");
  }
  EXPECT_EQ(has_error, true);
}

TEST(NodeIDTest, pb) {
#define TEST00(T, V)                           \
  {                                            \
    NodeID itest00 = NodeID::T;                \
    core::NodeID ptest00;                      \
    itest00.to_pb(&ptest00);                   \
    EXPECT_EQ(ptest00.type(), V);              \
    NodeID otest00 = NodeID::from_pb(ptest00); \
    EXPECT_EQ(otest00, NodeID::T);             \
  }

  TEST00(NONE, static_cast<unsigned int>(0));
  TEST00(THIS, static_cast<unsigned int>(2));
  TEST00(SEED, static_cast<unsigned int>(3));
  TEST00(NEXT, static_cast<unsigned int>(4));
#undef TEST00

  {
    NodeID itest01 = NodeID::from_str("0123456789abcdef9876543210ABCDEF");
    core::NodeID ptest01;
    itest01.to_pb(&ptest01);
    // EXPECT_EQ(ptest01.type(), Type::NORMAL);
    EXPECT_EQ(ptest01.id0(), static_cast<uint64_t>(0x0123456789ABCDEF));
    EXPECT_EQ(ptest01.id1(), static_cast<uint64_t>(0x9876543210ABCDEF));
    ptest01.set_id0(0x9876543210FEDCBA);
    ptest01.set_id1(0xFEDCBA9876543210);
    NodeID otest01 = NodeID::from_pb(ptest01);
    // EXPECT_EQ(otest01.type(), Type::NORMAL);
    EXPECT_EQ(ID0(otest01), 0x9876543210FEDCBA);
    EXPECT_EQ(ID1(otest01), 0xFEDCBA9876543210);
  }

  bool has_error = false;
  try {
    core::NodeID ptest02;
    ptest02.set_type(5);
    NodeID::from_pb(ptest02);

  } catch (InternalException& e) {
    has_error = true;
    EXPECT_STREQ(e.what(), "illegal node-id type in Protocol Buffers. (type : 5)");
  }
  EXPECT_EQ(has_error, true);
}

TEST(NodeIDTest, make_hash_from_str) {
  NodeID test00 = NodeID::make_hash_from_str("test");
  // md5 hash of "test"
  NodeID test01 = NodeID::from_str("098f6bcd4621d373cade4e832627b4f6");
  EXPECT_EQ(test00, test01);
}

TEST(NodeIDTest, make_random) {
  Random random;

  NodeID test00 = NodeID::make_random(random);
  NodeID test01 = NodeID::make_random(random);
  NodeID test02 = NodeID::make_random(random);

  EXPECT_NE(test00, test01);
  EXPECT_NE(test01, test02);
  EXPECT_NE(test02, test00);
}

TEST(NodeIDTest, eq) {
  Random random;

  NodeID test00;
  NodeID test01 = NodeID::make_random(random);

  test00 = test01;
  EXPECT_NE(test00, NodeID::NONE);
  EXPECT_EQ(ID0(test00), ID0(test01));
  EXPECT_EQ(ID1(test00), ID1(test01));
}

TEST(NodeIDTest, pluseq) {
  Random random;

  NodeID test0 = NodeID::make_random(random);
  NodeID test1 = NodeID::make_random(random);
  NodeID test2 = test0 + test1;
  NodeID test3 = test0;
  test3 += test1;
  NodeID test4 = test1;
  test4 += test0;

  EXPECT_EQ(test2, test3);
  EXPECT_EQ(test2, test4);
}

TEST(NodeIDTest, eqeq) {
  EXPECT_TRUE(NodeID::NONE == NodeID::from_str(""));
  EXPECT_TRUE(NodeID::SEED == NodeID::from_str("seed"));
  EXPECT_TRUE(NodeID::THIS == NodeID::from_str("."));
  EXPECT_TRUE(NodeID::NEXT == NodeID::from_str("next"));
  EXPECT_TRUE(
      NodeID::from_str("098f6bcd4621d373cade4e832627b4f6") == NodeID::from_str("098f6bcd4621d373cade4e832627b4f6"));

  EXPECT_FALSE(NodeID::NONE == NodeID::SEED);
  EXPECT_FALSE(NodeID::NONE == NodeID::from_str("098f6bcd4621d373cade4e832627b4f6"));
  EXPECT_FALSE(NodeID::NEXT == NodeID::from_str("098f6bcd4621d373cade4e832627b4f6"));
  EXPECT_FALSE(
      NodeID::from_str("098f6bcd4621d373cade4e832627b4f6") == NodeID::from_str("098f6bcd4621d373cade4e832627b4f7"));
}

TEST(NodeIDTest, neq) {
  EXPECT_FALSE(NodeID::NONE != NodeID::from_str(""));
  EXPECT_FALSE(NodeID::SEED != NodeID::from_str("seed"));
  EXPECT_FALSE(NodeID::THIS != NodeID::from_str("."));
  EXPECT_FALSE(NodeID::NEXT != NodeID::from_str("next"));
  EXPECT_FALSE(
      NodeID::from_str("098f6bcd4621d373cade4e832627b4f6") != NodeID::from_str("098f6bcd4621d373cade4e832627b4f6"));

  EXPECT_TRUE(NodeID::NONE != NodeID::SEED);
  EXPECT_TRUE(NodeID::NONE != NodeID::from_str("098f6bcd4621d373cade4e832627b4f6"));
  EXPECT_TRUE(NodeID::NEXT != NodeID::from_str("098f6bcd4621d373cade4e832627b4f6"));
  EXPECT_TRUE(
      NodeID::from_str("098f6bcd4621d373cade4e832627b4f6") != NodeID::from_str("098f6bcd4621d373cade4e832627b4f7"));
}

TEST(NodeIDTest, sm) {
  EXPECT_FALSE(NodeID::NONE < NodeID::NONE);
  EXPECT_TRUE(NodeID::NONE < NodeID::from_str("00000000000000000000000000000000"));
  EXPECT_FALSE(NodeID::from_str("00000000000000000000000000000000") < NodeID::NONE);
  EXPECT_TRUE(
      NodeID::from_str("00000000000000000000000000000000") < NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_FALSE(
      NodeID::from_str("00000000000000000000000000000001") < NodeID::from_str("00000000000000000000000000000000"));
  EXPECT_TRUE(
      NodeID::from_str("00000000000000000000000000000000") < NodeID::from_str("00000000000000010000000000000000"));
  EXPECT_FALSE(
      NodeID::from_str("00000000000000010000000000000000") < NodeID::from_str("00000000000000000000000000000000"));
}

TEST(NodeIDTest, gt) {
  EXPECT_FALSE(NodeID::NONE > NodeID::NONE);
  EXPECT_TRUE(NodeID::from_str("00000000000000000000000000000000") > NodeID::NONE);
  EXPECT_FALSE(NodeID::NONE > NodeID::from_str("00000000000000000000000000000000"));
  EXPECT_TRUE(
      NodeID::from_str("00000000000000000000000000000001") > NodeID::from_str("00000000000000000000000000000000"));
  EXPECT_FALSE(
      NodeID::from_str("00000000000000000000000000000000") > NodeID::from_str("00000000000000000000000000000001"));

  EXPECT_TRUE(
      NodeID::from_str("00000000000000010000000000000000") > NodeID::from_str("00000000000000000000000000000000"));
  EXPECT_FALSE(
      NodeID::from_str("00000000000000000000000000000000") > NodeID::from_str("00000000000000010000000000000000"));
}

TEST(NodeIDTest, plus) {
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000001") + NodeID::from_str("0000000000000000FFFFFFFFFFFFFFFF"),
      NodeID::from_str("00000000000000010000000000000000"));
  EXPECT_EQ(
      NodeID::from_str("0000000000000000FFFFFFFFFFFFFFFF") + NodeID::from_str("00000000000000000000000000000001"),
      NodeID::from_str("00000000000000010000000000000000"));
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000001") + NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
      NodeID::from_str("00000000000000000000000000000000"));
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000002") + NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
      NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_EQ(
      NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF") + NodeID::from_str("00000000000000000000000000000001"),
      NodeID::from_str("00000000000000000000000000000000"));
  EXPECT_EQ(
      NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF") + NodeID::from_str("00000000000000000000000000000002"),
      NodeID::from_str("00000000000000000000000000000001"));
}

TEST(NodeIDTest, minus) {
  EXPECT_EQ(
      NodeID::from_str("00000000000000010000000000000000") - NodeID::from_str("00000000000000000000000000000001"),
      NodeID::from_str("0000000000000000FFFFFFFFFFFFFFFF"));
  EXPECT_EQ(
      NodeID::from_str("00000000000000010000000000000000") - NodeID::from_str("0000000000000000FFFFFFFFFFFFFFFF"),
      NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000000") - NodeID::from_str("00000000000000000000000000000001"),
      NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"));
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000001") - NodeID::from_str("00000000000000000000000000000002"),
      NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"));
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000000") - NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
      NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000001") - NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
      NodeID::from_str("00000000000000000000000000000002"));
}

TEST(NodeIDTest, center_mod) {
  EXPECT_EQ(
      NodeID::center_mod(
          NodeID::from_str("00000000000000000000000000000000"), NodeID::from_str("00000000000000000000000000000001")),
      NodeID::from_str("00000000000000000000000000000000"));
  EXPECT_EQ(
      NodeID::center_mod(
          NodeID::from_str("00000000000000000000000000000001"), NodeID::from_str("00000000000000000000000000000002")),
      NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_EQ(
      NodeID::center_mod(
          NodeID::from_str("00000000000000000000000000000000"), NodeID::from_str("00000000000000000000000000000002")),
      NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_EQ(
      NodeID::center_mod(
          NodeID::from_str("00000000000000000000000000000001"), NodeID::from_str("00000000000000000000000000000003")),
      NodeID::from_str("00000000000000000000000000000002"));
  EXPECT_EQ(
      NodeID::center_mod(
          NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), NodeID::from_str("00000000000000000000000000000000")),
      NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"));
  EXPECT_EQ(
      NodeID::center_mod(
          NodeID::from_str("00000000000000000000000000000001"), NodeID::from_str("00000000000000000000000000000000")),
      NodeID::from_str("80000000000000000000000000000000"));
}

TEST(NodeIDTest, distance_from) {
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000000")
          .distance_from(NodeID::from_str("00000000000000000000000000000001")),
      NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000001")
          .distance_from(NodeID::from_str("00000000000000000000000000000000")),
      NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_EQ(
      NodeID::from_str("80000000000000000000000000000000")
          .distance_from(NodeID::from_str("80000000000000000000000000000000")),
      NodeID::from_str("00000000000000000000000000000000"));
  EXPECT_EQ(
      NodeID::from_str("00000000000000000000000000000000")
          .distance_from(NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")),
      NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_EQ(
      NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
          .distance_from(NodeID::from_str("00000000000000000000000000000000")),
      NodeID::from_str("00000000000000000000000000000001"));
  EXPECT_EQ(
      NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
          .distance_from(NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFE")),
      NodeID::from_str("00000000000000000000000000000001"));
}

TEST(NodeIDTest, is_between) {
  EXPECT_TRUE(NodeID::from_str("00000000000000000000000000000000")
                  .is_between(
                      NodeID::from_str("00000000000000000000000000000000"),
                      NodeID::from_str("00000000000000000000000000000001")));
  EXPECT_FALSE(NodeID::from_str("00000000000000000000000000000001")
                   .is_between(
                       NodeID::from_str("00000000000000000000000000000000"),
                       NodeID::from_str("00000000000000000000000000000001")));
  EXPECT_FALSE(NodeID::from_str("00000000000000000000000000000000")
                   .is_between(
                       NodeID::from_str("00000000000000000000000000000001"),
                       NodeID::from_str("00000000000000000000000000000002")));
  EXPECT_FALSE(NodeID::from_str("00000000000000000000000000000001")
                   .is_between(
                       NodeID::from_str("00000000000000000000000000000001"),
                       NodeID::from_str("00000000000000000000000000000001")));
  EXPECT_FALSE(NodeID::from_str("00000000000000000000000000000000")
                   .is_between(
                       NodeID::from_str("00000000000000000000000000000001"),
                       NodeID::from_str("00000000000000000000000000000000")));
  EXPECT_TRUE(NodeID::from_str("00000000000000000000000000000001")
                  .is_between(
                      NodeID::from_str("00000000000000000000000000000001"),
                      NodeID::from_str("00000000000000000000000000000000")));
  EXPECT_TRUE(NodeID::from_str("00000000000000000000000000000000")
                  .is_between(
                      NodeID::from_str("00000000000000000000000000000002"),
                      NodeID::from_str("00000000000000000000000000000001")));
}

TEST(NodeIDTest, is_special) {
  EXPECT_TRUE(NodeID::NONE.is_special());
  EXPECT_FALSE(NodeID::from_str("00000000000000000000000000000000").is_special());
}

TEST(NodeIDTest, log2) {
  EXPECT_EQ(NodeID::from_str("00000000000000000000000000000000").log2(), 0);
  EXPECT_EQ(NodeID::from_str("00000000000000000000000000000001").log2(), 0);
  EXPECT_EQ(NodeID::from_str("00000000000000000000000000000002").log2(), 1);
  EXPECT_EQ(NodeID::from_str("00000000000000000000000000000004").log2(), 2);
  EXPECT_EQ(NodeID::from_str("00000000000000000000000000000008").log2(), 3);
  EXPECT_EQ(NodeID::from_str("0000000000000000000000000000000F").log2(), 3);
  EXPECT_EQ(NodeID::from_str("00000000000000000000000000000010").log2(), 4);
  EXPECT_EQ(NodeID::from_str("00000000000000008000000000000000").log2(), 63);
  EXPECT_EQ(NodeID::from_str("80000000000000000000000000000000").log2(), 127);
  EXPECT_EQ(NodeID::from_str("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF").log2(), 127);
}

TEST(NodeIDTest, to_json) {
  EXPECT_STREQ(
      NodeID::from_str("00000000000000000000000000000000").to_json().serialize().c_str(),
      "\"00000000000000000000000000000000\"");
  EXPECT_STREQ(NodeID::NONE.to_json().serialize().c_str(), "\"\"");
  EXPECT_STREQ(NodeID::THIS.to_json().serialize().c_str(), "\".\"");
}
