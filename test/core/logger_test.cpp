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

#include "core/logger.hpp"

using namespace colonio;

class DummyContext : public LoggerDelegate {
  public:
  Logger logger;

  Logger* last_logger;
  LogLevel::Type last_level;
  std::string last_message;

  DummyContext() : logger(*this) {
  }

  void logger_on_output(Logger& logger, LogLevel::Type level, const std::string& message) override {
    last_logger = &logger;
    last_level  = level;
    last_message = message;
  }
};

class LoggerTest : public ::testing::Test {
  protected:
  DummyContext context;
  DummyContext context2;

  void test_output() {
    // 異なるインスタンスは互いに独立していること
    logI(context,  0, "test0");
    logE(context2, 1, "test1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::INFO, context.last_level);
    EXPECT_STREQ("[I] 48@logger_test 00000000 : test0", context.last_message.c_str());
    EXPECT_EQ(&context2.logger, context2.last_logger);
    EXPECT_EQ(LogLevel::ERROR, context2.last_level);
    EXPECT_STREQ("[E] 49@logger_test 00000001 : test1", context2.last_message.c_str());

    logi(2, "test2");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::INFO, context.last_level);
    EXPECT_STREQ("[I] 57@logger_test 00000002 : test2", context.last_message.c_str());

    loge(3, "test3");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::ERROR, context.last_level);
    EXPECT_STREQ("[E] 62@logger_test 00000003 : test3", context.last_message.c_str());

    logD(context, "test4");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::DEBUG, context.last_level);
    EXPECT_STREQ("[D] 67@logger_test : test4", context.last_message.c_str());

    logd("test5");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::DEBUG, context.last_level);
    EXPECT_STREQ("[D] 72@logger_test : test5", context.last_message.c_str());
  }
};

TEST_F(LoggerTest, output) {
  test_output();
}