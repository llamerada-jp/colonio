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
#include "routing.hpp"

#include <algorithm>
#include <cassert>
#include <map>
#include <set>
#include <tuple>
#include <vector>

#include "convert.hpp"
#include "logger.hpp"
#include "packet.hpp"
#include "random.hpp"
#include "routing_1d.hpp"
#include "routing_2d.hpp"
#include "scheduler.hpp"
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
    ModuleParam& param, RoutingDelegate& routing_delegate, const CoordSystem* coord_system,
    const picojson::object& config) :
    ModuleBase(param, Channel::SYSTEM_ROUTING),

    CONFIG_FORCE_UPDATE_COUNT(Utils::get_json(config, "forceUpdateCount", ROUTING_FORCE_UPDATE_COUNT)),
    CONFIG_SEED_CONNECT_INTERVAL(Utils::get_json(config, "seedConnectInterval", ROUTING_SEED_CONNECT_INTERVAL)),
    CONFIG_SEED_CONNECT_RATE(Utils::get_json(config, "seedConnectRate", ROUTING_SEED_CONNECT_RATE)),
    CONFIG_SEED_DISCONNECT_THREATHOLD(
        Utils::get_json(config, "seedDisconnectThreathold", ROUTING_SEED_DISCONNECT_THREATHOLD)),
    CONFIG_SEED_INFO_KEEP_THREATHOLD(
        Utils::get_json(config, "seedInfoKeepThreathold", ROUTING_SEED_INFO_KEEP_THREATHOLD)),
    CONFIG_SEED_INFO_NIDS_COUNT(Utils::get_json(config, "seedInfoNidsCount", ROUTING_SEED_INFO_NIDS_COUNT)),
    CONFIG_SEED_NEXT_POSITION(Utils::get_json(config, "seedNextPosition", ROUTING_SEED_NEXT_POSITION)),
    CONFIG_UPDATE_PERIOD(Utils::get_json(config, "updatePeriod", ROUTING_UPDATE_PERIOD)),

    delegate(routing_delegate),
    routing_1d(nullptr),
    routing_2d(nullptr),
    node_state(LinkState::OFFLINE),
    seed_state(LinkState::OFFLINE),
    routing_countdown(CONFIG_FORCE_UPDATE_COUNT),
    seed_online_timestamp(0) {
  routing_1d = new Routing1D(param, *this);
  algorithms.push_back(std::unique_ptr<RoutingAlgorithm>(routing_1d));

  if (coord_system) {
    routing_2d = new Routing2D(param, *this, *coord_system, CONFIG_UPDATE_PERIOD);
    algorithms.push_back(std::unique_ptr<RoutingAlgorithm>(routing_2d));
  }

  // add task
  scheduler.add_controller_loop(this, std::bind(&Routing::update, this), CONFIG_UPDATE_PERIOD);
}

Routing::~Routing() {
  scheduler.remove_task(this);
}

const NodeID& Routing::get_relay_nid_1d(const Packet& packet) {
  return routing_1d->get_relay_nid(packet);
}

bool Routing::is_covered_range_1d(const NodeID& nid) {
  return routing_1d->is_covered_range(nid);
}

const NodeID& Routing::get_relay_nid_2d(const Coordinate& dst) {
  assert(routing_2d != nullptr);
  return routing_2d->get_relay_nid(dst);
}

const NodeID& Routing::get_route_to_seed() {
  return next_to_seed;
}

bool Routing::is_direct_connect(const NodeID& nid) {
  return online_links.find(nid) != online_links.end();
}

void Routing::on_change_local_position(const Coordinate& position) {
  for (auto& algorithm : algorithms) {
    algorithm->on_change_local_position(position);
  }
}

/**
 * @param nids A set of links those are online.
 */
void Routing::on_change_online_links(const std::set<NodeID>& nids) {
  assert(nids != online_links);

  online_links            = nids;
  has_update_online_links = true;

  update_seed_route_by_links();
}

