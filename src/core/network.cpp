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
#include "network.hpp"

#include <cassert>

#include "colonio/colonio.hpp"
#include "command_manager.hpp"
#include "logger.hpp"
#include "scheduler.hpp"

namespace colonio {

NetworkDelegate::~NetworkDelegate() {
}

Network::Network(Logger& l, Random& r, Scheduler& s, CommandManager& c, const NodeID& n, NetworkDelegate& d) :
    logger(l),
    random(r),
    scheduler(s),
    command_manager(c),
    local_nid(n),
    delegate(d),
    enforce_online(false),
    link_state(LinkState::OFFLINE) {
}

Network::~Network() {
}

void Network::connect(
    const std::string& url, const std::string& token, std::function<void()>&& on_success,
    std::function<void(const Error&)>&& on_failure) {
  if (enforce_online || (link_state != LinkState::OFFLINE && link_state != LinkState::CLOSING)) {
    log_error("duplicate connection.");
    on_failure(colonio_error(ErrorCode::CONNECTION_FAILED, "duplicate connections."));
  }

  try {
    log_info("connect start").map("url", url);

    assert(!cb_connect_success);
    assert(!cb_connect_failure);
    cb_connect_success = on_success;
    cb_connect_failure = on_failure;

    enforce_online = true;

    seed_accessor = std::make_unique<SeedAccessor>(logger, scheduler, local_nid, *this, url, token);
    node_accessor = std::make_unique<NodeAccessor>(logger, scheduler, command_manager, local_nid, *this);

    scheduler.add_task(this, std::bind(&Network::update_accessor_state, this));

  } catch (Error& e) {
    cb_connect_success = nullptr;
    cb_connect_failure = nullptr;

    on_failure(e);
  }
}

void Network::disconnect(std::function<void()>&& on_success, std::function<void(const Error&)>&& on_failure) {
  scheduler.add_task(this, [this, on_success] {
    enforce_online = false;
    seed_accessor->disconnect();
    node_accessor->disconnect_all([this, on_success]() {
      scheduler.add_task(this, [this, on_success] {
        link_state = LinkState::OFFLINE;

        // TODO: tell state of network to Routing or other modules required.
        // for (auto& module : modules) {
        //   module.second->module_on_change_accessor_state(LinkState::OFFLINE, LinkState::OFFLINE);
        // }
        // clear_modules();

        scheduler.remove(this, false);
        if (on_success) {
          on_success();
        }
      });
    });
  });
}

bool Network::is_connected() {
  return link_state == LinkState::ONLINE;
}

void Network::change_local_position(const Coordinate& position) {
  routing->on_change_local_position(position);
}

bool Network::is_covered_range_1d(const NodeID& nid) {
  return routing->is_covered_range_1d(nid);
}

const NodeID& Network::get_relay_nid_2d(const Coordinate& position) {
  return routing->get_relay_nid_2d(position);
}

void Network::update_accessor_state() {
  LinkState::Type seed_status = seed_accessor->get_link_state();
  LinkState::Type node_status = node_accessor->get_link_state();

  log_debug("link status")
      .map_int("seed", seed_status)
      .map_int("node", node_status)
      .map_int("auth", seed_accessor->get_auth_status())
      .map_bool("only_one", seed_accessor->is_only_one());

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
    log_error("connect failure");
    if (cb_connect_failure) {
      cb_connect_failure(colonio_error(ErrorCode::CONNECTION_OFFLINE, "Connect failure."));
      cb_connect_success = nullptr;
      cb_connect_failure = nullptr;
    }

    scheduler.add_task(this, std::bind(&Network::disconnect, this, nullptr, nullptr));
    status = LinkState::OFFLINE;

  } else if (enforce_online) {
    seed_accessor->connect();
    status = LinkState::CONNECTING;
  }

  link_state = status;

  if (cb_connect_success && link_state == LinkState::ONLINE) {
    cb_connect_success();
    cb_connect_success = nullptr;
    cb_connect_failure = nullptr;
    return;
  }
}

void Network::node_accessor_on_change_online_links(const std::set<NodeID>& nids) {
  if (routing) {
    routing->on_change_online_links(nids);
  }

#ifndef NDEBUG
  {
    picojson::array a;
    for (auto& nid : nids) {
      a.push_back(picojson::value(nid.to_str()));
    }
    log_debug("links").map("nids", picojson::value(a));
  }
#endif
}

void Network::node_accessor_on_change_state() {
  scheduler.add_task(this, [this]() {
    update_accessor_state();
    if (routing) {
      routing->on_change_network_state(seed_accessor->get_link_state(), node_accessor->get_link_state());
    }
  });
}

