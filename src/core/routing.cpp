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
#include "routing.hpp"

#include <algorithm>
#include <cassert>
#include <map>
#include <set>
#include <tuple>
#include <vector>

#include "context.hpp"
#include "convert.hpp"
#include "routing_1d.hpp"
#include "routing_2d.hpp"
#include "utils.hpp"

namespace colonio {

RoutingDelegate::~RoutingDelegate() {
}

RoutingAlgorithm::RoutingAlgorithm(const std::string& name_) : name(name_) {
}

RoutingAlgorithm::~RoutingAlgorithm() {
}

RoutingAlgorithm1DDelegate::~RoutingAlgorithm1DDelegate() {
}

RoutingAlgorithm2DDelegate::~RoutingAlgorithm2DDelegate() {
}

/**
 * Constructor, set using reference values.
 * @param delegate_ Delegate instance (should WebrtcBundle).
 */
Routing::Routing(
    Context& context, ModuleDelegate& module_delegate, RoutingDelegate& routing_delegate, ModuleChannel::Type channel,
    const picojson::object& config) :
    Module(context, module_delegate, channel, 0),
    CONFIG_UPDATE_PERIOD(Utils::get_json(config, "updatePeriod", ROUTING_UPDATE_PERIOD)),
    CONFIG_FORCE_UPDATE_TIMES(Utils::get_json(config, "forceUpdateTimes", ROUTING_FORCE_UPDATE_TIMES)),
    delegate(routing_delegate),
    routing_1d(nullptr),
    routing_2d(nullptr),
    node_status(LinkStatus::OFFLINE),
    seed_status(LinkStatus::OFFLINE),
    RANDOM_WAIT_SEED_CONNECTION(Context::get_rnd_32() % ROUTING_SEED_RANDOM_WAIT),
    passed_seed_connection(0),
    routing_countdown(CONFIG_FORCE_UPDATE_TIMES) {
  routing_1d = new Routing1D(context, *this);
  algorithms.push_back(std::unique_ptr<RoutingAlgorithm>(routing_1d));

  if (context.coord_system) {
    routing_2d = new Routing2D(context, *this);
    algorithms.push_back(std::unique_ptr<RoutingAlgorithm>(routing_2d));
  }

  // set a watch.
  dists_from_seed.insert(std::make_pair(NodeID::NONE, std::make_pair(NodeID::NONE, INT32_MAX)));

  // add task
  context.scheduler.add_interval_task(this, std::bind(&Routing::update, this), CONFIG_UPDATE_PERIOD);
}

Routing::~Routing() {
  context.scheduler.remove_task(this);
}

const NodeID& Routing::get_relay_nid_1d(const Packet& packet) {
  return routing_1d->get_relay_nid(packet);
}

bool Routing::is_coverd_range_1d(const NodeID& nid) {
  return routing_1d->is_coverd_range(nid);
}

const NodeID& Routing::get_relay_nid_2d(const Coordinate& dest) {
  assert(routing_2d != nullptr);
  return routing_2d->get_relay_nid(dest);
}

std::tuple<const NodeID&, const NodeID&, uint32_t> Routing::get_route_to_seed() {
  return std::make_tuple(
      std::ref(next_to_seed), std::ref(dists_from_seed.at(next_to_seed).first),
      dists_from_seed.at(next_to_seed).second);
}

bool Routing::is_direct_connect(const NodeID& nid) {
  return online_links.find(nid) != online_links.end();
}

void Routing::on_change_my_position(const Coordinate& position) {
  for (auto& algorithm : algorithms) {
    algorithm->on_change_my_position(position);
  }
}

/**
 * @param nids A set of links those are online.
 */
void Routing::on_change_online_links(const std::set<NodeID>& nids) {
  online_links            = nids;
  has_update_online_links = true;

  auto it = dists_from_seed.begin();
  while (it != dists_from_seed.end()) {
    const NodeID& nid = it->first;
    if (nid != NodeID::NONE && nids.find(nid) == nids.end()) {
      it = dists_from_seed.erase(it);
    } else {
      it++;
    }
  }

  update_route_to_seed();
  routing_countdown = 0;
}

void Routing::on_recv_packet(const NodeID& nid, const Packet& packet) {
  for (auto& algorithm : algorithms) {
    algorithm->on_recv_packet(nid, packet);
  }
}

void Routing::algorithm_1d_on_change_nearby(
    RoutingAlgorithm& algorithm, const NodeID& prev_nid, const NodeID& next_nid) {
  delegate.routing_on_system_1d_change_nearby(*this, prev_nid, next_nid);
}

void Routing::algorithm_2d_on_change_nearby(RoutingAlgorithm& algorithm, const std::set<NodeID>& nids) {
  delegate.routing_on_system_2d_change_nearby(*this, nids);
}

void Routing::algorithm_2d_on_change_nearby_position(
    RoutingAlgorithm& algorithm, const std::map<NodeID, Coordinate>& positions) {
  delegate.routing_on_system_2d_change_nearby_position(*this, positions);
}

void Routing::module_on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status) {
  this->seed_status = seed_status;
  this->node_status = node_status;

  routing_countdown = 0;
}

