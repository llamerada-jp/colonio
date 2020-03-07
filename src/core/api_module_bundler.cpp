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

#include "api_module_bundler.hpp"

#include <cassert>

#include "api_module.hpp"
#include "packet.hpp"
#include "system_1d.hpp"
#include "system_2d.hpp"
#include "utils.hpp"

namespace colonio {

APIModuleBundler::APIModuleBundler(
    APIModuleDelegate& module_delegate_, System1DDelegate& system1d_delegate_, System2DDelegate& system2d_delegate_) :
    module_delegate(module_delegate_),
    system1d_delegate(system1d_delegate_),
    system2d_delegate(system2d_delegate_) {
}

void APIModuleBundler::clear() {
  modules.clear();
  modules_1d.clear();
  modules_2d.clear();
}

void APIModuleBundler::registrate(APIModule* module, bool is_1d, bool is_2d) {
  assert(module->channel != APIChannel::NONE);
  assert(module->module_channel != APIModuleChannel::NONE);
  assert(modules.find(std::make_pair(module->channel, module->module_channel)) == modules.end());

  modules.insert(std::make_pair(std::make_pair(module->channel, module->module_channel), module));

  if (is_1d) {
    modules_1d.insert(dynamic_cast<System1D*>(module));
  }

  if (is_2d) {
    modules_2d.insert(dynamic_cast<System2D*>(module));
  }
}

void APIModuleBundler::on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status) {
  for (auto module : modules) {
    module.second->module_on_change_accessor_status(seed_status, node_status);
  }
}

void APIModuleBundler::on_recv_packet(std::unique_ptr<const Packet> packet) {
  assert(packet->channel != APIChannel::NONE);
  assert(packet->module_channel != APIModuleChannel::NONE);

  auto module = modules.find(std::make_pair(packet->channel, packet->module_channel));
  if (module != modules.end()) {
    module->second->on_recv_packet(std::move(packet));
  } else {
    colonio_throw("Received incorrect packet entry", Utils::dump_packet(*packet).c_str());
  }
}

void APIModuleBundler::system_1d_on_change_nearby(const NodeID& prev_nid, const NodeID& next_nid) {
  for (auto& it : modules_1d) {
    it->system_1d_on_change_nearby(prev_nid, next_nid);
  }
}

void APIModuleBundler::system_2d_on_change_my_position(const Coordinate& position) {
  for (auto& it : modules_2d) {
    it->system_2d_on_change_my_position(position);
  }
}

void APIModuleBundler::system_2d_on_change_nearby(const std::set<NodeID>& nids) {
  for (auto& it : modules_2d) {
    it->system_2d_on_change_nearby(nids);
  }
}

void APIModuleBundler::system_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) {
  for (auto& it : modules_2d) {
    it->system_2d_on_change_nearby_position(positions);
  }
}

}  // namespace colonio
