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
#include "colonio/colonio.hpp"

#include <cassert>

#include "api.pb.h"
#include "api_gate.hpp"
#include "colonio_impl.hpp"

namespace colonio {
class Colonio::Impl {
 public:
  APIGate api_gate;
};

std::string get_error_message(const api::Reply& reply) {
  if (reply.has_failure()) {
    return reply.failure().message();
  } else {
    return "unknown error";
  }
}

Colonio::Colonio() {
  impl = std::make_unique<Colonio::Impl>();
  impl->api_gate.set_event_hook(APIChannel::COLONIO, [this](const api::Event& e) {
    switch (e.param_case()) {
      case api::Event::ParamCase::kColonioLog: {
        const api::colonio::LogEvent& l = e.colonio_log();
        on_output_log(static_cast<LogLevel::Type>(l.level()), l.message());
      } break;

      case api::Event::ParamCase::kColonioDebug: {
        const api::colonio::DebugEvent& d = e.colonio_debug();
        on_debug_event(static_cast<DebugEvent::Type>(d.event()), d.json());
      } break;

      default:
        assert(false);
        break;
    }
  });
}

Colonio::~Colonio() {
}

Map& Colonio::access_map(const std::string& name) {
  // TODO return impl->access<Map>(name);
  assert(false);
}

PubSub2D& Colonio::access_pubsub2d(const std::string& name) {
  // TODO return impl->access<PubSub2D>(name);
  assert(false);
}

void Colonio::connect(const std::string& url, const std::string& token) {
  assert(impl);

  impl->api_gate.init();

  api::Call call;
  api::colonio::Connect* api = call.mutable_colonio_connect();
  api->set_url(url);
  api->set_token(token);

  std::unique_ptr<api::Reply> reply = impl->api_gate.call_sync(APIChannel::COLONIO, call);
  if (reply) {
    if (reply->has_colonio_connect()) {
      // TODO
      // assert(false);
    } else {
      throw ColonioException(get_error_message(*reply));
    }
  }
}

void Colonio::disconnect() {
  assert(impl);

  api::Call call;
  call.mutable_colonio_disconnect();

  std::unique_ptr<api::Reply> reply = impl->api_gate.call_sync(APIChannel::COLONIO, call);
  if (reply) {
    if (!reply->has_success()) {
      throw ColonioException(get_error_message(*reply));
    }
  }

  impl->api_gate.quit();
}

std::string Colonio::get_local_nid() {
  assert(impl);

  api::Call call;
  call.mutable_colonio_get_local_nid();

  std::unique_ptr<api::Reply> reply = impl->api_gate.call_sync(APIChannel::COLONIO, call);
  if (reply) {
    if (reply->has_colonio_get_local_nid()) {
      return NodeID::from_pb(reply->colonio_get_local_nid().local_nid()).to_str();
    } else {
      throw ColonioException(get_error_message(*reply));
    }
  } else {
    return NodeID::NONE.to_str();
  }
}

std::tuple<double, double> Colonio::set_position(double x, double y) {
  assert(impl);

  api::Call call;
  api::colonio::SetPosition* api = call.mutable_colonio_set_position();
  Coordinate(x, y).to_pb(api->mutable_position());

  std::unique_ptr<api::Reply> reply = impl->api_gate.call_sync(APIChannel::COLONIO, call);
  if (reply->has_colonio_set_position()) {
    Coordinate position = Coordinate::from_pb(reply->colonio_set_position().position());
    return std::make_tuple(position.x, position.y);
  } else {
    throw ColonioException(get_error_message(*reply));
  }
}

void Colonio::on_output_log(LogLevel::Type level, const std::string& message) {
  // Drop log message.
}

void Colonio::on_debug_event(DebugEvent::Type event, const std::string& json) {
  // Drop debug event.
}
}  // namespace colonio
