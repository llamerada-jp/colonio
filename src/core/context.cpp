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
#include "context.hpp"

#include <cassert>

#include "coord_system.hpp"
#include "logger.hpp"

namespace colonio {

Context::Context(Logger& logger_, Scheduler& scheduler_) :
    link_status(LinkStatus::OFFLINE),
    logger(logger_),
    scheduler(scheduler_),
    local_nid(NodeID::make_random()) {
#ifndef NDEBUG
  enable_debug_event = false;
#endif
}

Coordinate Context::get_my_position() {
  assert(coord_system);

  return coord_system->get_my_position();
}

bool Context::has_my_position() {
  assert(coord_system);

  return coord_system->get_my_position().is_enable();
}

void Context::hook_on_change_my_position(std::function<void(const Coordinate&)> func) {
  funcs_on_change_my_position.push_back(func);
}

void Context::set_my_position(const Coordinate& pos) {
  assert(coord_system);

  Coordinate prev_my_position = coord_system->get_my_position();
  coord_system->set_my_position(pos);
  Coordinate new_my_position = coord_system->get_my_position();

  if (prev_my_position.x != new_my_position.x || prev_my_position.y != new_my_position.y) {
    logI((*this), 0x00020002, "Change my position.(x=%f, y=%f)", new_my_position.x, new_my_position.y);

    for (auto& it : funcs_on_change_my_position) {
      it(new_my_position);
    }
  }
}

#ifndef NDEBUG
void Context::hook_on_debug_event(std::function<void(DebugEvent::Type type, const picojson::value& data)> cb) {
  func_on_debug_event = cb;
  enable_debug_event  = true;
}

void Context::debug_event(DebugEvent::Type type, const picojson::value& data) {
  if (enable_debug_event) {
    func_on_debug_event(type, data);
  }
}
#endif
}  // namespace colonio
