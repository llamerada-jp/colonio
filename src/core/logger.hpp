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
 * Logger is a class for summarizing log messages.
 */
class Logger {
 public:
  class L {
   public:
    L(std::function<void(const std::string& json)>& f, const std::string& file_, unsigned long line_,
      const std::string& level_, const std::string& message_);
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
    std::function<void(const std::string& json)>& logger_func;
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

  explicit Logger(const std::function<void(const std::string&)>& f);
  virtual ~Logger();

  L create(const std::string& file, unsigned long line, const std::string& level, const std::string& message);

 private:
  std::function<void(const std::string& json)> logger_func;
};

#define log_info(FORMAT, ...) \
  this->logger.create(__FILE__, __LINE__, LogLevel::INFO, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

#define log_warn(FORMAT, ...) \
  this->logger.create(__FILE__, __LINE__, LogLevel::WARN, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

#define log_error(FORMAT, ...) \
  this->logger.create(__FILE__, __LINE__, LogLevel::ERROR, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

#ifndef NDEBUG
#  define log_debug(FORMAT, ...) \
    this->logger.create(__FILE__, __LINE__, LogLevel::DEBUG, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

#else
#  define log_debug(...) Logger::D()
#endif
}  // namespace colonio
