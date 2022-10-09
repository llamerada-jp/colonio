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

#include <gtest/gtest.h>

#include "core/utils.hpp"

using namespace colonio;

TEST(UtilsTest, Exception) {
  unsigned int l = 0;
  try {
    l = __LINE__;
    colonio_throw_error(ErrorCode::CONNECTION_FAILED, "test");

  } catch (Error& e) {
    EXPECT_FALSE(e.fatal);
    EXPECT_EQ(e.line, l + 1);
    EXPECT_STREQ(e.file.c_str(), "exception_test.cpp");
    EXPECT_EQ(e.code, ErrorCode::CONNECTION_FAILED);
    EXPECT_EQ(e.message, "test");
    EXPECT_STREQ(e.what(), "test");
  }

  try {
    l = __LINE__;
    colonio_throw_fatal("test");

  } catch (Error& e) {
    EXPECT_TRUE(e.fatal);
    EXPECT_EQ(e.line, l + 1);
    EXPECT_STREQ(e.file.c_str(), "exception_test.cpp");
    EXPECT_EQ(e.code, ErrorCode::UNDEFINED);
    EXPECT_EQ(e.message, "test");
    EXPECT_STREQ(e.what(), "test");
  }
}