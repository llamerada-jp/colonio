
#include "core/node_id.hpp"

#include <gtest/gtest.h>

using namespace colonio;

TEST(NodeIDTest, from_str) {
  const NodeID out00;
  EXPECT_EQ(out00, NodeID::NONE);

  const NodeID out01 = NodeID::from_str("");
  EXPECT_EQ(out01, NodeID::NONE);
}
