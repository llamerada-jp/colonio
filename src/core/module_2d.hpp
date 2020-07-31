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

#include <set>

#include "module_base.hpp"

namespace colonio {
class Coordinate;
class CoordSystem;
class Module2D;

class Module2DDelegate {
 public:
  virtual ~Module2DDelegate();
  virtual const NodeID& module_2d_do_get_relay_nid(Module2D& module_2d, const Coordinate& position) = 0;
};

class Module2D : public ModuleBase {
 public:
  virtual void module_2d_on_change_local_position(const Coordinate& position)                     = 0;
  virtual void module_2d_on_change_nearby(const std::set<NodeID>& nids)                           = 0;
  virtual void module_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) = 0;

 protected:
  const CoordSystem& coord_system;

  Module2D(
      Context& context, ModuleDelegate& module_delegate, Module2DDelegate& module_2d_delegate,
      const CoordSystem& coord_system_, APIChannel::Type channel, ModuleChannel::Type module_channel);

  const NodeID& get_relay_nid(const Coordinate& position);

 private:
  Module2DDelegate& delegate;
};
}  // namespace colonio
