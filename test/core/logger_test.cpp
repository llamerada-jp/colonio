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

#include "core/logger.hpp"

#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "colonio/value.hpp"
#include "core/node_id.hpp"
#include "core/packet.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

class DummyModule : public LoggerDelegate {
 public:
  Logger logger;

  Logger* last_logger;
  std::string file;
  std::string level;
  unsigned int line;
  std::string message;
  picojson::object param;
  std::string time;

  DummyModule() : logger(*this) {
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
  DummyModule module1;
  DummyModule module2;
  Logger& logger = module1.logger;

  void test_output() {
    static const std::string TIME_REGEX("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4}$");

    // 異なるインスタンスは互いに独立していること
    logI(module1, "test0");
    logE(module2, "test1");
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::INFO, module1.level);
    EXPECT_EQ(__LINE__ - 5, static_cast<int>(module1.line));
    EXPECT_STREQ("test0", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), module1.param.size());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));

    EXPECT_EQ(&module2.logger, module2.last_logger);
    EXPECT_EQ(LogLevel::ERROR, module2.level);
    EXPECT_EQ(module1.line + 1, module2.line);
    EXPECT_STREQ("test1", module2.message.c_str());
    EXPECT_THAT(module2.time, MatchesRegex(TIME_REGEX));

    logi("test info 1");
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::INFO, module1.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(module1.line));
    EXPECT_STREQ("test info 1", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), module1.param.size());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));

    logI(module1, "test info 2");
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::INFO, module1.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(module1.line));
    EXPECT_STREQ("test info 2", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), module1.param.size());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));

    logw("test warn 1");
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::WARN, module1.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(module1.line));
    EXPECT_STREQ("test warn 1", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), module1.param.size());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));

    logW(module1, "test warn 2");
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::WARN, module1.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(module1.line));
    EXPECT_STREQ("test warn 2", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), module1.param.size());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));

    loge("test error 1");
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::ERROR, module1.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(module1.line));
    EXPECT_STREQ("test error 1", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), module1.param.size());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));

    logE(module1, "test error 2");
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::ERROR, module1.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(module1.line));
    EXPECT_STREQ("test error 2", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), module1.param.size());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));

    logd("test debug 1");
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::DEBUG, module1.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(module1.line));
    EXPECT_STREQ("test debug 1", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), module1.param.size());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));

    logD(module1, "test debug 2");
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::DEBUG, module1.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(module1.line));
    EXPECT_STREQ("test debug 2", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), module1.param.size());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));

    Packet packet(
        {NodeID::NONE, NodeID::NEXT, 0, 0, std::make_shared<std::string>(""), PacketMode::EXPLICIT, Channel::COLONIO,
         CommandID::SUCCESS});
    Value value(true);
    logI(module1, "test map")
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
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::INFO, module1.level);
    EXPECT_EQ(__LINE__ - 15, static_cast<int>(module1.line));
    EXPECT_STREQ("test map", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(11), module1.param.size());
    EXPECT_EQ("string value", module1.param.at("string").get<std::string>());
    EXPECT_EQ(NodeID::THIS.to_str(), module1.param.at("node id").get<std::string>());
    EXPECT_TRUE(module1.param.at("packet").is<picojson::object>());
    EXPECT_STREQ("true", module1.param.at("value").get<std::string>().c_str());
    EXPECT_TRUE(module1.param.at("json").is<picojson::array>());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));
    EXPECT_FALSE(module1.param.at("bool").get<bool>());
    EXPECT_STREQ("68 65 6c 6c 6f", module1.param.at("dump").get<std::string>().c_str());
    EXPECT_DOUBLE_EQ(2.0, module1.param.at("float").get<double>());
    EXPECT_EQ(-12, static_cast<int64_t>(module1.param.at("int").get<double>()));
    EXPECT_STREQ("fffffff4", module1.param.at("u32").get<std::string>().c_str());
    EXPECT_STREQ("000000000000000c", module1.param.at("u64").get<std::string>().c_str());

    logI(module1, "test NodeID").map("key", NodeID::THIS);
    EXPECT_EQ(&module1.logger, module1.last_logger);
    EXPECT_STREQ("logger_test.cpp", module1.file.c_str());
    EXPECT_EQ(LogLevel::INFO, module1.level);
    EXPECT_EQ(__LINE__ - 4, static_cast<int>(module1.line));
    EXPECT_STREQ("test NodeID", module1.message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(1), module1.param.size());
    EXPECT_EQ(NodeID::THIS.to_str(), module1.param.at("key").get<std::string>());
    EXPECT_THAT(module1.time, MatchesRegex(TIME_REGEX));
  }
};

TEST_F(LoggerTest, output) {
  test_output();
}
