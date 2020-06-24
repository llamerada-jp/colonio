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
#pragma once

#include <functional>
#include <list>
#include <memory>

#include "coord_system.hpp"
#include "coordinate.hpp"
#include "definition.hpp"
#include "node_id.hpp"

namespace colonio {
class Logger;
class Scheduler;

class Context {
 public:
  LinkStatus::Type link_status;
  Logger& logger;
  Scheduler& scheduler;
  std::unique_ptr<CoordSystem> coord_system;
  const NodeID local_nid;

  Context(Logger& logger_, Scheduler& scheduler_);

  Coordinate get_local_position();
  bool has_local_position();
  void hook_on_change_local_position(std::function<void(const Coordinate&)> func);
  void set_local_position(const Coordinate& pos);

 private:
  std::list<std::function<void(const Coordinate&)>> funcs_on_change_local_position;

  Context(const Context&);
  void operator=(const Context&);
};
}  // namespace colonio
