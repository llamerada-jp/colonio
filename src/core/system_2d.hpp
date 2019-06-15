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

#include "context.hpp"
#include "module.hpp"

namespace colonio {

class System2DDelegate {
 public:
  virtual ~System2DDelegate();
  virtual const NodeID& system_2d_do_get_relay_nid(const Coordinate& position) = 0;
};

class System2DBase : public Module {
 public:
  virtual void system_2d_on_change_my_position(const Coordinate& position) = 0;
  virtual void system_2d_on_change_nearby(const std::set<NodeID>& nids) = 0;
  virtual void system_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) = 0;

 protected:
  System2DBase(Context& context, ModuleDelegate& module_delegate,
               System2DDelegate& system_delegate, ModuleChannel::Type channel);

  const NodeID& get_relay_nid(const Coordinate& position);
  
 private:
  System2DDelegate& delegate;
};

template <class BASE> class System2D : public BASE,
                                       public System2DBase {
 protected:
  System2D(Context& context, ModuleDelegate& module_delegate,
           System2DDelegate& system_delegate, ModuleChannel::Type channel) :
      System2DBase(context, module_delegate, system_delegate, channel) {
  }
};
}  // namespace colonio
