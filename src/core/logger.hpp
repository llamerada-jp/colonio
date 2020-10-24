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
#pragma once

#include <memory>
#include <string>

#include "definition.hpp"
#include "utils.hpp"

namespace colonio {
class NodeID;
class Packet;
class Value;

class Logger;
/**
 * LoggerDelegate is a delegate for Logger.
 * The upper module receives log message by implementing methods on the delegate.
 */
class LoggerDelegate {
 public:
  virtual ~LoggerDelegate();

  /**
   * It is the receiver for log messages from Logger.
   * @param logger Logger instance that sends a log message.
   * @param json JSON format log message.
   */
  virtual void logger_on_output(Logger& logger, const std::string& json) = 0;
};

/**
 * Logger is a class for summarizing log messages.
 * It instance put on the Context class, and other modules pass log messages by using it.
 */
class Logger {
 public:
  class L {
   public:
    L(Logger& logger_, const std::string& file_, unsigned long line_, const std::string& level_,
      const std::string& message_);
    virtual ~L();
    L& map(const std::string& name, const std::string& value);
    L& map(const std::string& name, const NodeID& value);
    L& map(const std::string& name, const Packet& value);
    L& map(const std::string& name, const Value& value);
    L& map(const std::string& name, const picojson::value& value);
    L& map_bool(const std::string& name, bool value);
    L& map_dump(const std::string& name, const std::string& value);
    L& map_float(const std::string& name, double value);
    L& map_int(const std::string& name, int64_t value);
    L& map_u32(const std::string& name, uint32_t value);
    L& map_u64(const std::string& name, uint64_t value);

   private:
    Logger& logger;
    std::string file;
    unsigned long line;
    std::string level;
    const std::string message;
    picojson::object params;
  };

  class D {
   public:
    D& map(const std::string& name, const std::string& value) {
      return *this;
    }

    D& map(const std::string& name, const NodeID& value) {
      return *this;
    }

    D& map(const std::string& name, const Packet& value) {
      return *this;
    }

    D& map(const std::string& name, const Value& value) {
      return *this;
    }

    D& map(const std::string& name, const picojson::value& value) {
      return *this;
    }

    D& map_bool(const std::string& name, bool value) {
      return *this;
    }

    D& map_dump(const std::string& name, const std::string& value) {
      return *this;
    }

    D& map_float(const std::string& name, double value) {
      return *this;
    }

    D& map_int(const std::string& name, int64_t value) {
      return *this;
    }

    D& map_u32(const std::string& name, uint32_t value) {
      return *this;
    }

    D& map_u64(const std::string& name, uint64_t value) {
      return *this;
    }
  };

  explicit Logger(LoggerDelegate& delegate_);
  virtual ~Logger();

  L create(const std::string& file, unsigned long line, const std::string& level, const std::string& message);

 private:
  LoggerDelegate& delegate;
};

#define logi(FORMAT, ...) \
  this->context.logger.create(__FILE__, __LINE__, LogLevel::INFO, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))
#define logI(INSTANCE, FORMAT, ...) \
  (INSTANCE).logger.create(__FILE__, __LINE__, LogLevel::INFO, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

#define logw(FORMAT, ...) \
  this->context.logger.create(__FILE__, __LINE__, LogLevel::WARN, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))
#define logW(INSTANCE, FORMAT, ...) \
  (INSTANCE).logger.create(__FILE__, __LINE__, LogLevel::WARN, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

#define loge(FORMAT, ...) \
  this->context.logger.create(__FILE__, __LINE__, LogLevel::ERROR, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))
#define logE(INSTANCE, FORMAT, ...) \
  (INSTANCE).logger.create(__FILE__, __LINE__, LogLevel::ERROR, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

#ifndef NDEBUG
#  define logd(FORMAT, ...) \
    this->context.logger.create(__FILE__, __LINE__, LogLevel::DEBUG, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))
#  define logD(INSTANCE, FORMAT, ...) \
    (INSTANCE).logger.create(__FILE__, __LINE__, LogLevel::DEBUG, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

#else
#  define logd(...) Logger::D()
#  define logD(...) Logger::D()
#endif
}  // namespace colonio