void Routing::module_process_command(std::unique_ptr<const Packet> packet) {
  switch (packet->command_id) {
    case CommandID::Routing::ROUTING:
      recv_routing_info(std::move(packet));
      break;

    default:
      // TODO(llamerada.jp@gmail.com) Warning on recving invalid packet.
      assert(false);
  }
}

void Routing::recv_routing_info(std::unique_ptr<const Packet> packet) {
  RoutingProtocol::RoutingInfo content;
  packet->parse_content(&content);
  NodeID seed_nid = NodeID::from_pb(content.seed_nid());
  if (seed_nid == context.local_nid) {
    if (seed_status == LinkStatus::OFFLINE && dists_from_seed.find(packet->src_nid) != dists_from_seed.end()) {
      dists_from_seed.erase(packet->src_nid);
    }
  } else {
    uint32_t distance = content.seed_distance();
    if (dists_from_seed.find(packet->src_nid) != dists_from_seed.end() &&
        dists_from_seed.at(packet->src_nid).second != distance) {
      routing_countdown = 0;
    }
    dists_from_seed[packet->src_nid] = std::make_pair(seed_nid, distance);
  }

  {
    auto it = routing_infos.find(packet->src_nid);
    if (it == routing_infos.end()) {
      routing_infos.insert(std::make_pair(packet->src_nid, std::make_tuple(std::move(packet), content)));
    } else {
      std::get<0>(it->second).swap(packet);
      std::get<1>(it->second) = content;
    }
  }

  update_route_to_seed();
}

/**
 * Send routing packet.
 */
void Routing::send_routing_info() {
  RoutingProtocol::RoutingInfo param;

  for (auto& algorithm : algorithms) {
    algorithm->send_routing_info(&param);
  }

  if (seed_status == LinkStatus::ONLINE) {
    param.set_seed_distance(1);
    context.local_nid.to_pb(param.mutable_seed_nid());

  } else {
    NodeID seed_nid;
    uint32_t distance;
    std::tie(seed_nid, distance) = dists_from_seed.at(next_to_seed);

    param.set_seed_distance(distance + 1);
    seed_nid.to_pb(param.mutable_seed_nid());
  }

  send_packet(NodeID::NEXT, PacketMode::EXPLICIT, CommandID::Routing::ROUTING, serialize_pb(param));
}

void Routing::update() {
  update_seed_connection();

  if (node_status == LinkStatus::ONLINE) {
    for (auto& row : routing_infos) {
      std::get<0>(row.second)->parse_content(&std::get<1>(row.second));
    }

    for (auto& algorithm : algorithms) {
      if (algorithm->update_routing_info(online_links, has_update_online_links, routing_infos)) {
        routing_countdown = 0;
      }
    }

    update_node_connection();

    if (routing_countdown <= 0) {
      routing_countdown = CONFIG_FORCE_UPDATE_TIMES;
      send_routing_info();
    } else {
      routing_countdown -= 1;
    }
  }

  has_update_online_links = false;
  routing_infos.clear();
}

void Routing::update_node_connection() {
  std::set<NodeID> required_nids;
  for (auto& algorithm : algorithms) {
    for (auto& nid : algorithm->get_required_nodes()) {
      required_nids.insert(nid);
    }
  }

  for (auto& nid : required_nids) {
    if (online_links.find(nid) == online_links.end()) {
      delegate.routing_do_connect_node(*this, nid);
    }
  }

  for (auto& nid : online_links) {
    if (required_nids.find(nid) == required_nids.end()) {
      delegate.routing_do_disconnect_node(*this, nid);
    }
  }
}

void Routing::update_route_to_seed() {
  const NodeID node_prev = next_to_seed;

  uint32_t min = UINT32_MAX;
  next_to_seed = NodeID::NONE;

  for (auto& it : dists_from_seed) {
    if (it.second.second < min) {
      min          = it.second.second;
      next_to_seed = it.first;
    }
  }

  if (node_prev != next_to_seed) {
    routing_countdown = 0;
  }
}

void Routing::update_seed_connection() {
  uint32_t distance = dists_from_seed.at(next_to_seed).second;

  if (node_status == LinkStatus::ONLINE && seed_status == LinkStatus::ONLINE &&
      distance < ROUTING_SEED_DISCONNECT_STEP) {
    passed_seed_connection += 1000;
    if (passed_seed_connection > RANDOM_WAIT_SEED_CONNECTION) {
      delegate.routing_do_disconnect_seed(*this);
    }

  } else if (seed_status == LinkStatus::OFFLINE && distance > ROUTING_SEED_CONNECT_STEP) {
    passed_seed_connection -= 1000;
    if (passed_seed_connection < -RANDOM_WAIT_SEED_CONNECTION) {
      delegate.routing_do_connect_seed(*this);
    }

  } else {
    passed_seed_connection = 0;
  }
}
}  // namespace colonio
