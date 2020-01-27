/*
 * Copyright 2017-2019 Yuji Ito <llamerada.jp@gmail.com>
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
#include "coord_system_sphere.hpp"
#include "pubsub_2d/pubsub_2d_impl.hpp"
#include "routing_1d.hpp"
#include "utils.hpp"

namespace colonio {
ColonioImpl::ColonioImpl(Colonio& colonio_) :
    context(*this, *this),
    colonio(colonio_),
    is_first_link(true),
    enable_retry(true),
    node_accessor(nullptr),
    routing(nullptr),
    node_status(LinkStatus::OFFLINE),
    seed_status(LinkStatus::OFFLINE) {
#ifndef NDEBUG
  context.hook_on_debug_event(
      [this](DebugEvent::Type event, const picojson::value& json) { colonio.on_debug_event(event, json.serialize()); });
#endif
}

ColonioImpl::~ColonioImpl() {
}

void ColonioImpl::connect(
    const std::string& url, const std::string& token, std::function<void()> on_success,
    std::function<void()> on_failure) {
  if (context.link_status != LinkStatus::OFFLINE && context.link_status != LinkStatus::CLOSING) {
    loge(0x00010004, "Duplicate connection.");
    return;

  } else {
    logi(0x00010001, "Connect start.(url=%s)", url.c_str());
  }

  context.scheduler.add_interval_task(
      this,
      [this]() {
        for (auto& module : modules) {
          module.second->module_on_persec(seed_status, node_status);
        }
      },
      1000);

  is_first_link      = true;
  enable_retry       = true;
  on_connect_success = on_success;
  on_connect_failure = on_failure;

  seed_accessor = std::make_unique<SeedAccessor>(context, *this, url, token);
  node_accessor = new NodeAccessor(context, *this, *this);
  add_module(node_accessor);

  context.scheduler.add_timeout_task(
      this, [this]() { on_change_accessor_status(seed_accessor->get_status(), node_accessor->get_status()); }, 0);
}

const NodeID& ColonioImpl::get_my_nid() {
  return context.my_nid;
}

LinkStatus::Type ColonioImpl::get_status() {
  return context.link_status;
}

void ColonioImpl::disconnect() {
  enable_retry = false;
  seed_accessor->disconnect();
  node_accessor->disconnect_all();

  context.scheduler.add_timeout_task(
      this,
      [this]() {
        on_change_accessor_status(LinkStatus::OFFLINE, LinkStatus::OFFLINE);
        modules.clear();
        modules_named.clear();
        node_accessor = nullptr;
        routing       = nullptr;

        modules_1d.clear();
        modules_2d.clear();

        seed_accessor.reset();

        context.scheduler.remove_task(this);
      },
      0);
}

Coordinate ColonioImpl::set_position(const Coordinate& pos) {
  if (context.coord_system) {
    Coordinate old_position = context.get_my_position();
    context.set_my_position(pos);
    Coordinate new_position = context.get_my_position();
    if (old_position != new_position) {
      context.scheduler.add_timeout_task(
          this, [this, new_position]() { this->routing->on_change_my_position(new_position); }, 0);
      context.scheduler.add_timeout_task(
          this,
          [this, new_position]() {
            for (auto& it : this->modules_2d) {
              it->system_2d_on_change_my_position(new_position);
            }
          },
          0);
#ifndef NDEBUG
      context.debug_event(DebugEvent::POSITION, Convert::coordinate2json(new_position));
#endif
    }
    return new_position;

  } else {
    logd("coordinate system was not enabled");
    assert(false);
    return Coordinate();
  }
}

void ColonioImpl::logger_on_output(Logger& logger, LogLevel::Type level, const std::string& message) {
  colonio.on_output_log(level, message);
}

void ColonioImpl::module_do_send_packet(Module& module, std::unique_ptr<const Packet> packet) {
  relay_packet(std::move(packet), false);
}

void ColonioImpl::module_do_relay_packet(Module& module, const NodeID& dst_nid, std::unique_ptr<const Packet> packet) {
  assert(!dst_nid.is_special() || dst_nid == NodeID::NEXT);

  node_accessor->relay_packet(dst_nid, std::move(packet));
}

void ColonioImpl::node_accessor_on_change_online_links(NodeAccessor& na, const std::set<NodeID> nids) {
  if (routing) {
    routing->on_change_online_links(nids);
  }

#ifndef NDEBUG
  {
    picojson::array a;
    for (auto nid : nids) {
      a.push_back(picojson::value(nid.to_str()));
    }
    context.debug_event(DebugEvent::LINKS, picojson::value(a));
  }
#endif
}

void ColonioImpl::node_accessor_on_change_status(NodeAccessor& na, LinkStatus::Type status) {
  context.scheduler.add_timeout_task(
      this, [this, status]() { on_change_accessor_status(seed_accessor->get_status(), status); }, 0);
}

void ColonioImpl::node_accessor_on_recv_packet(
    NodeAccessor& na, const NodeID& nid, std::unique_ptr<const Packet> packet) {
  if (routing) {
    routing->on_recv_packet(nid, *packet);
  }
  relay_packet(std::move(packet), false);
}

void ColonioImpl::routing_do_connect_node(Routing& routing, const NodeID& nid) {
  if (node_accessor->get_status() == LinkStatus::ONLINE) {
    node_accessor->connect_link(nid);
  }
}

void ColonioImpl::routing_do_disconnect_node(Routing& routing, const NodeID& nid) {
  if (node_accessor->get_status() == LinkStatus::ONLINE) {
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

void ColonioImpl::routing_on_system_1d_change_nearby(Routing& routing, const NodeID& prev_nid, const NodeID& next_nid) {
  context.scheduler.add_timeout_task(
      this,
      [this, prev_nid, next_nid]() {
        for (auto& it : modules_1d) {
          it->system_1d_on_change_nearby(prev_nid, next_nid);
        }
      },
      0);
}

const NodeID& ColonioImpl::system_2d_do_get_relay_nid(const Coordinate& position) {
  return routing->get_relay_nid_2d(position);
}

void ColonioImpl::routing_on_system_2d_change_nearby(Routing& routing, const std::set<NodeID>& nids) {
  context.scheduler.add_timeout_task(
      this,
      [this, nids]() {
        for (auto& it : modules_2d) {
          it->system_2d_on_change_nearby(nids);
        }
      },
      0);
}

void ColonioImpl::routing_on_system_2d_change_nearby_position(
    Routing& routing, const std::map<NodeID, Coordinate>& positions) {
  context.scheduler.add_timeout_task(
      this,
      [this, positions]() {
        for (auto& it : modules_2d) {
          it->system_2d_on_change_nearby_position(positions);
        }
      },
      0);
}

void ColonioImpl::seed_accessor_on_change_status(SeedAccessor& sa, LinkStatus::Type status) {
  context.scheduler.add_timeout_task(
      this, [this, status]() { on_change_accessor_status(status, node_accessor->get_status()); }, 0);
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
        context.coord_system = std::make_unique<CoordSystemSphere>(cs2d_config);
      }
    }

    // routing
    routing = new Routing(
        context, *this, *this, ModuleChannel::SYSTEM_ROUTING, Utils::get_json<picojson::object>(config, "routing"));
    add_module(routing);

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

bool ColonioImpl::system_1d_do_check_coverd_range(const NodeID& nid) {
  assert(routing);
  return routing->is_coverd_range_1d(nid);
}

void ColonioImpl::scheduler_on_require_invoke(Scheduler& sched, unsigned int msec) {
  colonio.on_require_invoke(msec);
}

void ColonioImpl::add_module(Module* module, const std::string& name) {
  modules.insert(std::make_pair(module->channel, std::unique_ptr<Module>(module)));
  if (name != "") {
    modules_named.insert(std::make_pair(name, module));
  }
}

void ColonioImpl::initialize_algorithms() {
  const picojson::object& modules_config = Utils::get_json<picojson::object>(config, "modules", picojson::object());
  for (auto& it : modules_config) {
    const std::string& name               = it.first;
    const picojson::object& module_config = Utils::get_json<picojson::object>(modules_config, name);
    const std::string& type               = Utils::get_json<std::string>(module_config, "type");
    // ModuleChannel::Type channel = Utils::get_json<double>(module_config, "channel");

    if (type == "pubsub2D") {
      PubSub2DImpl* tmp = new PubSub2DImpl(context, *this, *this, module_config, 0);
      logd("add PubSub2D module channel:%d name:%s", tmp->channel, name.c_str());
      add_module(tmp, name);
      modules_2d.insert(tmp);

    } else if (type == "mapPaxos") {
      MapPaxos* tmp = new MapPaxos(context, *this, *this, module_config, 0);
      logd("add MapPaxos module channel:%d name:%s", tmp->channel, name.c_str());
      add_module(tmp, name);
      modules_1d.insert(tmp);

    } else {
      // @todo warning
      assert(false);
    }
  }
}

void ColonioImpl::on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status) {
  logd(
      "seed(%d) node(%d) auth(%d) onlyone(%d)", seed_status, node_status, seed_accessor->get_auth_status(),
      seed_accessor->is_only_one());

  assert(seed_status != LinkStatus::CLOSING);
  assert(node_status != LinkStatus::CLOSING);

  this->seed_status = seed_status;
  this->node_status = node_status;

  LinkStatus::Type status = LinkStatus::OFFLINE;

  if (node_status == LinkStatus::ONLINE) {
    status = LinkStatus::ONLINE;

  } else if (node_status == LinkStatus::CONNECTING) {
    if (seed_accessor->is_only_one() == true) {
      status = LinkStatus::ONLINE;
    } else {
      status = LinkStatus::CONNECTING;
    }

  } else if (seed_status == LinkStatus::ONLINE) {
    if (seed_accessor->is_only_one() == true) {
      status = LinkStatus::ONLINE;
    } else {
      node_accessor->connect_init_link();
      status = LinkStatus::CONNECTING;
    }

  } else if (seed_status == LinkStatus::CONNECTING) {
    status = LinkStatus::CONNECTING;

  } else if (!seed_accessor) {
    assert(node_accessor == nullptr);

    status = LinkStatus::OFFLINE;

  } else if (seed_accessor->get_auth_status() == AuthStatus::FAILURE) {
    loge(0x00010003, "Connect failure.");
    context.scheduler.add_timeout_task(this, on_connect_failure, 0);
    context.scheduler.add_timeout_task(this, std::bind(&ColonioImpl::disconnect, this), 0);
    status = LinkStatus::OFFLINE;

  } else if (enable_retry) {
    seed_accessor->connect();
    status = LinkStatus::CONNECTING;
  }

  if (context.link_status != status) {
    context.link_status = status;
    if (status == LinkStatus::ONLINE && is_first_link) {
      assert(seed_accessor->get_auth_status() == AuthStatus::SUCCESS);
      logi(0x00010002, "Connect success.");
      context.scheduler.add_timeout_task(this, on_connect_success, 0);
      is_first_link = false;
    }
  }

  for (auto& module : modules) {
    module.second->module_on_change_accessor_status(seed_status, node_status);
  }
}

void ColonioImpl::relay_packet(std::unique_ptr<const Packet> packet, bool is_from_seed) {
  if ((packet->mode & PacketMode::RELAY_SEED) != 0x0) {
    if ((packet->mode & PacketMode::REPLY) != 0x0 && !is_from_seed) {
      if (seed_status == LinkStatus::ONLINE) {
        seed_accessor->relay_packet(std::move(packet));
      } else {
        assert(routing);
        node_accessor->relay_packet(std::get<0>(routing->get_route_to_seed()), std::move(packet));
      }
      return;

    } else if (packet->src_nid == context.my_nid) {
      assert(seed_status == LinkStatus::ONLINE);
      seed_accessor->relay_packet(std::move(packet));
      return;
    }
  }

  assert(routing);
  NodeID dst_nid = routing->get_relay_nid_1d(*packet);

#ifndef NDEBUG
  std::string dump = Utils::dump_packet(*packet);
#endif

  if (dst_nid == NodeID::THIS || dst_nid == context.my_nid) {
    logd("Recv.(packet=%s)", dump.c_str());
    // Pass a packet to the target module.
    const Packet& p = *packet;  // @todo use std::move
    packet.release();
    context.scheduler.add_timeout_task(
        this,
        [this, p]() {
          auto it = modules.find(p.channel);
          if (it != modules.end()) {
            it->second->on_recv_packet(std::make_unique<Packet>(p));

          } else {
            // @todo error
            assert(false);
          }
        },
        0);
    return;

  } else if (!dst_nid.is_special() || dst_nid == NodeID::NEXT) {
    NodeID src_nid = packet->src_nid;
    if (node_accessor->relay_packet(dst_nid, std::move(packet))) {
      if (src_nid == context.my_nid) {
        logd("Send.(dst=%s packet=%s)", dst_nid.to_str().c_str(), dump.c_str());
      } else {
        logd("Relay.(dst=%s packet=%s)", dst_nid.to_str().c_str(), dump.c_str());
      }
      return;
    }
  }

  node_accessor->update_link_status();

  logd("drop packet %s\n", dump.c_str());
}
}  // namespace colonio
