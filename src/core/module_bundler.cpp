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

#include "module_bundler.hpp"

#include <cassert>

#include "module_1d.hpp"
#include "module_2d.hpp"
#include "module_base.hpp"
#include "packet.hpp"
#include "utils.hpp"

namespace colonio {

ModuleBundler::ModuleBundler(
    ModuleDelegate& module_delegate_, Module1DDelegate& module_1d_delegate_, Module2DDelegate& module_2d_delegate_) :
    module_delegate(module_delegate_),
    module_1d_delegate(module_1d_delegate_),
    module_2d_delegate(module_2d_delegate_) {
}

void ModuleBundler::clear() {
  modules.clear();
  modules_1d.clear();
  modules_2d.clear();
}

void ModuleBundler::registrate(ModuleBase* module, bool is_1d, bool is_2d) {
  assert(module->channel != APIChannel::NONE);
  assert(module->module_channel != ModuleChannel::NONE);
  assert(modules.find(std::make_pair(module->channel, module->module_channel)) == modules.end());

  modules.insert(std::make_pair(std::make_pair(module->channel, module->module_channel), module));

  if (is_1d) {
    modules_1d.insert(dynamic_cast<Module1D*>(module));
  }

  if (is_2d) {
    modules_2d.insert(dynamic_cast<Module2D*>(module));
  }
}

void ModuleBundler::on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status) {
  for (auto module : modules) {
    module.second->module_on_change_accessor_status(seed_status, node_status);
  }
}

void ModuleBundler::on_recv_packet(std::unique_ptr<const Packet> packet) {
  assert(packet->channel != APIChannel::NONE);
  assert(packet->module_channel != ModuleChannel::NONE);

  auto module = modules.find(std::make_pair(packet->channel, packet->module_channel));
  if (module != modules.end()) {
    module->second->on_recv_packet(std::move(packet));
  } else {
    colonio_throw(
        Exception::Code::INCORRECT_DATA_FORMAT, "Received incorrect packet entry", Utils::dump_packet(*packet).c_str());
  }
}

void ModuleBundler::module_1d_on_change_nearby(const NodeID& prev_nid, const NodeID& next_nid) {
  for (auto& it : modules_1d) {
    it->module_1d_on_change_nearby(prev_nid, next_nid);
  }
}

void ModuleBundler::module_2d_on_change_local_position(const Coordinate& position) {
  for (auto& it : modules_2d) {
    it->module_2d_on_change_local_position(position);
  }
}

void ModuleBundler::module_2d_on_change_nearby(const std::set<NodeID>& nids) {
  for (auto& it : modules_2d) {
    it->module_2d_on_change_nearby(nids);
  }
}

void ModuleBundler::module_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) {
  for (auto& it : modules_2d) {
    it->module_2d_on_change_nearby_position(positions);
  }
}

}  // namespace colonio
