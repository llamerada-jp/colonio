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
#include "logger.hpp"

#include <chrono>
#include <cstdarg>
#include <ctime>
#include <iomanip>
#include <sstream>

#include "convert.hpp"
#include "node_id.hpp"
#include "packet.hpp"
#include "utils.hpp"
#include "value_impl.hpp"

namespace colonio {
Logger::L::L(
    std::function<void(const std::string& json)>& f, const std::string& file_, unsigned long line_,
    const std::string& level_, const std::string& message_) :
    logger_func(f), file(file_), line(line_), level(level_), message(message_) {
}

Logger::L::~L() {
  picojson::object obj;
  obj.insert(std::make_pair("file", picojson::value(Utils::file_basename(file))));
  obj.insert(std::make_pair("level", picojson::value(level)));
  obj.insert(std::make_pair("line", picojson::value(static_cast<double>(line))));
  obj.insert(std::make_pair("message", picojson::value(message)));
  obj.insert(std::make_pair("param", picojson::value(params)));

  std::chrono::system_clock::time_point now = std::chrono::system_clock::now();
  std::time_t t                             = std::chrono::system_clock::to_time_t(now);
  std::tm lt;
  localtime_r(&t, &lt);
  std::stringstream ss;
  ss << std::put_time(&lt, "%FT%T%z");
  obj.insert(std::make_pair("time", picojson::value(ss.str())));

  logger_func(picojson::value(obj).serialize());
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
  p.insert(std::make_pair("hop_count", picojson::value(Convert::int2str(value.hop_count))));
  p.insert(std::make_pair("content", picojson::value(value.content->as_proto().DebugString())));
  p.insert(std::make_pair("mode", picojson::value(Convert::int2str(value.mode))));
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
  params.insert(std::make_pair(name, picojson::value(Utils::dump_binary(&value))));
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

Logger::Logger(const std::function<void(const std::string&)>& f) : logger_func(f) {
}

Logger::~Logger() {
}

Logger::L Logger::create(
    const std::string& file, unsigned long line, const std::string& level, const std::string& message) {
  return L(logger_func, file, line, level, message);
}
}  // namespace colonio
