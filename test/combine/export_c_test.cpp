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

extern "C" {
#include <colonio/colonio.h>
}

#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "test_utils/all.hpp"

#define URL "http://localhost:8080/test"
#define TOKEN ""

struct TestData {
  AsyncHelper* helper;
  colonio_t* colonio;
};

TEST(ExternC, connect_sync) {
  TestSeed seed;
  seed.run();

  colonio_t colonio;
  colonio_error_t* err;

  err = colonio_init(&colonio);
  EXPECT_EQ(err, nullptr);

  err = colonio_connect(&colonio, URL, TOKEN);
  EXPECT_EQ(err, nullptr);

  {
    char nid[COLONIO_NID_LENGTH + 1];
    colonio_get_local_nid(&colonio, nid);
    EXPECT_EQ(strlen(nid), COLONIO_NID_LENGTH);
  }

  err = colonio_disconnect(&colonio);
  EXPECT_EQ(err, nullptr);
}

void connect_async_on_success(colonio_t* c) {
  TestData* data = reinterpret_cast<TestData*>(c->data);
  EXPECT_EQ(c, data->colonio);
  data->helper->pass_signal("connect");
}

void connect_async_on_failure(colonio_t* c, const colonio_error_t*) {
  ADD_FAILURE();
}

TEST(ExternC, connect_async) {
  AsyncHelper helper;
  TestSeed seed;
  TestData data;
  seed.run();

  colonio_t colonio;
  colonio_error_t* err;

  err = colonio_init(&colonio);
  EXPECT_EQ(err, nullptr);

  data.helper  = &helper;
  data.colonio = &colonio;
  colonio.data = &data;

  colonio_connect_async(&colonio, URL, TOKEN, connect_async_on_success, connect_async_on_failure);

  helper.wait_signal("connect");

  err = colonio_disconnect(&colonio);
  EXPECT_EQ(err, nullptr);
}
