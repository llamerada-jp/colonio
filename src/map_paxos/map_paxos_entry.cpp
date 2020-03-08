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
#include "map_paxos_entry.hpp"

#include "core/api_entry_bundler.hpp"
#include "core/api_module_bundler.hpp"
#include "core/utils.hpp"
#include "core/value_impl.hpp"
#include "map_paxos.hpp"

namespace colonio {
void MapPaxosEntry::make_entry(
    Context& context, APIEntryBundler& api_bundler, APIEntryDelegate& entry_delegate, APIModuleBundler& module_bundler,
    const picojson::object& config) {
  APIChannel::Type channel    = static_cast<APIChannel::Type>(Utils::get_json<double>(config, "channel"));
  unsigned int retry_max      = Utils::get_json<double>(config, "retryMax", MAP_PAXOS_RETRY_MAX);
  uint32_t retry_interval_min = Utils::get_json<double>(config, "retryIntervalMin", MAP_PAXOS_RETRY_INTERVAL_MIN);
  uint32_t retry_interval_max = Utils::get_json<double>(config, "retryIntervalMax", MAP_PAXOS_RETRY_INTERVAL_MAX);
  assert(retry_interval_min <= retry_interval_max);

  // create a module instance.
  std::unique_ptr<MapPaxos> module = std::make_unique<MapPaxos>(
      context, module_bundler.module_delegate, module_bundler.system1d_delegate, channel,
      APIModuleChannel::MapPaxos::MAP_PAXOS, retry_max, retry_interval_min, retry_interval_max);
  module_bundler.registrate(module.get(), true, false);

  // create a entry instance.
  std::shared_ptr<MapPaxosEntry> entry(new MapPaxosEntry(context, entry_delegate, channel, std::move(module)));
  api_bundler.registrate(entry);
}

MapPaxosEntry::MapPaxosEntry(
    Context& context, APIEntryDelegate& delegate, APIChannel::Type channel, std::unique_ptr<MapPaxos> module) :
    APIEntry(context, delegate, channel),
    mp(std::move(module)) {
}

void MapPaxosEntry::api_entry_on_recv_call(const api::Call& call) {
  switch (call.param_case()) {
    case api::Call::ParamCase::kMapGet:
      api_get(call.id(), ValueImpl::from_pb(call.map_get().key()));
      break;

    case api::Call::ParamCase::kMapSet:
      api_set(
          call.id(), ValueImpl::from_pb(call.map_set().key()), ValueImpl::from_pb(call.map_set().value()),
          call.map_set().opt());
      break;

    default:
      colonio_fatal("Called incorrect colonio API entry : %d", call.param_case());
      break;
  }
}

void MapPaxosEntry::api_get(uint32_t id, const Value& key) {
  mp->get(
      key,
      [this, id](const Value& value) {
        std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
        reply->set_id(id);
        api::map_api::GetReply* param = reply->mutable_map_get();
        ValueImpl::to_pb(param->mutable_value(), value);
        api_reply(std::move(reply));
      },
      [this, id](ColonioException::Code code) {
        // TODO error message
        api_failure(id, code, "");
      });
}

void MapPaxosEntry::api_set(uint32_t id, const Value& key, const Value& value, MapOption::Type opt) {
  mp->set(
      key, value, [this, id]() { api_success(id); },
      [this, id](ColonioException::Code code) {
        // TODO error message
        api_failure(id, code, "");
      },
      opt);
}

}  // namespace colonio