void Network::node_accessor_on_recv_packet(const NodeID& nid, std::unique_ptr<const Packet> packet) {
  if (routing) {
    routing->on_recv_packet(nid, *packet);
  }
  switch_packet(std::move(packet), false);
}

void Network::switch_packet(std::unique_ptr<const Packet> packet, bool is_from_seed) {
  if ((packet->mode & PacketMode::RELAY_SEED) != 0x0) {
    LinkState::Type seed_status = seed_accessor->get_link_state();

    if ((packet->mode & PacketMode::RESPONSE) != 0x0 && !is_from_seed) {
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
        log_warn("drop packet").map("packet", *packet);
      }
      return;
    }
  }

  assert(routing);
  const NodeID& dst_nid = routing->get_relay_nid_1d(*packet);

  if (dst_nid == NodeID::THIS || dst_nid == local_nid) {
    log_debug("receive packet").map("packet", *packet);
    // Pass a packet to the target module.
    const Packet* p = packet.release();
    scheduler.add_task(this, [this, p] {
      assert(p != nullptr);

      command_manager.receive_packet(std::unique_ptr<const Packet>(p));
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
        log_debug("send packet").map("dst", dst_nid).map("packet", copy);
      } else {
        log_debug("relay packet").map("dst", dst_nid).map("packet", copy);
      }
#endif
      return;
    }

  } else if (dst_nid == NodeID::NONE && (packet->mode & PacketMode::ONE_WAY) == 0x0) {
    std::unique_ptr<proto::PacketContent> pc = std::make_unique<proto::PacketContent>();
    proto::Error& content                    = *pc->mutable_error();
    content.set_code(static_cast<uint32_t>(ErrorCode::PACKET_NO_ONE_RECV));

    PacketMode::Type packet_mode = PacketMode::RESPONSE | PacketMode::EXPLICIT | PacketMode::ONE_WAY;
    if (packet->mode & PacketMode::RELAY_SEED) {
      packet_mode |= PacketMode::RELAY_SEED;
    }
    std::unique_ptr<Packet> error_response =
        std::make_unique<Packet>(packet->src_nid, local_nid, packet->id, std::move(pc), packet_mode);
    switch_packet(std::move(error_response), false);
    return;
  }

  if (packet) {
    log_warn("drop packet").map("packet", *packet);
  }
}

void Network::relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet) {
  node_accessor->relay_packet(dst_nid, std::move(packet));
}

void Network::routing_do_connect_node(const NodeID& nid) {
  if (enforce_online && node_accessor->get_link_state() == LinkState::ONLINE) {
    node_accessor->connect_link(nid);
  }
}

void Network::routing_do_disconnect_node(const NodeID& nid) {
  if (node_accessor->get_link_state() == LinkState::ONLINE) {
    node_accessor->disconnect_link(nid);
  }
}

void Network::routing_do_connect_seed() {
  if (enforce_online) {
    seed_accessor->connect();
  }
}

void Network::routing_do_disconnect_seed() {
  seed_accessor->disconnect();
}

void Network::routing_on_module_1d_change_nearby(const NodeID& prev_nid, const NodeID& next_nid) {
  delegate.network_on_change_nearby_1d(prev_nid, next_nid);
}

void Network::routing_on_module_2d_change_nearby(const std::set<NodeID>& nids) {
  delegate.network_on_change_nearby_2d(nids);
}

void Network::routing_on_module_2d_change_nearby_position(const std::map<NodeID, Coordinate>& positions) {
  delegate.network_on_change_nearby_position(positions);
}

void Network::seed_accessor_on_change_state() {
  scheduler.add_task(this, [this]() {
    update_accessor_state();
    if (routing) {
      routing->on_change_network_state(seed_accessor->get_link_state(), node_accessor->get_link_state());
    }
  });
}

void Network::seed_accessor_on_recv_config(const picojson::object& config) {
  if (!routing) {
    // node_accessor
    node_accessor->initialize(config);

    // routing
    routing = std::make_unique<Routing>(
        logger, random, scheduler, command_manager, local_nid, *this, delegate.network_on_require_coord_system(config),
        Utils::get_json<picojson::object>(config, "routing"));
  }

  delegate.network_on_change_global_config(config);
}

void Network::seed_accessor_on_recv_packet(std::unique_ptr<const Packet> packet) {
  switch_packet(std::move(packet), true);
}

void Network::seed_accessor_on_recv_require_random() {
  if (node_accessor != nullptr) {
    node_accessor->connect_random_link();
  }
}
}  // namespace colonio
