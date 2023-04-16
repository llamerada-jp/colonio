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

#include <functional>
#include <iostream>
#include <string>

#include "colonio/colonio.hpp"
#include "core/logger.hpp"

std::string last_log;

void log_receiver(const std::string& json) {
  std::cout << json << std::endl;
  last_log = json;
}

colonio::Logger logger(log_receiver);

colonio::ColonioConfig make_config_with_name(const std::string& name) {
  colonio::ColonioConfig config;
  config.disable_seed_verification = true;
  config.logger_func               = [name](colonio::Colonio& _, const std::string& json) {
    std::cout << name << ":" << json << std::endl;
  };
  return config;
}