void Routing::on_recv_packet(const NodeID& nid, const Packet& packet) {
  for (auto& algorithm : algorithms) {
    algorithm->on_recv_packet(nid, packet);
  }
}

void Routing::algorithm_1d_on_change_nearby(
    RoutingAlgorithm& algorithm, const NodeID& prev_nid, const NodeID& next_nid) {
  delegate.routing_on_module_1d_change_nearby(*this, prev_nid, next_nid);
}

void Routing::algorithm_2d_on_change_nearby(RoutingAlgorithm& algorithm, const std::set<NodeID>& nids) {
  delegate.routing_on_module_2d_change_nearby(*this, nids);
}

void Routing::algorithm_2d_on_change_nearby_position(
    RoutingAlgorithm& algorithm, const std::map<NodeID, Coordinate>& positions) {
  delegate.routing_on_module_2d_change_nearby_position(*this, positions);
}

void Routing::module_on_change_accessor_state(LinkState::Type seed_status, LinkState::Type node_status) {
  this->seed_state = seed_status;
  this->node_state = node_status;

  update_seed_route_by_state();
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

  update_seed_route_by_info(packet->src_nid, content);

  auto it = routing_infos.find(packet->src_nid);
  if (it == routing_infos.end()) {
    const NodeID& src_nid = packet->src_nid;
    routing_infos.insert(std::make_pair(src_nid, std::make_tuple(std::move(packet), content)));
  } else {
    std::get<0>(it->second).swap(packet);
    std::get<1>(it->second) = content;
  }
}

/**
 * Send routing packet.
 */
void Routing::send_routing_info() {
  RoutingProtocol::RoutingInfo param;

  {
    auto it = distance_from_seed.find(next_to_seed);
    if (it == distance_from_seed.end()) {
      param.set_seed_distance(UINT32_MAX);

    } else {
      uint32_t distance = it->second;
      param.set_seed_distance(distance + 1);
    }
  }

  int64_t current = Utils::get_current_msec();
  std::multimap<int64_t, NodeID> seed_nids;
  for (auto& it : seed_timestamps) {
    int64_t link_duration = current - it.second;
    if (link_duration < CONFIG_SEED_DISCONNECT_THREATHOLD) {
      seed_nids.insert(std::make_pair(link_duration, it.first));
    }
  }
  unsigned int idx = 0;
  for (auto& it : seed_nids) {
    if (idx >= CONFIG_SEED_INFO_NIDS_COUNT) {
      break;
    }
    colonio::RoutingProtocol::SeedInfoOne* seed_info = param.add_seed_infos();
    it.second.to_pb(seed_info->mutable_nid());
    seed_info->set_duration(it.first);
    idx++;
  }

  for (auto& algorithm : algorithms) {
    algorithm->send_routing_info(&param);
  }

  send_packet(NodeID::NEXT, PacketMode::EXPLICIT, CommandID::Routing::ROUTING, serialize_pb(param));
}

