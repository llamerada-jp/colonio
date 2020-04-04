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

#include "core/logger.hpp"

#include <gmock/gmock.h>
#include <gtest/gtest.h>

using namespace colonio;
using ::testing::MatchesRegex;

class DummyContext : public LoggerDelegate {
 public:
  Logger logger;

  Logger* last_logger;
  LogLevel::Type last_level;
  std::string last_message;

  DummyContext() : logger(*this) {
  }

  void logger_on_output(Logger& logger, LogLevel::Type level, const std::string& message) override {
    last_logger  = &logger;
    last_level   = level;
    last_message = message;
  }
};

class LoggerTest : public ::testing::Test {
 protected:
  DummyContext context;
  DummyContext context2;

  void test_output() {
    // 異なるインスタンスは互いに独立していること
    logI(context, "test0");
    logE(context2, "test1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::INFO, context.last_level);
    EXPECT_THAT(
        context.last_message,
        MatchesRegex(
            "^\\[I] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} logger_test\\.cpp:50: test0$"));
    EXPECT_EQ(&context2.logger, context2.last_logger);
    EXPECT_EQ(LogLevel::ERROR, context2.last_level);
    EXPECT_THAT(
        context2.last_message,
        MatchesRegex(
            "^\\[E] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} logger_test\\.cpp:51: test1$"));

    logi("test info 1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::INFO, context.last_level);
    EXPECT_THAT(
        context.last_message, MatchesRegex("^\\[I] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} "
                                           "logger_test\\.cpp:65: test info 1$"));

    logI(context, "test info 2");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::INFO, context.last_level);
    EXPECT_THAT(
        context.last_message, MatchesRegex("^\\[I] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} "
                                           "logger_test\\.cpp:72: test info 2$"));

    logw("test warn 1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::WARN, context.last_level);
    EXPECT_THAT(
        context.last_message, MatchesRegex("^\\[W] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} "
                                           "logger_test\\.cpp:79: test warn 1$"));

    logW(context, "test warn 2");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::WARN, context.last_level);
    EXPECT_THAT(
        context.last_message, MatchesRegex("^\\[W] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} "
                                           "logger_test\\.cpp:86: test warn 2$"));

    loge("test error 1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::ERROR, context.last_level);
    EXPECT_THAT(
        context.last_message, MatchesRegex("^\\[E] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} "
                                           "logger_test\\.cpp:93: test error 1$"));

    logE(context, "test error 2");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::ERROR, context.last_level);
    EXPECT_THAT(
        context.last_message, MatchesRegex("^\\[E] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} "
                                           "logger_test\\.cpp:100: test error 2$"));

    logd("test debug 1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::DEBUG, context.last_level);
    EXPECT_THAT(
        context.last_message, MatchesRegex("^\\[D] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} "
                                           "logger_test\\.cpp:107: test debug 1$"));

    logD(context, "test debug 2");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_EQ(LogLevel::DEBUG, context.last_level);
    EXPECT_THAT(
        context.last_message, MatchesRegex("^\\[D] [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4} "
                                           "logger_test\\.cpp:114: test debug 2$"));
  }
};

TEST_F(LoggerTest, output) {
  test_output();
}
