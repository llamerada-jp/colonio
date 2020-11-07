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

#include "colonio/value.hpp"
#include "core/node_id.hpp"
#include "core/packet.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

class DummyContext : public LoggerDelegate {
 public:
  Logger logger;

  Logger* last_logger;
  std::string file;
  std::string level;
  unsigned int line;
  std::string message;
  picojson::object param;
  std::string time;

  DummyContext() : logger(*this) {
  }

  void logger_on_output(Logger& logger, const std::string& json) override {
    picojson::value v;
    std::string err = picojson::parse(v, json);
    if (!err.empty()) {
      std::cerr << err << std::endl;
    }

    picojson::object& o = v.get<picojson::object>();

    last_logger = &logger;
    file        = o.at(LogJSONKey::FILE).get<std::string>();
    level       = o.at(LogJSONKey::LEVEL).get<std::string>();
    line        = static_cast<unsigned int>(o.at(LogJSONKey::LINE).get<double>());
    message     = o.at(LogJSONKey::MESSAGE).get<std::string>();
    param       = o.at(LogJSONKey::PARAM).get<picojson::object>();
    time        = o.at(LogJSONKey::TIME).get<std::string>();
  }
};

class LoggerTest : public ::testing::Test {
 protected:
  DummyContext context;
  DummyContext context2;

  void test_output() {
    static const std::string TIME_REGEX("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4}$");

    // 異なるインスタンスは互いに独立していること
    logI(context, "test0");
    logE(context2, "test1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::INFO, context.level);
    EXPECT_EQ(__LINE__ - 5, static_cast<int>(context.line));
    EXPECT_STREQ("test0", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), context.param.size());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));

    EXPECT_EQ(&context2.logger, context2.last_logger);
    EXPECT_EQ(LogLevel::ERROR, context2.level);
    EXPECT_EQ(context.line + 1, context2.line);
    EXPECT_STREQ("test1", context2.message.c_str());
    EXPECT_THAT(context2.time, MatchesRegex(TIME_REGEX));

    logi("test info 1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::INFO, context.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(context.line));
    EXPECT_STREQ("test info 1", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), context.param.size());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));

    logI(context, "test info 2");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::INFO, context.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(context.line));
    EXPECT_STREQ("test info 2", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), context.param.size());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));

    logw("test warn 1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::WARN, context.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(context.line));
    EXPECT_STREQ("test warn 1", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), context.param.size());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));

    logW(context, "test warn 2");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::WARN, context.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(context.line));
    EXPECT_STREQ("test warn 2", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), context.param.size());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));

    loge("test error 1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::ERROR, context.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(context.line));
    EXPECT_STREQ("test error 1", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), context.param.size());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));

    logE(context, "test error 2");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::ERROR, context.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(context.line));
    EXPECT_STREQ("test error 2", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), context.param.size());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));

    logd("test debug 1");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::DEBUG, context.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(context.line));
    EXPECT_STREQ("test debug 1", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), context.param.size());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));

    logD(context, "test debug 2");
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::DEBUG, context.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(context.line));
    EXPECT_STREQ("test debug 2", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), context.param.size());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));

    Packet packet(
        {NodeID::NONE, NodeID::NEXT, 0, 0, std::make_shared<std::string>(""), PacketMode::EXPLICIT, APIChannel::COLONIO,
         ModuleChannel::Colonio::MAIN, CommandID::SUCCESS});
    Value value(true);
    logI(context, "test map")
        .map("string", std::string("string value"))
        .map("node id", NodeID::THIS)
        .map("packet", packet)
        .map("value", value)
        .map("json", picojson::value(picojson::array()))
        .map_bool("bool", false)
        .map_dump("dump", std::string("hello"))
        .map_float("float", 2.0)
        .map_int("int", -12)
        .map_u32("u32", -12)
        .map_u64("u64", 12);
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::INFO, context.level);
    EXPECT_EQ(__LINE__ - 15, static_cast<int>(context.line));
    EXPECT_STREQ("test map", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(11), context.param.size());
    EXPECT_EQ("string value", context.param.at("string").get<std::string>());
    EXPECT_EQ(NodeID::THIS.to_str(), context.param.at("node id").get<std::string>());
    EXPECT_TRUE(context.param.at("packet").is<picojson::object>());
    EXPECT_STREQ("true", context.param.at("value").get<std::string>().c_str());
    EXPECT_TRUE(context.param.at("json").is<picojson::array>());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));
    EXPECT_FALSE(context.param.at("bool").get<bool>());
    EXPECT_STREQ("68 65 6c 6c 6f", context.param.at("dump").get<std::string>().c_str());
    EXPECT_DOUBLE_EQ(2.0, context.param.at("float").get<double>());
    EXPECT_EQ(-12, static_cast<int64_t>(context.param.at("int").get<double>()));
    EXPECT_STREQ("fffffff4", context.param.at("u32").get<std::string>().c_str());
    EXPECT_STREQ("000000000000000c", context.param.at("u64").get<std::string>().c_str());

    logI(context, "test NodeID").map("key", NodeID::THIS);
    EXPECT_EQ(&context.logger, context.last_logger);
    EXPECT_STREQ("logger_test.cpp", context.file.c_str());
    EXPECT_EQ(LogLevel::INFO, context.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(context.line));
    EXPECT_STREQ("test NodeID", context.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(1), context.param.size());
    EXPECT_EQ(NodeID::THIS.to_str(), context.param.at("key").get<std::string>());
    EXPECT_THAT(context.time, MatchesRegex(TIME_REGEX));
  }
};

TEST_F(LoggerTest, output) {
  test_output();
}
