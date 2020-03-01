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

#include "definition.hpp"

namespace colonio {
class APIModule;
class APIModuleDelegate;
class Packet;

class APIModuleBundler {
 public:
  APIModuleBundler(APIModuleDelegate& delegate_);

  void clear();
  void registrate(std::shared_ptr<APIModule> module);
  void on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status);
  void on_recv_packet(std::unique_ptr<Packet> packet);

 private:
  APIModuleDelegate& delegate;

  std::map<std::pair<APIChannel::Type, APIModuleChannel::Type>, std::shared_ptr<APIModule>> modules;
};
}  // namespace colonio
