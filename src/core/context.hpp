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

#include <functional>
#include <list>

#include "coordinate.hpp"
#include "logger.hpp"
#include "node_id.hpp"
#include "scheduler.hpp"

namespace colonio {
class CoordSystem;
class Context {
 public:
  LinkStatus::Type link_status;
  Logger logger;
  Scheduler scheduler;
  std::unique_ptr<CoordSystem> coord_system;
  const NodeID my_nid;

  Context(LoggerDelegate& logger_delegate, SchedulerDelegate& sched_delegate);

  static uint32_t get_rnd_32();
  static uint64_t get_rnd_64();

  Coordinate get_my_position();
  bool has_my_position();
  void hook_on_change_my_position(std::function<void(const Coordinate&)> func);
  void set_my_position(const Coordinate& pos);

#ifndef NDEBUG
  void hook_on_debug_event(std::function<void(DebugEvent::Type type, const picojson::value& data)> cb);
  void debug_event(DebugEvent::Type type, const picojson::value& data);
#endif

 private:
  std::list<std::function<void(const Coordinate&)>> funcs_on_change_my_position;

#ifndef NDEBUG
  bool enable_debug_event;
  std::function <void(DebugEvent::Type type, const picojson::value& data)> func_on_debug_event;
#endif
  
  Context(const Context&);
  void operator=(const Context&);
};
}  // namespace colonio
