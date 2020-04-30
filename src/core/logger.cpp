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
#include "logger.hpp"

#include <cassert>
#include <chrono>
#include <cstdarg>
#include <iomanip>
#include <sstream>
#include <vector>

#include "convert.hpp"
#include "node_id.hpp"
#include "packet.hpp"
#include "utils.hpp"
#include "value_impl.hpp"

namespace colonio {

LoggerDelegate::~LoggerDelegate() {
}

Logger::L::L(
    Logger& logger_, const std::string file_, unsigned long line_, LogLevel level_, const std::string& message_) :
    logger(logger_), file(file_), line(line_), level(level_), message(message_) {
}

Logger::L::~L() {
  std::stringstream stream;
  switch (level) {
    case LogLevel::INFO:
      stream << "[I]";
      break;
    case LogLevel::WARN:
      stream << "[W]";
      break;
    case LogLevel::ERROR:
      stream << "[E]";
      break;
    case LogLevel::DEBUG:
      stream << "[D]";
      break;
    default:
      assert(false);
      break;
  }

  std::chrono::system_clock::time_point now = std::chrono::system_clock::now();
  std::time_t t                             = std::chrono::system_clock::to_time_t(now);
  const std::tm* lt                         = std::localtime(&t);
  stream << " " << std::put_time(lt, "%FT%T%z") << " " << Utils::file_basename(file) << ":" << line << ": " << message;
  if (params.size() != 0) {
    stream << ": " << picojson::value(params).serialize();
  }

  logger.delegate.logger_on_output(logger, level, stream.str());
}

Logger::L& Logger::L::map(const std::string& name, const std::string& value) {
  params.insert(std::make_pair(name, picojson::value(value)));
  return *this;
}

Logger::L& Logger::L::map(const std::string& name, const NodeID& value) {
  params.insert(std::make_pair(name, picojson::value(value.to_str())));
  return *this;
}

Logger::L& Logger::L::map(const std::string& name, const Packet& value) {
  picojson::object p;
  p.insert(std::make_pair("dst_nid", picojson::value(value.dst_nid.to_str())));
  p.insert(std::make_pair("src_nid", picojson::value(value.src_nid.to_str())));
  p.insert(std::make_pair("id", picojson::value(Convert::int2str(value.id))));
  p.insert(std::make_pair("mode", picojson::value(Convert::int2str(value.mode))));
  p.insert(std::make_pair("channel", picojson::value(Convert::int2str(value.channel))));
  p.insert(std::make_pair("module_channel", picojson::value(Convert::int2str(value.module_channel))));
  p.insert(std::make_pair("command_id", picojson::value(Convert::int2str(value.command_id))));
  if (value.content) {
    p.insert(std::make_pair("content_size", picojson::value(static_cast<double>(value.content->size()))));
  } else {
    p.insert(std::make_pair("content", picojson::value()));
  }
  params.insert(std::make_pair(name, picojson::value(p)));
  return *this;
}

Logger::L& Logger::L::map(const std::string& name, const Value& value) {
  params.insert(std::make_pair(name, picojson::value(ValueImpl::to_str(value))));
  return *this;
}

Logger::L& Logger::L::map_bool(const std::string& name, bool value) {
  params.insert(std::make_pair(name, picojson::value(value)));
  return *this;
}

Logger::L& Logger::L::map_dump(const std::string& name, const std::string& value) {
  params.insert(std::make_pair(name, picojson::value(Utils::dump_binary(value))));
  return *this;
}

Logger::L& Logger::L::map_float(const std::string& name, double value) {
  params.insert(std::make_pair(name, picojson::value(value)));
  return *this;
}

Logger::L& Logger::L::map_int(const std::string& name, int64_t value) {
  params.insert(std::make_pair(name, picojson::value(static_cast<double>(value))));
  return *this;
}

Logger::L& Logger::L::map_u32(const std::string& name, uint32_t value) {
  params.insert(std::make_pair(name, picojson::value(Convert::int2str(value))));
  return *this;
}

Logger::L& Logger::L::map_u64(const std::string& name, uint64_t value) {
  params.insert(std::make_pair(name, picojson::value(Convert::int2str(value))));
  return *this;
}

Logger::Logger(LoggerDelegate& delegate_) : delegate(delegate_) {
}

Logger::~Logger() {
}

Logger::L Logger::create(const std::string& file, unsigned long line, LogLevel level, const std::string& message) {
  return L(*this, file, line, level, message);
}

}  // namespace colonio
