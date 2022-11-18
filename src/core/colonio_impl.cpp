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
  assert(scheduler && messaging);
  Pipe<Value> pipe;

  scheduler->add_task(this, [&pipe, this, dst_nid, name, message, opt]() {
    messaging->post(
        dst_nid, name, message, opt,
        [&pipe](const Value& response) {
          pipe.push(response);
        },
        [&pipe](const Error& error) {
          pipe.push_error(error);
        });
  });

  return *pipe.pop_with_throw();
}

void ColonioImpl::messaging_post(
    const std::string& dst_nid, const std::string& name, const Value& message, uint32_t opt,
    std::function<void(Colonio&, const Value&)>&& on_response,
    std::function<void(Colonio&, const Error&)>&& on_failure) {
  assert(scheduler && messaging);

  scheduler->add_task(this, [this, dst_nid, name, message, opt, on_response, on_failure]() {
    messaging->post(
        dst_nid, name, message, opt,
        [this, on_response](const Value& response) {
          if (user_thread_pool) {
            user_thread_pool->push([this, on_response, response]() {
              on_response(*this, response);
            });
          } else {
            on_response(*this, response);
          }
        },
        [this, on_failure](const Error& error) {
          if (user_thread_pool) {
            user_thread_pool->push([this, on_failure, error]() {
              on_failure(*this, error);
            });
          } else {
            on_failure(*this, error);
          }
        });
  });
}

void ColonioImpl::messaging_set_handler(
    const std::string& name, std::function<Value(Colonio&, const MessagingRequest&)>&& handler) {
  assert(messaging);
  if (user_thread_pool) {
    messaging->set_handler(
        name, [this, handler](
                  std::shared_ptr<const Colonio::MessagingRequest> request,
                  std::shared_ptr<Colonio::MessagingResponseWriter> response_writer) {
          user_thread_pool->push([this, handler, request, response_writer]() {
            Value response = handler(*this, *request);
            response_writer->write(response);
          });
        });
  } else {
    messaging->set_handler(
        name, [this, handler](
                  std::shared_ptr<const Colonio::MessagingRequest> request,
                  std::shared_ptr<Colonio::MessagingResponseWriter> response_writer) {
          Value response = handler(*this, *request);
          response_writer->write(response);
        });
  }
}

void ColonioImpl::messaging_set_handler(
    const std::string& name,
    std::function<void(Colonio&, const MessagingRequest&, std::shared_ptr<MessagingResponseWriter>)>&& handler) {
  assert(messaging);
  if (user_thread_pool) {
    messaging->set_handler(
        name, [this, handler](
                  std::shared_ptr<const Colonio::MessagingRequest> request,
                  std::shared_ptr<Colonio::MessagingResponseWriter> response_writer) {
          user_thread_pool->push([this, handler, request, response_writer]() {
            handler(*this, *request, response_writer);
          });
        });
  } else {
    messaging->set_handler(
        name, [this, handler](
                  std::shared_ptr<const Colonio::MessagingRequest> request,
                  std::shared_ptr<Colonio::MessagingResponseWriter> response_writer) {
          handler(*this, *request, response_writer);
        });
  }
}

void ColonioImpl::messaging_unset_handler(const std::string& name) {
  assert(messaging);
  messaging->unset_handler(name);
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

  // messaging
  messaging = std::make_unique<Messaging>(logger, *command_manager);
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
  // coord_system module is required to build routing.
  // because of this, allocate only it before other modules.
  assert(!coord_system);

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
  messaging.reset();
}

void ColonioImpl::release_resources() {
  messaging.reset();
  network.reset();
  command_manager.reset();
  coord_system.reset();
  scheduler.reset();
  // user_thread_pool.reset();
  local_nid = NodeID::NONE;
}

/*

const NodeID& Network::module_2d_do_get_relay_nid(Module2D& module_2d, const Coordinate& position) {
  return routing->get_relay_nid_2d(position);
}

bool ColonioImpl::module_1d_do_check_covered_range(Module1D& module_1d, const NodeID& nid) {
  assert(routing);
  return routing->is_covered_range_1d(nid);
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
