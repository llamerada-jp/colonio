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

#include "map_impl.hpp"

#include "value_impl.hpp"

namespace colonio {

MapImpl ::MapImpl(APIGate& api_gate_, APIChannel::Type channel_) : api_gate(api_gate_), channel(channel_) {
}

Value MapImpl ::get(const Value& key) {
  api::Call call;
  api::map_api::Get* api = call.mutable_map_get();
  ValueImpl::to_pb(api->mutable_key(), key);

  std::unique_ptr<api::Reply> reply = api_gate.call_sync(channel, call);
  if (reply->has_map_get()) {
    return ValueImpl::from_pb(reply->map_get().value());
  } else {
    throw get_exception(*reply);
  }
}

void MapImpl::get(
    const Value& key, std::function<void(Map&, const Value&)> on_success,
    std::function<void(Map&, const Error&)> on_failure) {
  api::Call call;
  api::map_api::Get* api = call.mutable_map_get();
  ValueImpl::to_pb(api->mutable_key(), key);

  api_gate.call_async(channel, call, [on_success, on_failure, this](const api::Reply& reply) {
    if (reply.has_map_get()) {
      on_success(*this, ValueImpl::from_pb(reply.map_get().value()));
    } else {
      on_failure(*this, get_error(reply));
    }
  });
}

void MapImpl::set(const Value& key, const Value& value, uint32_t opt) {
  api::Call call;
  api::map_api::Set* api = call.mutable_map_set();
  ValueImpl::to_pb(api->mutable_key(), key);
  ValueImpl::to_pb(api->mutable_value(), value);
  api->set_opt(opt);

  std::unique_ptr<api::Reply> reply = api_gate.call_sync(channel, call);
  if (!reply->has_success()) {
    throw get_exception(*reply);
  }
}

void MapImpl::set(
    const Value& key, const Value& value, uint32_t opt, std::function<void(Map&)> on_success,
    std::function<void(Map&, const Error&)> on_failure) {
  api::Call call;
  api::map_api::Set* api = call.mutable_map_set();
  ValueImpl::to_pb(api->mutable_key(), key);
  ValueImpl::to_pb(api->mutable_value(), value);
  api->set_opt(opt);

  api_gate.call_async(channel, call, [on_success, on_failure, this](const api::Reply& reply) {
    if (reply.has_success()) {
      on_success(*this);
    } else {
      on_failure(*this, get_error(reply));
    }
  });
}
}  // namespace colonio
