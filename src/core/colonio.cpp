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
#include "colonio/colonio.hpp"

#include <cassert>

#include "colonio_impl.hpp"

namespace colonio {
Colonio::Colonio() {
  impl = std::make_unique<ColonioImpl>(*this);
}

Colonio::~Colonio() {
}

Map& Colonio::access_map(const std::string& name) {
  return impl->access<Map>(name);
}

PubSub2D& Colonio::access_pubsub2d(const std::string& name) {
  return impl->access<PubSub2D>(name);
}

void Colonio::connect(
    const std::string& url, const std::string& token, std::function<void(Colonio&)> on_success,
    std::function<void(Colonio&)> on_failure) {
  assert(impl);
  impl->connect(url, token, [this, on_success]() { on_success(*this); }, [this, on_failure]() { on_failure(*this); });
}

void Colonio::disconnect() {
  assert(impl);
  impl->disconnect();
}

std::string Colonio::get_my_nid() {
  assert(impl);
  return impl->get_my_nid().to_str();
}

std::tuple<double, double> Colonio::set_position(double x, double y) {
  assert(impl);
  Coordinate pos(x, y);
  pos = impl->set_position(pos);
  return std::make_tuple(pos.x, pos.y);
}

void Colonio::on_output_log(LogLevel::Type level, const std::string& message) {
  // Drop log message.
}

void Colonio::on_debug_event(DebugEvent::Type event, const std::string& json) {
  // Drop debug event.
}

unsigned int Colonio::invoke() {
  assert(impl);
  return impl->context.scheduler.invoke();
}
}  // namespace colonio
