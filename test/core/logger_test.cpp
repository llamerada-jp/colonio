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

#include "core/node_id.hpp"
#include "core/packet.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

static std::string file;
static std::string level;
static unsigned int line;
static std::string message;
static picojson::object param;
static std::string log_time;

void output(const std::string& json) {
  picojson::value v;
  std::string err = picojson::parse(v, json);
  if (!err.empty()) {
    std::cerr << err << std::endl;
  }

  picojson::object& o = v.get<picojson::object>();

  file     = o.at("file").get<std::string>();
  level    = o.at("level").get<std::string>();
  line     = static_cast<unsigned int>(o.at("line").get<double>());
  message  = o.at("message").get<std::string>();
  param    = o.at("param").get<picojson::object>();
  log_time = o.at("time").get<std::string>();
}

Logger l(output);

class LoggerTest : public ::testing::Test {
 protected:
  Logger& logger = l;

  void test_output() {
    static const std::string TIME_REGEX("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\+[0-9]{4}$");

    log_info("test info 1");
    EXPECT_STREQ("logger_test.cpp", file.c_str());
    EXPECT_EQ(LogLevel::INFO, level);
    EXPECT_EQ(__LINE__ - 3, static_cast<int>(line));
    EXPECT_STREQ("test info 1", message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), param.size());
    EXPECT_THAT(log_time, MatchesRegex(TIME_REGEX));

    log_warn("test warn 1");
    EXPECT_STREQ("logger_test.cpp", file.c_str());
    EXPECT_EQ(LogLevel::WARN, level);
    EXPECT_EQ(__LINE__ - 3, static_cast<int>(line));
    EXPECT_STREQ("test warn 1", message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), param.size());
    EXPECT_THAT(log_time, MatchesRegex(TIME_REGEX));

    log_error("test error 1");
    EXPECT_STREQ("logger_test.cpp", file.c_str());
    EXPECT_EQ(LogLevel::ERROR, level);
    EXPECT_EQ(__LINE__ - 3, static_cast<int>(line));
    EXPECT_STREQ("test error 1", message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), param.size());
    EXPECT_THAT(log_time, MatchesRegex(TIME_REGEX));

    log_debug("test debug 1");
    EXPECT_STREQ("logger_test.cpp", file.c_str());
    EXPECT_EQ(LogLevel::DEBUG, level);
    EXPECT_EQ(__LINE__ - 3, static_cast<int>(line));
    EXPECT_STREQ("test debug 1", message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(0), param.size());
    EXPECT_THAT(log_time, MatchesRegex(TIME_REGEX));

    Packet packet(
        NodeID::NONE, NodeID::NEXT, 0, 0, std::make_shared<Packet::Content>(std::make_unique<const std::string>()),
        PacketMode::EXPLICIT);
    Value value(true);
    log_info("test map")
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
    EXPECT_STREQ("logger_test.cpp", file.c_str());
    EXPECT_EQ(LogLevel::INFO, level);
    EXPECT_EQ(__LINE__ - 14, static_cast<int>(line));
    EXPECT_STREQ("test map", message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(11), param.size());
    EXPECT_EQ("string value", param.at("string").get<std::string>());
    EXPECT_EQ(NodeID::THIS.to_str(), param.at("node id").get<std::string>());
    EXPECT_TRUE(param.at("packet").is<picojson::object>());
    EXPECT_STREQ("true", param.at("value").get<std::string>().c_str());
    EXPECT_TRUE(param.at("json").is<picojson::array>());
    EXPECT_THAT(log_time, MatchesRegex(TIME_REGEX));
    EXPECT_FALSE(param.at("bool").get<bool>());
    EXPECT_STREQ("68 65 6c 6c 6f", param.at("dump").get<std::string>().c_str());
    EXPECT_DOUBLE_EQ(2.0, param.at("float").get<double>());
    EXPECT_EQ(-12, static_cast<int64_t>(param.at("int").get<double>()));
    EXPECT_STREQ("fffffff4", param.at("u32").get<std::string>().c_str());
    EXPECT_STREQ("000000000000000c", param.at("u64").get<std::string>().c_str());

    log_info("test NodeID").map("key", NodeID::THIS);
    EXPECT_STREQ("logger_test.cpp", file.c_str());
    EXPECT_EQ(LogLevel::INFO, level);
    EXPECT_EQ(__LINE__ - 3, static_cast<int>(line));
    EXPECT_STREQ("test NodeID", message.c_str());
    EXPECT_EQ(static_cast<unsigned int>(1), param.size());
    EXPECT_EQ(NodeID::THIS.to_str(), param.at("key").get<std::string>());
    EXPECT_THAT(log_time, MatchesRegex(TIME_REGEX));
  }
};

TEST_F(LoggerTest, output) {
  test_output();
}
