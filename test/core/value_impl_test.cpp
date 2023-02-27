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

#include "core/value_impl.hpp"

#include <gtest/gtest.h>

#include "core/colonio.pb.h"
#include "test_utils/all.hpp"

using namespace colonio;

TEST(ValueImplTest, null) {
  Value src;
  EXPECT_EQ(ValueImpl::to_str(src), std::string("null"));

  proto::Value pb;
  ValueImpl::to_pb(&pb, src);

  Value dst = ValueImpl::from_pb(pb);
  EXPECT_EQ(dst.get_type(), Value::Type::NULL_T);
}

TEST(ValueImplTest, bool) {
  Value src(true);
  EXPECT_EQ(ValueImpl::to_str(src), std::string("true"));

  proto::Value pb;
  ValueImpl::to_pb(&pb, src);

  Value dst = ValueImpl::from_pb(pb);
  EXPECT_EQ(dst.get_type(), Value::Type::BOOL_T);
  EXPECT_TRUE(dst.get<bool>());
}

TEST(ValueImplTest, int) {
  Value src(static_cast<int64_t>(-9817));
  EXPECT_EQ(ValueImpl::to_str(src), std::string("-9817"));

  proto::Value pb;
  ValueImpl::to_pb(&pb, src);

  Value dst = ValueImpl::from_pb(pb);
  EXPECT_EQ(dst.get_type(), Value::Type::INT_T);
  EXPECT_EQ(dst.get<int64_t>(), -9817);
}

TEST(ValueImplTest, double) {
  Value src(3.14159265359);
  EXPECT_EQ(ValueImpl::to_str(src), std::string("3.141593"));

  proto::Value pb;
  ValueImpl::to_pb(&pb, src);

  Value dst = ValueImpl::from_pb(pb);
  EXPECT_EQ(dst.get_type(), Value::Type::DOUBLE_T);
  EXPECT_EQ(dst.get<double>(), 3.14159265359);
}

TEST(ValueImplTest, string) {
  Value src("👾💀💪");
  EXPECT_EQ(ValueImpl::to_str(src), std::string("\"👾💀💪\""));

  proto::Value pb;
  ValueImpl::to_pb(&pb, src);

  Value dst = ValueImpl::from_pb(pb);
  EXPECT_EQ(dst.get_type(), Value::Type::STRING_T);
  EXPECT_EQ(dst.get<std::string>(), std::string("👾💀💪"));
}

TEST(ValueImplTest, binary) {
  std::string data("🐳");
  Value src(data.data(), data.size());
  EXPECT_EQ(ValueImpl::to_str(src), std::string("f0 9f 90 b3"));

  proto::Value pb;
  ValueImpl::to_pb(&pb, src);

  Value dst = ValueImpl::from_pb(pb);
  EXPECT_EQ(dst.get_type(), Value::Type::BINARY_T);
  EXPECT_EQ(dst.get_binary_size(), data.size());
  EXPECT_TRUE(check_bin_equal(dst.get_binary(), data.data(), dst.get_binary_size()));
}
