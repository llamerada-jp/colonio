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

#include "module_base.hpp"

namespace colonio {
class Context;
class NodeID;
class Module1D;

class Module1DDelegate {
 public:
  virtual ~Module1DDelegate();
  virtual bool module_1d_do_check_covered_range(Module1D& module_1d, const NodeID& nid) = 0;
};

class Module1D : public ModuleBase {
 public:
  virtual void module_1d_on_change_nearby(const NodeID& prev_nid, const NodeID& next_nid) = 0;

 protected:
  Module1D(ModuleParam& param, Module1DDelegate& module_1d_delegate, Channel::Type channel);

  bool module_1d_check_covered_range(const NodeID& nid);

 private:
  Module1DDelegate& delegate;
};
}  // namespace colonio
