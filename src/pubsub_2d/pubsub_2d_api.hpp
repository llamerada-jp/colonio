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

#include <picojson.h>

#include "colonio/pubsub_2d.hpp"
#include "core/api_base.hpp"
#include "pubsub_2d_module.hpp"

namespace colonio {
class APIBundler;
class APIDelegate;
class ModuleBundler;
class Context;
class Pubsub2DModule;
class Value;

class Pubsub2DAPI : public APIBase, public Pubsub2DModuleDelegate {
 public:
  static void make_entry(
      Context& context, APIBundler& api_bundler, APIDelegate& api_delegate, ModuleBundler& module_bundler,
      const CoordSystem& coord_system, const picojson::object& config);

 private:
  std::unique_ptr<Pubsub2DModule> module;

  Pubsub2DAPI(Context& context, APIDelegate& delegate, APIChannel::Type channel);

  void pubsub_2d_module_on_on(Pubsub2DModule& ps2_module, const std::string& name, const Value& value) override;

  void api_on_recv_call(const api::Call& call) override;
  void api_publish(
      uint32_t id, const std::string& name, double x, double y, double r, const Value& value, uint32_t opt);
};

}  // namespace colonio
