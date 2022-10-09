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
#include "colonio_impl.hpp"

#include <cassert>

#include "convert.hpp"
#include "coord_system_plane.hpp"
#include "coord_system_sphere.hpp"
#include "logger.hpp"
#include "packet.hpp"
#include "pipe.hpp"
#include "routing_1d.hpp"
#include "scheduler.hpp"
#include "utils.hpp"

namespace colonio {
ColonioImpl::ColonioImpl(const ColonioConfig& config) :
    logger([&, this](const std::string& json) {
      config.logger_func(*this, json);
    }),
    local_config(config) {
}

ColonioImpl::~ColonioImpl() {
  release_resources();
  user_thread_pool.reset();
}

void ColonioImpl::connect(const std::string& url, const std::string& token) {
  Pipe<int> pipe;

  allocate_resources();
  scheduler->add_task(this, [this, &pipe, url, token]() {
    network->connect(
        url, token,
        [&pipe]() {
          pipe.push(1);
        },
        [&pipe](const Error& error) {
          pipe.push_error(error);
        });
  });

  pipe.pop_with_throw();
}

void ColonioImpl::connect(
    const std::string& url, const std::string& token, std::function<void(Colonio&)>&& on_success,
    std::function<void(Colonio&, const Error&)>&& on_failure) {
  allocate_resources();
  scheduler->add_task(this, [this, url, token, on_success, on_failure]() {
    if (user_thread_pool) {
      network->connect(
          url, token,
          [this, on_success]() {
            user_thread_pool->push([this, on_success]() {
              on_success(*this);
            });
          },
          [this, on_failure](const Error& e) {
            user_thread_pool->push([this, on_failure, e]() {
              on_failure(*this, e);
            });
          });
    } else {
      network->connect(
          url, token,
          [this, on_success]() {
            on_success(*this);
          },
          [this, on_failure](const Error& e) {
            on_failure(*this, e);
          });
    }
  });
}

void ColonioImpl::disconnect() {
  assert(scheduler);
  Pipe<int> pipe;

  scheduler->add_task(this, [&, this]() {
    network->disconnect(
        [&pipe]() {
          pipe.push(1);
        },
        [&pipe](const Error& error) {
          pipe.push_error(error);
        });
  });

  pipe.pop_with_throw();
  release_resources();
}

void ColonioImpl::disconnect(
    std::function<void(Colonio&)>&& on_success, std::function<void(Colonio&, const Error&)>&& on_failure) {
  assert(scheduler);

  scheduler->add_task(this, [this, on_success, on_failure]() {
    if (user_thread_pool) {
      network->disconnect(
          [this, on_success]() {
            user_thread_pool->push([this, on_success]() {
              release_resources();
              on_success(*this);
            });
          },
          [this, on_failure](const Error& e) {
            user_thread_pool->push([this, on_failure, e]() {
              on_failure(*this, e);
              release_resources();
            });
          });

    } else {
      network->disconnect(
          [this, on_success]() {
            on_success(*this);
          },
          [this, on_failure](const Error& e) {
            on_failure(*this, e);
          });
    }
  });
}

bool ColonioImpl::is_connected() {
  if (!network) {
    return false;
  }
  return network->is_connected();
}

std::string ColonioImpl::get_local_nid() {
  if (!is_connected()) {
    return "";
  }

  return local_nid.to_str();
}

std::tuple<double, double> ColonioImpl::set_position(double x, double y) {
  if (!coord_system) {
    colonio_throw_error(ErrorCode::CONFLICT_WITH_SETTING, "coordinate system was not enabled");
  }

  Coordinate new_position(x, y);
  Coordinate old_position = coord_system->get_local_position();
  if (new_position != old_position) {
    coord_system->set_local_position(new_position);
    assert(false);
    /* TODO
    for (auto& it : modules_2d) {
      it->module_2d_on_change_local_position(new_position);
    }
    //*/
    network->change_local_position(new_position);
    log_debug("current position").map("coordinate", Convert::coordinate2json(new_position));
  }

  Coordinate adjusted = coord_system->get_local_position();

  return std::make_tuple(adjusted.x, adjusted.y);
}

Value ColonioImpl::messaging_post(
    const std::string& dst_nid, const std::string& name, const Value& message, uint32_t opt) {
  assert(false);
  // @TODO
  return Value();
}

void ColonioImpl::messaging_post(
    const std::string& dst_nid, const std::string& name, const Value& message, uint32_t opt,
    std::function<void(Colonio&, const Value&)>&& on_success,
    std::function<void(Colonio&, const Error&)>&& on_failure) {
  assert(false);
  // @TODO
}

void ColonioImpl::messaging_set_handler(
    const std::string& name, std::function<Value(Colonio&, const MessageData&)>&& func) {
  assert(false);
  // @TODO
}

void ColonioImpl::messaging_set_handler(
    const std::string& name, std::function<void(Colonio&, const MessageData&, MessageResponseWriter&)>&& func) {
  assert(false);
  // @TODO
}

void ColonioImpl::messaging_unset_handler(const std::string& name) {
  assert(false);
  // @TODO
}

void ColonioImpl::command_manager_do_send_packet(std::unique_ptr<const Packet> packet) {
  network->switch_packet(std::move(packet), false);
}

void ColonioImpl::command_manager_do_relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet) {
  assert(!dst_nid.is_special() || dst_nid == NodeID::NEXT);
  network->relay_packet(dst_nid, std::move(packet));
}

