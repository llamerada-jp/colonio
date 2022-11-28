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
    logger([this, logger_func = config.logger_func](const std::string& json) {
      logger_func(*this, json);
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
    colonio_throw_error(ErrorCode::SYSTEM_CONFLICT_WITH_SETTING, "coordinate system was not enabled");
  }

  Coordinate new_position(x, y);
  Coordinate old_position = coord_system->get_local_position();
  if (new_position != old_position) {
    coord_system->set_local_position(new_position);
    scheduler->add_task(this, [this, new_position]() {
      network->change_local_position(new_position);
      log_debug("current position").map("coordinate", Convert::coordinate2json(new_position));
    });
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

std::shared_ptr<std::map<std::string, Value>> ColonioImpl::kvs_get_local_data() {
  assert(scheduler && kvs);
  Pipe<std::shared_ptr<std::map<std::string, Value>>> pipe;

  scheduler->add_task(this, [this, &pipe]() {
    std::shared_ptr<std::map<std::string, Value>> data = kvs->get_local_data();
    pipe.push(data);
  });

  return *pipe.pop_with_throw();
}

void ColonioImpl::kvs_get_local_data(
    std::function<void(Colonio&, std::shared_ptr<std::map<std::string, Value>>)> handler) {
  assert(scheduler && kvs);

  scheduler->add_task(this, [this, handler]() {
    std::shared_ptr<std::map<std::string, Value>> data = kvs->get_local_data();
    if (user_thread_pool) {
      user_thread_pool->push([this, handler, data]() {
        handler(*this, data);
      });

    } else {
      handler(*this, data);
    }
  });
}

Value ColonioImpl::kvs_get(const std::string& key) {
  assert(scheduler && kvs);
  Pipe<Value> pipe;

  scheduler->add_task(this, [this, key, &pipe]() {
    kvs->get(
        key,
        [this, &pipe](const Value& value) {
          pipe.push(value);
        },
        [this, &pipe](const Error& error) {
          pipe.push_error(error);
        });
  });
  return *pipe.pop_with_throw();
}

void ColonioImpl::kvs_get(
    const std::string& key, std::function<void(Colonio&, const Value&)>&& on_success,
    std::function<void(Colonio&, const Error&)>&& on_failure) {
  assert(scheduler && kvs);

  scheduler->add_task(this, [this, key, on_success, on_failure]() {
    kvs->get(
        key,
        [this, on_success](const Value& value) {
          if (user_thread_pool) {
            user_thread_pool->push([this, on_success, value] {
              on_success(*this, value);
            });
          } else {
            on_success(*this, value);
          }
        },
        [this, on_failure](const Error& error) {
          if (user_thread_pool) {
            user_thread_pool->push([this, on_failure, error] {
              on_failure(*this, error);
            });
          } else {
            on_failure(*this, error);
          }
        });
  });
}

void ColonioImpl::kvs_set(const std::string& key, const Value& value, uint32_t opt) {
  assert(scheduler && kvs);
  Pipe<int> pipe;

  scheduler->add_task(this, [this, &pipe, key, value, opt]() {
    kvs->set(
        key, value, opt,
        [&pipe]() {
          pipe.push(1);
        },
        [&pipe](const Error& error) {
          pipe.push_error(error);
        });
  });

  pipe.pop_with_throw();
}

void ColonioImpl::kvs_set(
    const std::string& key, const Value& value, uint32_t opt, std::function<void(Colonio&)>&& on_success,
    std::function<void(Colonio&, const Error&)>&& on_failure) {
  assert(scheduler && kvs);

  scheduler->add_task(this, [this, key, value, opt, on_success, on_failure]() {
    kvs->set(
        key, value, opt,
        [this, on_success]() {
          if (user_thread_pool) {
            user_thread_pool->push([this, on_success] {
              on_success(*this);
            });

          } else {
            on_success(*this);
          }
        },
        [this, on_failure](const Error& error) {
          if (user_thread_pool) {
            user_thread_pool->push([this, on_failure, error] {
              on_failure(*this, error);
            });

          } else {
            on_failure(*this, error);
          }
        });
  });
}

void ColonioImpl::spread_post(
    double x, double y, double r, const std::string& name, const Value& message, uint32_t opt) {
  if (!coord_system) {
    colonio_throw_error(
        ErrorCode::SYSTEM_CONFLICT_WITH_SETTING, "coordinate system and spread feature were not enabled");
  }
  assert(scheduler && spread);

  Pipe<int> pipe;

  scheduler->add_task(this, [this, &pipe, x, y, r, name, message, opt]() {
    spread->post(
        x, y, r, name, message, opt,
        [&pipe]() {
          pipe.push(1);
        },
        [&pipe](const Error& error) {
          pipe.push_error(error);
        });
  });

  pipe.pop_with_throw();
}

void ColonioImpl::spread_post(
    double x, double y, double r, const std::string& name, const Value& message, uint32_t opt,
    std::function<void(Colonio&)>&& on_success, std::function<void(Colonio&, const Error&)>&& on_failure) {
  if (!coord_system) {
    colonio_throw_error(
        ErrorCode::SYSTEM_CONFLICT_WITH_SETTING, "coordinate system and spread feature were not enabled");
  }
  assert(scheduler && spread);

  scheduler->add_task(this, [this, x, y, r, name, message, opt, on_success, on_failure]() {
    spread->post(
        x, y, r, name, message, opt,
        [this, on_success]() {
          if (user_thread_pool) {
            user_thread_pool->push([this, on_success]() {
              on_success(*this);
            });
          } else {
            on_success(*this);
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

void ColonioImpl::spread_set_handler(
    const std::string& name, std::function<void(Colonio&, const SpreadRequest&)>&& handler) {
  if (!coord_system) {
    colonio_throw_error(
        ErrorCode::SYSTEM_CONFLICT_WITH_SETTING, "coordinate system and spread feature were not enabled");
  }
  assert(spread);
  if (user_thread_pool) {
    spread->set_handler(name, [this, handler](const Colonio::SpreadRequest& request) {
      user_thread_pool->push([this, handler, request]() {
        handler(*this, request);
      });
    });

  } else {
    spread->set_handler(name, [this, handler](const Colonio::SpreadRequest& request) {
      handler(*this, request);
    });
  }
}

void ColonioImpl::spread_unset_handler(const std::string& name) {
  if (!coord_system) {
    colonio_throw_error(
        ErrorCode::SYSTEM_CONFLICT_WITH_SETTING, "coordinate system and spread feature were not enabled");
  }
  assert(spread);
  spread->unset_handler(name);
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

  // kvs
  kvs = std::make_unique<KVS>(
      logger, random, *scheduler, *command_manager, *this,
      Utils::get_json<picojson::object>(global_config, "kvs", picojson::object()));

  // spread
  if (coord_system) {
    spread = std::make_unique<Spread>(
        logger, random, *scheduler, *command_manager, local_nid, *coord_system, *this,
        Utils::get_json<picojson::object>(global_config, "spread", picojson::object()));
  }
}

void ColonioImpl::network_on_change_nearby_1d(const NodeID& prev_nid, const NodeID& next_nid) {
  kvs->balance_records(prev_nid, next_nid);
}

void ColonioImpl::network_on_change_nearby_2d(const std::set<NodeID>& nids) {
  // @TODO unused?
}

void ColonioImpl::network_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) {
  if (spread) {
    spread->update_next_positions(positions);
  }
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

bool ColonioImpl::kvs_on_check_covered_range(const NodeID& nid) {
  return network->is_covered_range_1d(nid);
}

const NodeID& ColonioImpl::spread_do_get_relay_nid(const Coordinate& position) {
  return network->get_relay_nid_2d(position);
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
  kvs.reset();
  spread.reset();
  network.reset();
  command_manager.reset();
  coord_system.reset();
  scheduler.reset();
  // user_thread_pool.reset();
  local_nid = NodeID::NONE;
}

}  // namespace colonio
