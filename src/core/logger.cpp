/*
 * Copyright 2017-2019 Yuji Ito <llamerada.jp@gmail.com>
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
#include <cassert>
#include <cstdarg>
#include <iomanip>
#include <sstream>
#include <vector>

#include "logger.hpp"
#include "utils.hpp"

namespace colonio {

LoggerDelegate::~LoggerDelegate() {
}

Logger::Logger(LoggerDelegate& delegate_) :
    delegate(delegate_) {
}

Logger::~Logger() {
}

void Logger::output(const std::string& file, unsigned long line,
                    LogLevel::Type level, unsigned long mid, const std::string& format, int dummy, ...) {
  std::vector<char> buffer(format.size() * 1.5);
  int r = 0;
  va_list args;
  va_list args_copy;
  va_start(args, dummy);
  va_copy(args_copy, args);
  while ((r = vsnprintf(buffer.data(), buffer.size(), format.c_str(), args_copy)) >= 0 &&
         static_cast<unsigned int>(r) >= buffer.size()) {
    buffer.resize(r + 1);
    va_end(args_copy);
    va_copy(args_copy, args);
  }
  va_end(args);
  va_end(args_copy);

  std::stringstream stream;
  switch (level) {
    case LogLevel::INFO:  stream << "[I]"; break;
    case LogLevel::ERROR: stream << "[E]"; break;
    case LogLevel::DEBUG: stream << "[D]"; break;
    default: assert(false); break;
  }

#ifndef NDEBUG
  stream << " " << line << "@" << Utils::file_basename(file, true);
#endif
  
  if (level != LogLevel::DEBUG) {
    stream << " " << std::setw(8) << std::setfill('0') <<  std::hex << mid;
  }

  stream << " : " << buffer.data();
  
  delegate.logger_on_output(*this, level, stream.str());
}
}  // namespace colonio
