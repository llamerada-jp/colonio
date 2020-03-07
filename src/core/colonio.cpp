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
#include "map_impl.hpp"

namespace colonio {
class Colonio::Impl {
 public:
  APIGate api_gate;
  std::map<std::string, std::unique_ptr<Map>> maps;
};

Colonio::Colonio() {
}

Colonio::~Colonio() {
  if (impl) {
    disconnect();
  }
}

Map& Colonio::access_map(const std::string& name) {
  auto it = impl->maps.find(name);
  if (it != impl->maps.end()) {
    return *it->second;

  } else {
    throw ColonioException(
        ColonioException::Code::CONFLICT_WITH_SETTING, Utils::format_string("map not found : ", 0, name.c_str()));
  }
}

PubSub2D& Colonio::access_pubsub2d(const std::string& name) {
  // TODO return impl->access<PubSub2D>(name);
  assert(false);
}

void Colonio::connect(const std::string& url, const std::string& token) {
  assert(!impl);

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

  impl->api_gate.init();

  api::Call call;
  api::colonio::Connect* api = call.mutable_colonio_connect();
  api->set_url(url);
  api->set_token(token);

  std::unique_ptr<api::Reply> reply = impl->api_gate.call_sync(APIChannel::COLONIO, call);
  if (reply) {
    if (reply->has_colonio_connect()) {
      const api::colonio::ConnectReply& param = reply->colonio_connect();
      for (auto& module_param : param.modules()) {
        APIChannel::Type channel = static_cast<APIChannel::Type>(module_param.channel());
        switch (module_param.type()) {
          case api::colonio::ConnectReply_ModuleType_MAP:
            impl->maps.insert(
                std::make_pair(module_param.name(), std::unique_ptr<Map>(new MapImpl(impl->api_gate, channel))));
            break;

          default:
            assert(false);
            break;
        }
      }
    } else {
      throw get_exception(*reply);
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
      throw get_exception(*reply);
    }
  }

  impl->api_gate.quit();
  impl.reset();
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
      throw get_exception(*reply);
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
    throw get_exception(*reply);
  }
}

void Colonio::on_output_log(LogLevel::Type level, const std::string& message) {
  // Drop log message.
}

void Colonio::on_debug_event(DebugEvent::Type event, const std::string& json) {
  // Drop debug event.
}
}  // namespace colonio
