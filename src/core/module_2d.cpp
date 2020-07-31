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

#include "module_2d.hpp"

namespace colonio {

Module2DDelegate::~Module2DDelegate() {
}

Module2D::Module2D(
    Context& context, ModuleDelegate& module_delegate, Module2DDelegate& module_2d_delegate,
    const CoordSystem& coord_system_, APIChannel::Type channel, ModuleChannel::Type module_channel) :
    ModuleBase(context, module_delegate, channel, module_channel),
    coord_system(coord_system_),
    delegate(module_2d_delegate) {
}

const NodeID& Module2D::get_relay_nid(const Coordinate& position) {
  return delegate.module_2d_do_get_relay_nid(*this, position);
}
}  // namespace colonio
