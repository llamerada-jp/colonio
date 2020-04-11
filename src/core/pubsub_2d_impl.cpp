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
#include "pubsub_2d_impl.hpp"

#include "value_impl.hpp"

namespace colonio {
Pubsub2DImpl::Pubsub2DImpl(APIGate& api_gate_, APIChannel::Type channel_) : api_gate(api_gate_), channel(channel_) {
  api_gate.set_event_hook(channel, [this](const api::Event& e) {
    switch (e.param_case()) {
      case api::Event::ParamCase::kPubsub2DOn: {
        const api::pubsub_2d::OnEvent& o = e.pubsub_2d_on();
        auto it                          = subscribers.find(o.name());
        if (it != subscribers.end()) {
          it->second(ValueImpl::from_pb(o.value()));
        }
      } break;

      default:
        assert(false);
        break;
    }
  });
}

void Pubsub2DImpl::publish(const std::string& name, double x, double y, double r, const Value& value, uint32_t opt) {
  api::Call call;
  api::pubsub_2d::Publish* api = call.mutable_pubsub_2d_publish();
  api->set_name(name);
  api->set_x(x);
  api->set_y(y);
  api->set_r(r);
  ValueImpl::to_pb(api->mutable_value(), value);
  api->set_opt(opt);

  std::unique_ptr<api::Reply> reply = api_gate.call_sync(channel, call);
  if (!reply->has_success()) {
    throw get_exception(*reply);
  }
}

void Pubsub2DImpl::publish(
    const std::string& name, double x, double y, double r, const Value& value,
    std::function<void(Pubsub2D&)> on_success, std::function<void(Pubsub2D&, const Error&)> on_failure, uint32_t opt) {
  api::Call call;
  api::pubsub_2d::Publish* api = call.mutable_pubsub_2d_publish();
  api->set_name(name);
  api->set_x(x);
  api->set_y(y);
  api->set_r(r);
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

void Pubsub2DImpl::on(const std::string& name, const std::function<void(const Value&)>& subscriber) {
  subscribers.insert(std::make_pair(name, subscriber));
}

void Pubsub2DImpl::off(const std::string& name) {
  auto it = subscribers.find(name);
  if (it != subscribers.end()) {
    subscribers.erase(it);
  }
}
}  // namespace colonio
