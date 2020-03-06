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

#include <map>
#include <memory>

#include "coordinate.hpp"
#include "definition.hpp"
#include "node_id.hpp"

namespace colonio {
class APIModule;
class APIModuleDelegate;
class Packet;
class System1D;
class System1DDelegate;
class System2D;
class System2DDelegate;

class APIModuleBundler {
 public:
  APIModuleDelegate& module_delegate;
  System1DDelegate& system1d_delegate;
  System2DDelegate& system2d_delegate;

  APIModuleBundler(
      APIModuleDelegate& module_delegate_, System1DDelegate& system1d_delegate_, System2DDelegate& system2d_delegate_);

  void clear();
  void registrate(APIModule* module, bool is_1d, bool is_2d);

  void on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status);
  void on_recv_packet(std::unique_ptr<const Packet> packet);

  void system_1d_on_change_nearby(const NodeID& prev_nid, const NodeID& next_nid);

  void system_2d_on_change_my_position(const Coordinate& position);
  void system_2d_on_change_nearby(const std::set<NodeID>& nids);
  void system_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions);

 private:
  std::map<std::pair<APIChannel::Type, APIModuleChannel::Type>, APIModule*> modules;
  std::set<System1D*> modules_1d;
  std::set<System2D*> modules_2d;
};
}  // namespace colonio
