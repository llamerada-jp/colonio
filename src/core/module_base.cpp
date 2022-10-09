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

#include "module_base.hpp"

#include <cassert>

#include "command.hpp"
#include "convert.hpp"
#include "logger.hpp"
#include "packet.hpp"
#include "random.hpp"
#include "scheduler.hpp"
#include "utils.hpp"

namespace colonio {
/**
 * Simple destructor for vtable.
 */
ModuleDelegate::~ModuleDelegate() {
}

/**
 * Constructor with a module that parent module of this instance.
 * @param module Module type.
 */
ModuleBase::ModuleBase(ModuleParam& param, Channel::Type channel_) :
    channel(channel_),
    logger(param.logger),
    random(param.random),
    scheduler(param.scheduler),
    local_nid(param.local_nid),
    delegate(param.delegate) {
  assert(channel_ != Channel::NONE);

  scheduler.add_controller_loop(this, std::bind(&ModuleBase::on_persec, this), 1000);
}

ModuleBase::~ModuleBase() {
  scheduler.remove_task(this);
}

std::unique_ptr<const Packet> ModuleBase::copy_packet_for_reply(const Packet& src) {
  return std::make_unique<const Packet>(src);
}

void ModuleBase::module_on_change_accessor_state(LinkState::Type seed_status, LinkState::Type node_status) {
  // for override.
}

void ModuleBase::reset() {
  assert(scheduler.is_controller_thread());
  containers.clear();
}

bool ModuleBase::cancel_packet(uint32_t id) {
  return containers.erase(id) > 0;
}
}  // namespace colonio
