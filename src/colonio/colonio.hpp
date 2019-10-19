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

#include <colonio/constant.hpp>
#include <colonio/map.hpp>
#include <colonio/pubsub_2d.hpp>
#include <colonio/value.hpp>
#include <functional>
#include <memory>
#include <string>
#include <tuple>

namespace colonio {
class Colonio;
class ColonioImpl;

class Colonio {
 public:
  Colonio();
  virtual ~Colonio();

  Map& access_map(const std::string& name);
  PubSub2D& access_pubsub2d(const std::string& name);
  void connect(
      const std::string& url, const std::string& token, std::function<void(Colonio&)> on_success,
      std::function<void(Colonio&)> on_failure);
  void disconnect();
  std::string get_my_nid();
  std::tuple<double, double> set_position(double x, double y);

 protected:
  virtual void on_require_invoke(unsigned int msec) = 0;
  virtual void on_output_log(LogLevel::Type level, const std::string& message);
  virtual void on_debug_event(DebugEvent::Type event, const std::string& json);

  unsigned int invoke();

 private:
  friend ColonioImpl;
  std::unique_ptr<ColonioImpl> impl;

  Colonio(const Colonio&);
  Colonio& operator=(const Colonio&);
};
}  // namespace colonio
