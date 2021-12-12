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

#include "colonio_module.hpp"
#include "convert.hpp"
#include "coord_system_plane.hpp"
#include "coord_system_sphere.hpp"
#include "logger.hpp"
#include "map_impl.hpp"
#include "map_paxos/map_paxos_module.hpp"
#include "packet.hpp"
#include "pipe.hpp"
#include "pubsub_2d/pubsub_2d_module.hpp"
#include "pubsub_2d_impl.hpp"
#include "routing_1d.hpp"
#include "scheduler.hpp"
#include "utils.hpp"

namespace colonio {
ColonioImpl::ColonioImpl(std::function<void(Colonio&, const std::string&)> log_receiver_, uint32_t opt) :
    log_receiver(log_receiver_),
    logger(*this),
    scheduler(Scheduler::new_instance(logger, opt)),
    local_nid(NodeID::make_random(random)),
    module_param{*this, logger, random, *scheduler, local_nid},
    enable_retry(true),
    node_accessor(nullptr),
    routing(nullptr),
    colonio_module(nullptr),
    link_state(LinkState::OFFLINE) {
}

ColonioImpl::~ColonioImpl() {
  scheduler->remove_task(this);
}

void ColonioImpl::connect(const std::string& url, const std::string& token) {
  Pipe<int> pipe;

  connect(
      url, token,
      [&pipe](Colonio&) {
        pipe.push(1);
      },
      [&pipe](Colonio&, const Error& error) {
        pipe.pushError(error);
      });

  pipe.pop_with_throw();
}

void ColonioImpl::connect(
    const std::string& url, const std::string& token, std::function<void(Colonio&)>&& on_success,
    std::function<void(Colonio&, const Error&)>&& on_failure) {
  assert(!connect_cb);
  connect_cb.reset(new std::pair<std::function<void(Colonio&)>, std::function<void(Colonio&, const Error&)>>(
      on_success, on_failure));

  try {
    if (link_state != LinkState::OFFLINE && link_state != LinkState::CLOSING) {
      loge("duplicate connection.");
      scheduler->add_user_task(this, [this]() {
        auto d = Utils::defer([this] {
          connect_cb.reset();
        });
        connect_cb->second(*this, colonio_error(ErrorCode::CONNECTION_FAILD, "duplicate connectin."));
      });

    } else {
      logi("connect start").map("url", url);
    }

    enable_retry = true;

    seed_accessor = std::make_unique<SeedAccessor>(module_param, *this, url, token);
    node_accessor = new NodeAccessor(module_param, *this);
    register_module(node_accessor, nullptr, false, false);
    colonio_module = new ColonioModule(module_param);
    register_module(colonio_module, nullptr, false, false);

    scheduler->add_controller_task(this, [this]() {
      update_accessor_state();
    });

  } catch (Error& e) {
    if (connect_cb) {
      auto d = Utils::defer([this] {
        connect_cb.reset();
      });
      connect_cb->second(*this, e);
    }
  }
}

void ColonioImpl::disconnect() {
  Pipe<int> pipe;

  disconnect(
      [&pipe](Colonio&) {
        pipe.push(1);
      },
      [&pipe](Colonio&, const Error& error) {
        pipe.pushError(error);
      });

  pipe.pop_with_throw();
}

void ColonioImpl::disconnect(
    std::function<void(Colonio&)>&& on_success_, std::function<void(Colonio&, const Error&)>&& on_failure_) {
  std::function<void(Colonio&)> on_success = on_success_;

  scheduler->add_controller_task(this, [this, on_success] {
    enable_retry = false;
    seed_accessor->disconnect();
    node_accessor->disconnect_all([this, on_success]() {
      scheduler->add_controller_task(this, [this, on_success] {
        link_state = LinkState::OFFLINE;
        for (auto& module : modules) {
          module.second->module_on_change_accessor_state(LinkState::OFFLINE, LinkState::OFFLINE);
        }

        clear_modules();

        scheduler->remove_task(this, false);
        scheduler->add_user_task(this, [this, on_success]() {
          on_success(*this);
        });
      });
    });
  });
}

bool ColonioImpl::is_connected() {
  return link_state == LinkState::ONLINE;
}

std::string ColonioImpl::get_local_nid() {
  if (!is_connected()) {
    return "";
  }

  return local_nid.to_str();
}

Map& ColonioImpl::access_map(const std::string& name) {
  auto it = if_map.find(name);
  if (it == if_map.end()) {
    colonio_throw_error(ErrorCode::CONFLICT_WITH_SETTING, "map module not found : %s", name.c_str());
  }

  return *it->second;
}

Pubsub2D& ColonioImpl::access_pubsub_2d(const std::string& name) {
  auto it = if_pubsub2d.find(name);
  if (it == if_pubsub2d.end()) {
    colonio_throw_error(ErrorCode::CONFLICT_WITH_SETTING, "pubsub module not found : %s", name.c_str());
  }

  return *it->second;
}

std::tuple<double, double> ColonioImpl::set_position(double x, double y) {
  Pipe<std::tuple<double, double>> pipe;

  set_position(
      x, y,
      [&pipe](Colonio&, double rx, double ry) {
        pipe.push(std::make_tuple(rx, ry));
      },
      [&pipe](Colonio&, const Error& error) {
        pipe.pushError(error);
      });

  return *pipe.pop_with_throw();
}

void ColonioImpl::set_position(
    double x, double y, std::function<void(Colonio&, double, double)> on_success,
    std::function<void(Colonio&, const Error&)> on_failure) {
  if (!coord_system) {
    on_failure(*this, colonio_error(ErrorCode::CONFLICT_WITH_SETTING, "coordinate system was not enabled"));
  }

  scheduler->add_controller_task(this, [this, x, y, on_success] {
    Coordinate new_position(x, y);
    Coordinate old_position = coord_system->get_local_position();
    if (new_position != old_position) {
      coord_system->set_local_position(new_position);
      for (auto& it : modules_2d) {
        it->module_2d_on_change_local_position(new_position);
      }
      routing->on_change_local_position(new_position);
      logd("current position").map("coordinate", Convert::coordinate2json(new_position));
    }

    Coordinate ret_position = coord_system->get_local_position();

    scheduler->add_user_task(this, [this, on_success, ret_position] {
      on_success(*this, ret_position.x, ret_position.y);
    });
  });
}

void ColonioImpl::send(const std::string& dst_nid, const Value& value, uint32_t opt) {
  Pipe<int> pipe;

  send(
      dst_nid, value, opt,
      [&pipe](Colonio&) {
        pipe.push(1);
      },
      [&pipe](Colonio&, const Error& error) {
        pipe.pushError(error);
      });

  pipe.pop_with_throw();
}

void ColonioImpl::send(
    const std::string& dst_nid, const Value& value, uint32_t opt, std::function<void(Colonio&)>&& on_success,
    std::function<void(Colonio&, const Error&)>&& on_failure) {
  try {
    colonio_module->send(
        dst_nid, value, opt,
        [this, on_success] {
          on_success(*this);
        },
        [this, on_failure](const Error& err) {
          on_failure(*this, err);
        });
  } catch (Error& e) {
    on_failure(*this, e);
  }
}

void ColonioImpl::on(std::function<void(Colonio&, const Value&)>&& receiver) {
  colonio_module->on([this, receiver](const Value& value) {
    receiver(*this, value);
  });
}

void ColonioImpl::off() {
  colonio_module->off();
}

void ColonioImpl::start_on_event_thread() {
  scheduler->start_user_routine();
}

void ColonioImpl::start_on_controller_thread() {
  scheduler->start_controller_routine();
}

void ColonioImpl::logger_on_output(Logger& logger, const std::string& json) {
  log_receiver(*this, json);
}

void ColonioImpl::module_do_send_packet(ModuleBase& module, std::unique_ptr<const Packet> packet) {
  relay_packet(std::move(packet), false);
}

void ColonioImpl::module_do_relay_packet(
    ModuleBase& module, const NodeID& dst_nid, std::unique_ptr<const Packet> packet) {
  assert(!dst_nid.is_special() || dst_nid == NodeID::NEXT);

  node_accessor->relay_packet(dst_nid, std::move(packet));
}

void ColonioImpl::node_accessor_on_change_online_links(NodeAccessor& na, const std::set<NodeID>& nids) {
  if (routing) {
    routing->on_change_online_links(nids);
  }

#ifndef NDEBUG
  {
    picojson::array a;
    for (auto& nid : nids) {
      a.push_back(picojson::value(nid.to_str()));
    }
    logd("links").map("nids", picojson::value(a));
  }
#endif
}

void ColonioImpl::node_accessor_on_change_state(NodeAccessor& na) {
  scheduler->add_controller_task(this, [this]() {
    update_accessor_state();
  });
}

void ColonioImpl::node_accessor_on_recv_packet(
    NodeAccessor& na, const NodeID& nid, std::unique_ptr<const Packet> packet) {
  if (routing) {
    routing->on_recv_packet(nid, *packet);
  }
  relay_packet(std::move(packet), false);
}

void ColonioImpl::routing_do_connect_node(Routing& routing, const NodeID& nid) {
  if (node_accessor->get_link_state() == LinkState::ONLINE) {
    node_accessor->connect_link(nid);
  }
}

void ColonioImpl::routing_do_disconnect_node(Routing& routing, const NodeID& nid) {
  if (node_accessor->get_link_state() == LinkState::ONLINE) {
    node_accessor->disconnect_link(nid);
  }
}

void ColonioImpl::routing_do_connect_seed(Routing& route) {
  if (enable_retry) {
    seed_accessor->connect();
  }
}

void ColonioImpl::routing_do_disconnect_seed(Routing& route) {
  seed_accessor->disconnect();
}

void ColonioImpl::routing_on_module_1d_change_nearby(Routing& routing, const NodeID& prev_nid, const NodeID& next_nid) {
  scheduler->add_controller_task(this, [this, prev_nid, next_nid]() {
    for (auto& it : modules_1d) {
      it->module_1d_on_change_nearby(prev_nid, next_nid);
    }
  });
}

const NodeID& ColonioImpl::module_2d_do_get_relay_nid(Module2D& module_2d, const Coordinate& position) {
  return routing->get_relay_nid_2d(position);
}

void ColonioImpl::routing_on_module_2d_change_nearby(Routing& routing, const std::set<NodeID>& nids) {
  scheduler->add_controller_task(this, [this, nids]() {
    for (auto& it : modules_2d) {
      it->module_2d_on_change_nearby(nids);
    }
  });
}

void ColonioImpl::routing_on_module_2d_change_nearby_position(
    Routing& routing, const std::map<NodeID, Coordinate>& positions) {
  scheduler->add_controller_task(this, [this, positions]() {
    for (auto& it : modules_2d) {
      it->module_2d_on_change_nearby_position(positions);
    }
  });
}

void ColonioImpl::seed_accessor_on_change_state(SeedAccessor& sa) {
  scheduler->add_controller_task(this, [this]() {
    update_accessor_state();
  });
}

void ColonioImpl::seed_accessor_on_recv_config(SeedAccessor& sa, const picojson::object& newconfig) {
  if (config.empty()) {
    config = newconfig;
    // node_accessor
    node_accessor->initialize(config);

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

    // routing
    routing =
        new Routing(module_param, *this, coord_system.get(), Utils::get_json<picojson::object>(config, "routing"));
    register_module(routing, nullptr, false, false);

    // modules
    initialize_algorithms();

  } else {
    // check revision
    if (newconfig.at("revision").get<double>() != config.at("revision").get<double>()) {
      // @todo Warn new revision to the application.
    }
  }
}

void ColonioImpl::seed_accessor_on_recv_packet(SeedAccessor& sa, std::unique_ptr<const Packet> packet) {
  relay_packet(std::move(packet), true);
}

void ColonioImpl::seed_accessor_on_recv_require_random(SeedAccessor& sa) {
  if (node_accessor != nullptr) {
    node_accessor->connect_random_link();
  }
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

void ColonioImpl::update_accessor_state() {
  LinkState::Type seed_status = seed_accessor->get_link_state();
  LinkState::Type node_status = node_accessor->get_link_state();

  logd("link status")
      .map_int("seed", seed_status)
      .map_int("node", node_status)
      .map_int("auth", seed_accessor->get_auth_status())
      .map_bool("onlyone", seed_accessor->is_only_one());

  LinkState::Type status = LinkState::OFFLINE;

  if (node_status == LinkState::ONLINE) {
    status = LinkState::ONLINE;

  } else if (node_status == LinkState::CONNECTING) {
    if (seed_accessor->is_only_one() == true) {
      status = LinkState::ONLINE;
    } else {
      status = LinkState::CONNECTING;
    }

  } else if (seed_status == LinkState::ONLINE) {
    if (seed_accessor->is_only_one() == true) {
      status = LinkState::ONLINE;
    } else {
      node_accessor->connect_init_link();
      status = LinkState::CONNECTING;
    }

  } else if (seed_status == LinkState::CONNECTING) {
    status = LinkState::CONNECTING;

  } else if (!seed_accessor) {
    assert(node_accessor == nullptr);

    status = LinkState::OFFLINE;

  } else if (seed_accessor->get_auth_status() == AuthStatus::FAILURE) {
    loge("connect failure");
    if (connect_cb) {
      scheduler->add_user_task(this, [this] {
        auto d = Utils::defer([this] {
          connect_cb.reset();
        });
        connect_cb->second(*this, colonio_error(ErrorCode::OFFLINE, "Connect failure."));
      });
    }

    scheduler->add_controller_task(this, [this]() {
      disconnect();
    });
    status = LinkState::OFFLINE;

  } else if (enable_retry) {
    seed_accessor->connect();
    status = LinkState::CONNECTING;
  }

  link_state = status;
  check_api_connect();

  for (auto& module : modules) {
    module.second->module_on_change_accessor_state(seed_status, node_status);
  }
}

void ColonioImpl::relay_packet(std::unique_ptr<const Packet> packet, bool is_from_seed) {
  assert(packet->channel != Channel::NONE);

  if ((packet->mode & PacketMode::RELAY_SEED) != 0x0) {
    LinkState::Type seed_status = seed_accessor->get_link_state();

    if ((packet->mode & PacketMode::REPLY) != 0x0 && !is_from_seed) {
      if (seed_status == LinkState::ONLINE) {
        seed_accessor->relay_packet(std::move(packet));
      } else {
        assert(routing);
        node_accessor->relay_packet(routing->get_route_to_seed(), std::move(packet));
      }
      return;

    } else if (packet->src_nid == local_nid) {
      if (seed_status == LinkState::ONLINE) {
        seed_accessor->relay_packet(std::move(packet));
      } else {
        logw("drop packet").map("packet", *packet);
      }
      return;
    }
  }

  assert(routing);
  const NodeID& dst_nid = routing->get_relay_nid_1d(*packet);

  if (dst_nid == NodeID::THIS || dst_nid == local_nid) {
    logd("receive packet").map("packet", *packet);
    // Pass a packet to the target module.
    const Packet* p = packet.get();
    packet.release();
    scheduler->add_controller_task(this, [this, p]() mutable {
      assert(p != nullptr);
      std::unique_ptr<const Packet> packet(p);

      auto module = modules.find(packet->channel);
      if (module != modules.end()) {
        module->second->on_recv_packet(std::move(packet));
      } else {
#warning dump-packet
        colonio_throw_error(
            ErrorCode::INCORRECT_DATA_FORMAT, "received incorrect packet entry", Utils::dump_packet(*packet).c_str());
      }
    });
    return;

  } else if (!dst_nid.is_special() || dst_nid == NodeID::NEXT) {
#ifndef NDEBUG
    const NodeID src_nid = packet->src_nid;
    const Packet copy    = *packet;
#endif
    if (node_accessor->relay_packet(dst_nid, std::move(packet))) {
#ifndef NDEBUG
      if (src_nid == local_nid) {
        logd("send packet").map("dst", dst_nid).map("packet", copy);
      } else {
        logd("relay packet").map("dst", dst_nid).map("packet", copy);
      }
#endif
      return;
    }
  }

  if (packet) {
    logw("drop packet").map("packet", *packet);
  }
}
}  // namespace colonio
