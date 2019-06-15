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
#pragma once

#include <string>

#include "definition.hpp"

namespace colonio {
class Logger;
class LoggerDelegate {
 public:
  virtual ~LoggerDelegate();
  virtual void logger_on_output(Logger& logger, LogLevel::Type level, const std::string& message) = 0;
};

class Logger {
 public:
  Logger(LoggerDelegate& delegate_);
  virtual ~Logger();

  void output(const std::string& file, unsigned long line,
              LogLevel::Type level, unsigned long mid, const std::string& format, int dummy, ...);

 private:
  LoggerDelegate& delegate;
};

#define logi(MID, FORMAT, ...)                                          \
  this->context.logger.output(__FILE__, __LINE__, LogLevel::INFO, MID, FORMAT, 0, ##__VA_ARGS__)
#define logI(INSTANCE, MID, FORMAT, ...)                                \
  INSTANCE.logger.output(__FILE__, __LINE__, LogLevel::INFO, MID, FORMAT, 0, ##__VA_ARGS__)

#define loge(MID, FORMAT, ...)                                          \
  this->context.logger.output(__FILE__, __LINE__, LogLevel::ERROR, MID, FORMAT, 0, ##__VA_ARGS__)
#define logE(INSTANCE, MID, FORMAT, ...)                                \
  INSTANCE.logger.output(__FILE__, __LINE__, LogLevel::ERROR, MID, FORMAT, 0, ##__VA_ARGS__)

#ifndef NDEBUG
#  define logd(FORMAT, ...)                                             \
  this->context.logger.output(__FILE__, __LINE__, LogLevel::DEBUG, 0, FORMAT, 0, ##__VA_ARGS__)
#  define logD(INSTANCE, FORMAT, ...)                                   \
  INSTANCE.logger.output(__FILE__, __LINE__, LogLevel::DEBUG, 0, FORMAT, 0, ##__VA_ARGS__)

#else
inline void do_nothing() {}
#  define logd(...) do_nothing()
#  define logD(...) do_nothing()
#endif
}  // namespace colonio
