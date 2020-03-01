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

#include "api_module.hpp"

namespace colonio {
class Context;
class NodeID;
class System1D;

class System1DDelegate {
 public:
  virtual ~System1DDelegate();
  virtual bool system_1d_do_check_covered_range(System1D& system1d, const NodeID& nid) = 0;
};

class System1D : public APIModule {
 public:
  virtual void system_1d_on_change_nearby(const NodeID& prev_nid, const NodeID& next_nid) = 0;

 protected:
  System1D(
      Context& context, APIModuleDelegate& module_delegate, System1DDelegate& system_delegate, APIChannel::Type channel,
      APIModuleChannel::Type module_channel);

  bool system_1d_check_covered_range(const NodeID& nid);

 private:
  System1DDelegate& delegate;
};
}  // namespace colonio
