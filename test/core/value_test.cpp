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

#include "colonio/colonio.h"
#include "colonio/colonio.hpp"
#include "core/value_impl.hpp"

using namespace colonio;

bool check_bin_equal(const void* ptr1, const void* ptr2, std::size_t siz) {
  const uint8_t* bin1 = reinterpret_cast<const uint8_t*>(ptr1);
  const uint8_t* bin2 = reinterpret_cast<const uint8_t*>(ptr2);

  for (std::size_t i = 0; i < siz; i++) {
    if (bin1[i] != bin2[i]) {
      return false;
    }
  }

  return true;
}

TEST(ValueTest, cpp) {
  // null
  Value value_null;
  EXPECT_EQ(value_null.get_type(), Value::Type::NULL_T);
  // overwrite
  value_null.set(false);
  EXPECT_EQ(value_null.get_type(), Value::Type::BOOL_T);
  EXPECT_EQ(value_null.get<bool>(), false);

  // bool
  Value value_bool(true);
  EXPECT_EQ(value_bool.get_type(), Value::Type::BOOL_T);
  EXPECT_EQ(value_bool.get<bool>(), true);
  // overwrite
  value_bool.set(static_cast<int64_t>(3));
  EXPECT_EQ(value_bool.get_type(), Value::Type::INT_T);
  EXPECT_EQ(value_bool.get<int64_t>(), 3);

  // int
  Value value_int(static_cast<int64_t>(10));
  EXPECT_EQ(value_int.get_type(), Value::Type::INT_T);
  EXPECT_EQ(value_int.get<int64_t>(), 10);
  // overwrite
  value_int.set(10.1);
  EXPECT_EQ(value_int.get_type(), Value::Type::DOUBLE_T);
  EXPECT_EQ(value_int.get<double>(), 10.1);

  // double
  Value value_double(3.14);
  EXPECT_EQ(value_double.get_type(), Value::Type::DOUBLE_T);
  EXPECT_EQ(value_double.get<double>(), 3.14);
  // overwrite
  value_double.set(std::string("hello"));
  EXPECT_EQ(value_double.get_type(), Value::Type::STRING_T);
  EXPECT_EQ(value_double.get<std::string>(), std::string("hello"));

  // string
  Value value_string("hoge");
  EXPECT_EQ(value_string.get_type(), Value::Type::STRING_T);
  EXPECT_EQ(value_string.get<std::string>(), std::string("hoge"));
  // overwrite
  const char data1[] = "🤗";
  value_string.set(data1, sizeof(data1));
  EXPECT_EQ(value_string.get_type(), Value::Type::BINARY_T);
  EXPECT_EQ(value_string.get_binary_size(), sizeof(data1));
  EXPECT_TRUE(check_bin_equal(data1, value_string.get_binary(), sizeof(data1)));

  // binary
  const char data2[] = "🌒😮";
  Value value_binary(data2, sizeof(data2));
  EXPECT_EQ(value_binary.get_type(), Value::Type::BINARY_T);
  EXPECT_EQ(value_binary.get_binary_size(), sizeof(data2));
  EXPECT_TRUE(check_bin_equal(data2, value_binary.get_binary(), sizeof(data2)));
  // overwrite
  value_binary.reset();
  EXPECT_EQ(value_binary.get_type(), Value::Type::NULL_T);
}

TEST(ValueTest, c) {
  // null
  colonio_value_t value_null;
  colonio_value_create(&value_null);
  EXPECT_EQ(colonio_value_get_type(value_null), COLONIO_VALUE_TYPE_NULL);
  colonio_value_free(&value_null);

  // bool
  colonio_value_t value_bool;
  colonio_value_create(&value_bool);
  colonio_value_set_bool(value_bool, true);
  EXPECT_EQ(colonio_value_get_type(value_bool), COLONIO_VALUE_TYPE_BOOL);
  EXPECT_EQ(colonio_value_get_bool(value_bool), true);
  colonio_value_free(&value_bool);

  // int
  colonio_value_t value_int;
  colonio_value_create(&value_int);
  colonio_value_set_int(value_int, 1024);
  EXPECT_EQ(colonio_value_get_type(value_int), COLONIO_VALUE_TYPE_INT);
  EXPECT_EQ(colonio_value_get_int(value_int), 1024);
  colonio_value_free(&value_int);

  // double
  colonio_value_t value_double;
  colonio_value_create(&value_double);
  colonio_value_set_double(value_double, 3.14);
  EXPECT_EQ(colonio_value_get_type(value_double), COLONIO_VALUE_TYPE_DOUBLE);
  EXPECT_EQ(colonio_value_get_double(value_double), 3.14);
  colonio_value_free(&value_double);

  // string
  const char str[] = "hello";
  colonio_value_t value_string;
  colonio_value_create(&value_string);
  colonio_value_set_string(value_string, str, sizeof(str));
  EXPECT_EQ(colonio_value_get_type(value_string), COLONIO_VALUE_TYPE_STRING);
  unsigned int str_siz;
  EXPECT_TRUE(check_bin_equal(colonio_value_get_string(value_string, &str_siz), str, sizeof(str)));
  EXPECT_EQ(sizeof(str), str_siz);
  colonio_value_free(&value_string);

  // binary
  const char bin[] = "ばなな🍌";
  colonio_value_t value_binary;
  colonio_value_create(&value_binary);
  colonio_value_set_binary(value_binary, bin, sizeof(bin));
  EXPECT_EQ(colonio_value_get_type(value_binary), COLONIO_VALUE_TYPE_BINARY);
  unsigned int bin_siz;
  EXPECT_TRUE(check_bin_equal(colonio_value_get_binary(value_binary, &bin_siz), bin, sizeof(bin)));
  EXPECT_EQ(sizeof(bin), bin_siz);
  colonio_value_free(&value_binary);
}