void ColonioImpl::network_on_change_global_config(const picojson::object& config) {
  if (!global_config.empty()) {
    // check revision
    if (global_config.at("revision").get<double>() != config.at("revision").get<double>()) {
      // @TODO Warn new revision to the application.
    }
    return;
  }
  global_config = config;

  // @TODO: initialize modules
  // assert(false);
}

void ColonioImpl::network_on_change_nearby_1d(const NodeID& prev_nid, const NodeID& next_nid) {
  // @TODO
  // assert(false);
}

void ColonioImpl::network_on_change_nearby_2d(const std::set<NodeID>& nids) {
  assert(false);
  // @TODO
}

void ColonioImpl::network_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) {
  assert(false);
  // @TODO
}

const CoordSystem* ColonioImpl::network_on_require_coord_system(const picojson::object& config) {
  assert(!coord_system);

  // coord_system
  picojson::object cs2d_config;
  if (Utils::check_json_optional(config, "coordSystem2D", &cs2d_config)) {
    const std::string& type = Utils::get_json<std::string>(cs2d_config, "type");
    if (type == "sphere") {
      coord_system = std::make_unique<CoordSystemSphere>(random, cs2d_config);
    } else if (type == "plane") {
      coord_system = std::make_unique<CoordSystemPlane>(random, cs2d_config);
    }
  }

  return coord_system.get();
}

void ColonioImpl::allocate_resources() {
  assert(local_nid == NodeID::NONE);

  local_nid = NodeID::make_random(random);
  if (!local_config.disable_callback_thread) {
    user_thread_pool = std::make_unique<UserThreadPool>(logger, local_config.max_user_threads, 3 * 60 * 1000);
  }
  scheduler.reset(Scheduler::new_instance(logger));
  coord_system.reset();
  command_manager = std::make_unique<CommandManager>(logger, random, *scheduler, local_nid, *this);
  network         = std::make_unique<Network>(logger, random, *scheduler, *command_manager, local_nid, *this);
}

void ColonioImpl::release_resources() {
  network.reset();
  command_manager.reset();
  coord_system.reset();
  scheduler.reset();
  // user_thread_pool.reset();
  local_nid = NodeID::NONE;
}

