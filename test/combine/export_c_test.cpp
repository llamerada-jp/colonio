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

extern "C" {
#include <colonio/colonio.h>
}

#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include <colonio/colonio.hpp>
#include <iostream>

#include "test_utils/all.hpp"

const char URL[]   = "http://localhost:8080/test";
const char TOKEN[] = "";

struct TestData {
  colonio_t colonio;
  AsyncHelper* helper;
};

TEST(ExternC, definition) {
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::UNDEFINED), COLONIO_ERROR_CODE_UNDEFINED);
  EXPECT_EQ(
      static_cast<int>(colonio::ErrorCode::SYSTEM_INCORRECT_DATA_FORMAT),
      COLONIO_ERROR_CODE_SYSTEM_INCORRECT_DATA_FORMAT);
  EXPECT_EQ(
      static_cast<int>(colonio::ErrorCode::SYSTEM_CONFLICT_WITH_SETTING),
      COLONIO_ERROR_CODE_SYSTEM_CONFLICT_WITH_SETTING);

  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::CONNECTION_FAILED), COLONIO_ERROR_CODE_CONNECTION_FAILED);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::CONNECTION_OFFLINE), COLONIO_ERROR_CODE_CONNECTION_OFFLINE);

  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::PACKET_NO_ONE_RECV), COLONIO_ERROR_CODE_PACKET_NO_ONE_RECV);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::PACKET_TIMEOUT), COLONIO_ERROR_CODE_PACKET_TIMEOUT);

  EXPECT_EQ(
      static_cast<int>(colonio::ErrorCode::MESSAGING_HANDLER_NOT_FOUND),
      COLONIO_ERROR_CODE_MESSAGING_HANDLER_NOT_FOUND);

  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::KVS_NOT_FOUND), COLONIO_ERROR_CODE_KVS_NOT_FOUND);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::KVS_PROHIBIT_OVERWRITE), COLONIO_ERROR_CODE_KVS_PROHIBIT_OVERWRITE);
  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::KVS_COLLISION), COLONIO_ERROR_CODE_KVS_COLLISION);

  EXPECT_EQ(static_cast<int>(colonio::ErrorCode::SPREAD_NO_ONE_RECEIVE), COLONIO_ERROR_CODE_SPREAD_NO_ONE_RECEIVE);

  EXPECT_STREQ(colonio::LogLevel::INFO.c_str(), COLONIO_LOG_LEVEL_INFO);
  EXPECT_STREQ(colonio::LogLevel::WARN.c_str(), COLONIO_LOG_LEVEL_WARN);
  EXPECT_STREQ(colonio::LogLevel::ERROR.c_str(), COLONIO_LOG_LEVEL_ERROR);
  EXPECT_STREQ(colonio::LogLevel::DEBUG.c_str(), COLONIO_LOG_LEVEL_DEBUG);
}

colonio_t instance_for_test_logger;

void log_receiver(colonio_t c, const char* message, unsigned int siz) {
  std::cout << std::string(message, siz) << std::endl;
  EXPECT_EQ(instance_for_test_logger, c);
}

TEST(ExportC, connect_sync) {
  TestSeed seed;
  seed.run();

  colonio_config_t config;
  colonio_config_set_default(&config);
  config.logger_func = log_receiver;

  colonio_t colonio;
  colonio_error_t* err;
  err = colonio_init(&colonio, &config);
  EXPECT_EQ(err, nullptr);

  instance_for_test_logger = colonio;

  err = colonio_connect(colonio, URL, strlen(URL), TOKEN, strlen(TOKEN));
  EXPECT_EQ(err, nullptr);

  EXPECT_TRUE(colonio_is_connected(colonio));

  {
    char nid[COLONIO_NID_LENGTH + 1] = {};
    colonio_get_local_nid(colonio, nid);
    EXPECT_EQ(strlen(nid), static_cast<unsigned int>(COLONIO_NID_LENGTH));
  }

  err = colonio_disconnect(colonio);
  EXPECT_EQ(err, nullptr);
  err = colonio_quit(&colonio);
  EXPECT_EQ(err, nullptr);
  EXPECT_EQ(colonio, nullptr);
}

void connect_async_on_success(colonio_t c, void* ptr) {
  TestData* data      = reinterpret_cast<TestData*>(ptr);
  AsyncHelper& helper = *data->helper;
  EXPECT_EQ(c, data->colonio);
  helper.pass_signal("connect");
}

void connect_async_on_failure(colonio_t, void*, const colonio_error_t*) {
  ADD_FAILURE();
}

TEST(ExportC, connect_async) {
  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  colonio_t colonio;
  colonio_error_t* err;

  colonio_config_t config;
  colonio_config_set_default(&config);
  err = colonio_init(&colonio, &config);
  EXPECT_EQ(err, nullptr);

  TestData data{.colonio = colonio, .helper = &helper};
  colonio_connect_async(
      colonio, URL, strlen(URL), TOKEN, strlen(TOKEN), &data, connect_async_on_success, connect_async_on_failure);

  helper.wait_signal("connect");

  err = colonio_disconnect(colonio);
  EXPECT_EQ(err, nullptr);
  err = colonio_quit(&colonio);
  EXPECT_EQ(err, nullptr);
}

TEST(ExportC, e2e) {
  TestSeed seed;
  seed.run();

  colonio_error_t* err;

  // init & connect colonio1
  colonio_config_t config1;
  colonio_config_set_default(&config1);

  colonio_t colonio1;
  err = colonio_init(&colonio1, &config1);
  EXPECT_EQ(err, nullptr);

  err = colonio_connect(colonio1, URL, strlen(URL), TOKEN, strlen(TOKEN));
  EXPECT_EQ(err, nullptr);

  // init & connect colonio2
  colonio_config_t config2;
  colonio_config_set_default(&config2);

  colonio_t colonio2;
  err = colonio_init(&colonio2, &config2);
  EXPECT_EQ(err, nullptr);

  err = colonio_connect(colonio2, URL, strlen(URL), TOKEN, strlen(TOKEN));
  EXPECT_EQ(err, nullptr);

  // disconnect & quit colonio1
  err = colonio_disconnect(colonio1);
  EXPECT_EQ(err, nullptr);
  err = colonio_quit(&colonio1);
  EXPECT_EQ(err, nullptr);

  // disconnect & quit colonio2
  err = colonio_disconnect(colonio2);
  EXPECT_EQ(err, nullptr);
  err = colonio_quit(&colonio2);
  EXPECT_EQ(err, nullptr);
}
