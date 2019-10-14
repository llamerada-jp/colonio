/*
 * Copyright 2017-2019 Yuji Ito <llamerada.jp@gmail.com>
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

TEST(UtilsTest, format_string) {
  EXPECT_EQ(Utils::format_string("", 0), "");
  EXPECT_EQ(Utils::format_string("test", 0), "test");
  EXPECT_EQ(Utils::format_string("test %d", 0, 0), "test 0");
  EXPECT_EQ(Utils::format_string("test %s %d", 0, "hello world", 1), "test hello world 1");
}