void Routing::update() {
  update_seed_connection();

  if (node_state == LinkState::ONLINE) {
    for (auto& row : routing_infos) {
      std::get<0>(row.second)->parse_content(&std::get<1>(row.second));
    }

    for (auto& algorithm : algorithms) {
      if (algorithm->update_routing_info(online_links, has_update_online_links, routing_infos)) {
        logi("force routing");
        routing_countdown = 0;
      }
    }

    update_node_connection();

    if (routing_countdown <= 0) {
      send_routing_info();
      routing_countdown = CONFIG_FORCE_UPDATE_COUNT;
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

void Routing::update_seed_connection() {
  int64_t current = Utils::get_current_msec();

  if (seed_state == LinkState::ONLINE) {
    if (seed_online_timestamp == 0) {
      seed_online_timestamp = Utils::get_current_msec();
      logi("force routing");
      routing_countdown          = 0;
      seed_timestamps[local_nid] = current;
    }

    distance_from_seed[local_nid] = 1;

    int64_t online_duration = current - seed_online_timestamp;
    if (online_duration > CONFIG_SEED_DISCONNECT_THREATHOLD && node_state == LinkState::ONLINE) {
      delegate.routing_do_disconnect_seed(*this);
    }

  } else if (seed_state == LinkState::OFFLINE) {
    if (seed_online_timestamp != 0) {
      seed_online_timestamp = 0;
      logi("force routing");
      routing_countdown = 0;
    }
    distance_from_seed.erase(local_nid);

    if (node_state != LinkState::ONLINE) {
      delegate.routing_do_connect_seed(*this);
    }

    int count            = 0;
    int64_t min_duration = INT64_MAX;
    NodeID min_nid;
    for (auto& it : seed_timestamps) {
      int64_t duration = current - it.second;
      if (duration < CONFIG_SEED_DISCONNECT_THREATHOLD) {
        count++;
      }
      if (duration < min_duration) {
        min_duration = duration;
        min_nid      = it.first;
      }
    }
    if (count == 0) {
      if (random.generate_u32() % CONFIG_SEED_CONNECT_RATE == 0) {
        delegate.routing_do_connect_seed(*this);
      }
      return;
    }
    NodeID target_nid = min_nid + NodeID::NID_MAX * CONFIG_SEED_NEXT_POSITION;
    if (min_duration > CONFIG_SEED_CONNECT_INTERVAL && is_covered_range_1d(target_nid)) {
      delegate.routing_do_connect_seed(*this);
      return;
    }
  }
}

void Routing::update_seed_route_by_info(const NodeID& src_nid, const RoutingProtocol::RoutingInfo& info) {
  const int64_t CURRENT       = Utils::get_current_msec();
  uint32_t distance           = info.seed_distance();
  distance_from_seed[src_nid] = distance;

  if (distance_from_seed.find(next_to_seed) == distance_from_seed.end() ||
      distance < distance_from_seed.at(next_to_seed)) {
    next_to_seed = src_nid;
    logi("force routing");
    routing_countdown = 0;
  }

  auto it = seed_timestamps.begin();
  while (it != seed_timestamps.end()) {
    if (CURRENT - it->second >= CONFIG_SEED_INFO_KEEP_THREATHOLD) {
      it = seed_timestamps.erase(it);

    } else {
      it++;
    }
  }

  for (auto& seed_info : info.seed_infos()) {
    NodeID nid       = NodeID::from_pb(seed_info.nid());
    int64_t duration = seed_info.duration();
    auto it          = seed_timestamps.find(nid);
    if (it == seed_timestamps.end()) {
      seed_timestamps.insert(std::make_pair(nid, CURRENT - duration));

    } else if (CURRENT - duration < it->second) {
      it->second = CURRENT - duration;
    }
  }
}

void Routing::update_seed_route_by_links() {
  auto it = distance_from_seed.begin();
  while (it != distance_from_seed.end()) {
    const NodeID& nid = it->first;
    if (nid != NodeID::NONE && online_links.find(nid) == online_links.end()) {
      it = distance_from_seed.erase(it);

    } else {
      it++;
    }
  }

  if (distance_from_seed.find(next_to_seed) == distance_from_seed.end()) {
    uint32_t min = UINT32_MAX;
    next_to_seed = NodeID::NONE;

    for (auto& it : distance_from_seed) {
      if (it.second < min) {
        min          = it.second;
        next_to_seed = it.first;
      }
    }
    logi("force routing");
    routing_countdown = 0;
  }
}

void Routing::update_seed_route_by_state() {
  if (seed_state == LinkState::ONLINE) {
    if (next_to_seed != local_nid) {
      next_to_seed                     = local_nid;
      distance_from_seed[next_to_seed] = 1;
      logi("force routing");
      routing_countdown = 0;
    }

  } else {
    if (next_to_seed == local_nid) {
      distance_from_seed.erase(local_nid);
      update_seed_route_by_links();
    }
  }
}
}  // namespace colonio
