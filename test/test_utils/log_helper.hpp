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

#include <iostream>
#include <string>

#include "core/logger.hpp"

class LogHelper : public colonio::LoggerDelegate {
 public:
  std::string last_log;

  void logger_on_output(colonio::Logger& logger, const std::string& json) override {
    last_log = json;
    std::cout << json << std::endl;
  }
};

std::function<void(colonio::Colonio&, const std::string& message)> log_receiver(const std::string& name) {
  return [name](colonio::Colonio&, const std::string& message) {
    std::cout << name << ":" << message << std::endl;
  };
}

LogHelper log_helper;
colonio::Logger logger(log_helper);
