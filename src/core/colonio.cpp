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
#include "pubsub_2d_impl.hpp"

namespace colonio {
class Colonio::Impl {
 public:
  bool enabled;
  APIGate api_gate;
  NodeID local_nid;
  std::map<std::string, std::unique_ptr<Map>> maps;
  std::map<std::string, std::unique_ptr<Pubsub2D>> pubsub_2ds;

  Impl() : enabled(false) {
  }
};

Colonio::Colonio() : impl(std::make_unique<Colonio::Impl>()) {
}

Colonio::~Colonio() {
  if (impl->enabled) {
#ifndef EMSCRIPTEN
    disconnect();
#else
    assert(false);
#endif
  }
}

Map& Colonio::access_map(const std::string& name) {
  auto it = impl->maps.find(name);
  if (it != impl->maps.end()) {
    return *it->second;

  } else {
    throw Exception(ErrorCode::CONFLICT_WITH_SETTING, Utils::format_string("map not found : ", 0, name.c_str()));
  }
}

Pubsub2D& Colonio::access_pubsub_2d(const std::string& name) {
  auto it = impl->pubsub_2ds.find(name);
  if (it != impl->pubsub_2ds.end()) {
    return *it->second;

  } else {
    throw Exception(ErrorCode::CONFLICT_WITH_SETTING, Utils::format_string("pubsub_2d not found : ", 0, name.c_str()));
  }
}

void Colonio::connect(const std::string& url, const std::string& token) {
  assert(!impl->enabled);

  impl->enabled = true;
  impl->api_gate.set_event_hook(APIChannel::COLONIO, [this](const api::Event& e) {
    switch (e.param_case()) {
      case api::Event::ParamCase::kColonioLog: {
        const api::colonio::LogEvent& l = e.colonio_log();
        on_output_log(l.json());
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
      impl->local_nid                         = NodeID::from_pb(param.local_nid());

      for (auto& module_param : param.modules()) {
        APIChannel::Type channel = static_cast<APIChannel::Type>(module_param.channel());
        switch (module_param.type()) {
          case api::colonio::ConnectReply_ModuleType_MAP:
            impl->maps.insert(
                std::make_pair(module_param.name(), std::unique_ptr<Map>(new MapImpl(impl->api_gate, channel))));
            break;

          case api::colonio::ConnectReply_ModuleType_PUBSUB_2D:
            impl->pubsub_2ds.insert(std::make_pair(
                module_param.name(), std::unique_ptr<Pubsub2D>(new Pubsub2DImpl(impl->api_gate, channel))));
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

void Colonio::connect(
    const std::string& url, const std::string& token, std::function<void(Colonio&)> on_success,
    std::function<void(Colonio&, const Error&)> on_failure) {
  assert(!impl->enabled);

  impl = std::make_unique<Colonio::Impl>();
  impl->api_gate.set_event_hook(APIChannel::COLONIO, [this](const api::Event& e) {
    switch (e.param_case()) {
      case api::Event::ParamCase::kColonioLog: {
        const api::colonio::LogEvent& l = e.colonio_log();
        on_output_log(l.json());
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

  impl->api_gate.call_async(APIChannel::COLONIO, call, [on_success, on_failure, this](const api::Reply& reply) {
    if (reply.has_colonio_connect()) {
      const api::colonio::ConnectReply& param = reply.colonio_connect();
      impl->local_nid                         = NodeID::from_pb(param.local_nid());

      for (auto& module_param : param.modules()) {
        APIChannel::Type channel = static_cast<APIChannel::Type>(module_param.channel());
        switch (module_param.type()) {
          case api::colonio::ConnectReply_ModuleType_MAP:
            impl->maps.insert(
                std::make_pair(module_param.name(), std::unique_ptr<Map>(new MapImpl(impl->api_gate, channel))));
            break;

          case api::colonio::ConnectReply_ModuleType_PUBSUB_2D:
            impl->pubsub_2ds.insert(std::make_pair(
                module_param.name(), std::unique_ptr<Pubsub2D>(new Pubsub2DImpl(impl->api_gate, channel))));
            break;

          default:
            assert(false);
            break;
        }
      }
      on_success(*this);

    } else {
      on_failure(*this, get_error(reply));
    }
  });
}

#ifndef EMSCRIPTEN
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
  impl->enabled = false;
}
#else

void Colonio::disconnect(
    std::function<void(Colonio&)> on_success, std::function<void(Colonio&, const Error&)> on_failure) {
  api::Call call;
  call.mutable_colonio_disconnect();

  std::unique_ptr<api::Reply> reply = impl->api_gate.call_sync(APIChannel::COLONIO, call);
  impl->api_gate.call_async(APIChannel::COLONIO, call, [on_success, on_failure, this](const api::Reply& reply) {
    if (reply.has_success()) {
      impl->api_gate.quit();
      impl->enabled = false;
      on_success(*this);
    } else {
      on_failure(*this, get_error(reply));
    }
  });
}
#endif

std::string Colonio::get_local_nid() {
  // is_special is true when local_nid is not set.
  if (impl == nullptr || impl->local_nid.is_special()) {
    return "";
  }

  return impl->local_nid.to_str();
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

void Colonio::set_position(
    double x, double y, std::function<void(Colonio&, double, double)> on_success,
    std::function<void(Colonio&, const Error&)> on_failure) {
  assert(impl);

  api::Call call;
  api::colonio::SetPosition* api = call.mutable_colonio_set_position();
  Coordinate(x, y).to_pb(api->mutable_position());

  impl->api_gate.call_async(APIChannel::COLONIO, call, [on_success, on_failure, this](const api::Reply& reply) {
    if (reply.has_colonio_set_position()) {
      Coordinate position = Coordinate::from_pb(reply.colonio_set_position().position());
      on_success(*this, position.x, position.y);
    } else {
      on_failure(*this, get_error(reply));
    }
  });
}

void Colonio::on_output_log(const std::string& json) {
  // Drop log message.
}
}  // namespace colonio
