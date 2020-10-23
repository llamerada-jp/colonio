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

#include <colonio/colonio.hpp>

#include "test_utils/all.hpp"

const char URL[]   = "http://localhost:8080/test";
const char TOKEN[] = "";

struct TestData {
  AsyncHelper* helper;
  colonio_t* colonio;
};

TEST(ExternC, definition) {
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::UNDEFINED), COLONIO_ERROR_CODE_UNDEFINED);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::SYSTEM_ERROR), COLONIO_ERROR_CODE_SYSTEM_ERROR);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::OFFLINE), COLONIO_ERROR_CODE_OFFLINE);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::INCORRECT_DATA_FORMAT), COLONIO_ERROR_CODE_INCORRECT_DATA_FORMAT);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::CONFLICT_WITH_SETTING), COLONIO_ERROR_CODE_CONFLICT_WITH_SETTING);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::NOT_EXIST_KEY), COLONIO_ERROR_CODE_NOT_EXIST_KEY);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::EXIST_KEY), COLONIO_ERROR_CODE_EXIST_KEY);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::CHANGED_PROPOSER), COLONIO_ERROR_CODE_CHANGED_PROPOSER);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::COLLISION_LATE), COLONIO_ERROR_CODE_COLLISION_LATE);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::NO_ONE_RECV), COLONIO_ERROR_CODE_NO_ONE_RECV);

  EXPECT_STREQ(colonio::LogLevel::INFO.c_str(), COLONIO_LOG_LEVEL_INFO);
  EXPECT_STREQ(colonio::LogLevel::WARN.c_str(), COLONIO_LOG_LEVEL_WARN);
  EXPECT_STREQ(colonio::LogLevel::ERROR.c_str(), COLONIO_LOG_LEVEL_ERROR);
  EXPECT_STREQ(colonio::LogLevel::DEBUG.c_str(), COLONIO_LOG_LEVEL_DEBUG);
}

TEST(ExternC, connect_sync) {
  TestSeed seed;
  seed.run();

  colonio_t colonio;
  colonio_error_t* err;

  err = colonio_init(&colonio);
  EXPECT_EQ(err, nullptr);

  err = colonio_connect(&colonio, URL, strlen(URL), TOKEN, strlen(TOKEN));
  EXPECT_EQ(err, nullptr);

  {
    char nid[COLONIO_NID_LENGTH + 1];
    unsigned int nid_str_len;
    colonio_get_local_nid(&colonio, nid, &nid_str_len);
    EXPECT_EQ(strlen(nid), COLONIO_NID_LENGTH);
    EXPECT_EQ(nid_str_len, COLONIO_NID_LENGTH);
  }

  err = colonio_disconnect(&colonio);
  EXPECT_EQ(err, nullptr);
  err = colonio_quit(&colonio);
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

  colonio_connect_async(
      &colonio, URL, strlen(URL), TOKEN, strlen(TOKEN), connect_async_on_success, connect_async_on_failure);

  helper.wait_signal("connect");

  err = colonio_disconnect(&colonio);
  EXPECT_EQ(err, nullptr);
  err = colonio_quit(&colonio);
  EXPECT_EQ(err, nullptr);
}
