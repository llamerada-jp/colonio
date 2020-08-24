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
#include <ctime>
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
    Logger& logger_, const std::string file_, unsigned long line_, const std::string& level_,
    const std::string& message_) :
    logger(logger_), file(file_), line(line_), level(level_), message(message_) {
}

Logger::L::~L() {
  picojson::object obj;
  obj.insert(std::make_pair(LogJSONKey::FILE, picojson::value(Utils::file_basename(file))));
  obj.insert(std::make_pair(LogJSONKey::LEVEL, picojson::value(level)));
  obj.insert(std::make_pair(LogJSONKey::LINE, picojson::value(static_cast<double>(line))));
  obj.insert(std::make_pair(LogJSONKey::MESSAGE, picojson::value(message)));
  obj.insert(std::make_pair(LogJSONKey::PARAM, picojson::value(params)));

  std::chrono::system_clock::time_point now = std::chrono::system_clock::now();
  std::time_t t                             = std::chrono::system_clock::to_time_t(now);
  std::tm lt;
  localtime_r(&t, &lt);
  std::stringstream ss;
  ss << std::put_time(&lt, "%FT%T%z");
  obj.insert(std::make_pair(LogJSONKey::TIME, picojson::value(ss.str())));

  logger.delegate.logger_on_output(logger, picojson::value(obj).serialize());
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

Logger::L& Logger::L::map(const std::string& name, const picojson::value& value) {
  params.insert(std::make_pair(name, value));
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

Logger::L Logger::create(
    const std::string& file, unsigned long line, const std::string& level, const std::string& message) {
  return L(*this, file, line, level, message);
}

}  // namespace colonio
