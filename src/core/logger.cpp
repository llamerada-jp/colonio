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
#include <cstdarg>
#include <iomanip>
#include <sstream>
#include <vector>

#include "utils.hpp"

namespace colonio {

LoggerDelegate::~LoggerDelegate() {
}

Logger::Logger(LoggerDelegate& delegate_) : delegate(delegate_) {
}

Logger::~Logger() {
}

void Logger::output(
    const std::string& file, unsigned long line, LogLevel::Type level, unsigned long mid, const std::string& message) {
  std::stringstream stream;
  switch (level) {
    case LogLevel::INFO:
      stream << "[I]";
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

#ifndef NDEBUG
  stream << " " << line << "@" << Utils::file_basename(file, true);
#endif

  if (level != LogLevel::DEBUG) {
    stream << " " << std::setw(8) << std::setfill('0') << std::hex << mid;
  }

  stream << " : " << message;

  delegate.logger_on_output(*this, level, stream.str());
}
}  // namespace colonio
