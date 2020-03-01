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
#include "utils.hpp"

namespace colonio {

APIModuleBundler::APIModuleBundler(APIModuleDelegate& delegate_) : delegate(delegate_) {
}

void APIModuleBundler::clear() {
  modules.clear();
}

void APIModuleBundler::registrate(std::shared_ptr<APIModule> module) {
  assert(module->channel != APIChannel::NONE);
  assert(module->module_channel != APIModuleChannel::NONE);
  assert(modules.find(std::make_pair(module->channel, module->module_channel)) == modules.end());

  modules.insert(std::make_pair(std::make_pair(module->channel, module->module_channel), module));
}

void APIModuleBundler::on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status) {
  for (auto module : modules) {
    module.second->module_on_change_accessor_status(seed_status, node_status);
  }
}

void APIModuleBundler::on_recv_packet(std::unique_ptr<Packet> packet) {
  assert(packet->channel != APIChannel::NONE);
  assert(packet->module_channel != APIModuleChannel::NONE);

  auto module = modules.find(std::make_pair(packet->channel, packet->module_channel));
  if (module != modules.end()) {
    module->second->on_recv_packet(std::move(packet));
  } else {
    colonio_throw("Received incorrect packet entry", Utils::dump_packet(*packet));
  }
}

}  // namespace colonio