/*
Value ColonioImpl::call_by_nid(const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt) {
  Pipe<Value> pipe;

  call_by_nid(
      dst_nid, name, value, opt,
      [&pipe](Colonio&, const Value& reply) {
        pipe.push(reply);
      },
      [&pipe](Colonio&, const Error& error) {
        pipe.push_error(error);
      });

  return *pipe.pop_with_throw();
}

void ColonioImpl::call_by_nid(
    const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt,
    std::function<void(Colonio&, const Value&)>&& on_success,
    std::function<void(Colonio&, const Error&)>&& on_failure) {
  try {
    colonio_module->call_by_nid(
        dst_nid, name, value, opt,
        [this, on_success](const Value& reply) {
          on_success(*this, reply);
        },
        [this, on_failure](const Error& err) {
          on_failure(*this, err);
        });
  } catch (Error& e) {
    on_failure(*this, e);
  }
}

void ColonioImpl::on_call(const std::string& name, std::function<Value(Colonio&, const CallParameter&)>&& func) {
  colonio_module->on_call(name, [this, func](const CallParameter& parameter) {
    return func(*this, parameter);
  });
}

void ColonioImpl::off_call(const std::string& name) {
  colonio_module->off_call(name);
}

const NodeID& Network::module_2d_do_get_relay_nid(Module2D& module_2d, const Coordinate& position) {
  return routing->get_relay_nid_2d(position);
}

bool ColonioImpl::module_1d_do_check_covered_range(Module1D& module_1d, const NodeID& nid) {
  assert(routing);
  return routing->is_covered_range_1d(nid);
}

void ColonioImpl::check_api_connect() {
  if (!connect_cb || link_state != LinkState::ONLINE) {
    return;
  }
  assert(seed_accessor->get_auth_status() == AuthStatus::SUCCESS);

  scheduler->add_user_task(this, [this]() {
    if (connect_cb) {
      auto d = Utils::defer([this] {
        connect_cb.reset();
      });
      connect_cb->first(*this);
    }
  });
}

void ColonioImpl::clear_modules() {
  if_map.clear();
  if_pubsub2d.clear();

  modules.clear();
  modules_1d.clear();
  modules_2d.clear();

  node_accessor = nullptr;
  routing       = nullptr;

  seed_accessor.reset();
}

void ColonioImpl::register_module(ModuleBase* module, const std::string* name, bool is_1d, bool is_2d) {
  assert(module->channel != Channel::NONE);
  assert(modules.find(module->channel) == modules.end());
  assert(name == nullptr || module_names.find(*name) == module_names.end());

  modules.insert(std::make_pair(module->channel, module));

  if (name != nullptr) {
    module_names.insert(std::make_pair(*name, module->channel));
  }
  if (is_1d) {
    modules_1d.insert(dynamic_cast<Module1D*>(module));
  }
  if (is_2d) {
    modules_2d.insert(dynamic_cast<Module2D*>(module));
  }
}

void ColonioImpl::initialize_algorithms() {
  const picojson::object& modules_config = Utils::get_json<picojson::object>(config, "modules", picojson::object());

  for (auto& it : modules_config) {
    const std::string& name               = it.first;
    const picojson::object& module_config = Utils::get_json<picojson::object>(modules_config, name);
    const std::string& type               = Utils::get_json<std::string>(module_config, "type");

    if (type == "pubsub2D") {
      Pubsub2DModule* mod = Pubsub2DModule::new_instance(module_param, *this, *coord_system, module_config);
      register_module(mod, &name, false, true);
      if_pubsub2d.insert(std::make_pair(name, std::make_unique<Pubsub2DImpl>(*mod)));

    } else if (type == "mapPaxos") {
      MapPaxosModule* mod = MapPaxosModule::new_instance(module_param, *this, module_config);
      register_module(mod, &name, true, false);
      if_map.insert(std::make_pair(name, std::make_unique<MapImpl>(*mod)));

    } else {
      colonio_throw_fatal("unsupported algorithm(%s)", type.c_str());
    }
  }

  check_api_connect();
}
//*/
}  // namespace colonio
