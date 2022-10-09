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

#include "colonio.pb.h"
#include "command_manager.hpp"
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
    Logger& l, Random& r, Scheduler& s, CommandManager& c, const NodeID& n, RoutingDelegate& d,
    const CoordSystem* coord_system, const picojson::object& config) :
    CONFIG_FORCE_UPDATE_COUNT(Utils::get_json(config, "forceUpdateCount", ROUTING_FORCE_UPDATE_COUNT)),
    CONFIG_SEED_CONNECT_INTERVAL(Utils::get_json(config, "seedConnectInterval", ROUTING_SEED_CONNECT_INTERVAL)),
    CONFIG_SEED_CONNECT_RATE(Utils::get_json(config, "seedConnectRate", ROUTING_SEED_CONNECT_RATE)),
    CONFIG_SEED_DISCONNECT_THRESHOLD(
        Utils::get_json(config, "seedDisconnectThreshold", ROUTING_SEED_DISCONNECT_THRESHOLD)),
    CONFIG_SEED_INFO_KEEP_THRESHOLD(Utils::get_json(config, "seedInfoKeepThreshold", ROUTING_SEED_INFO_KEEP_THRESHOLD)),
    CONFIG_SEED_INFO_NIDS_COUNT(Utils::get_json(config, "seedInfoNidsCount", ROUTING_SEED_INFO_NIDS_COUNT)),
    CONFIG_SEED_NEXT_POSITION(Utils::get_json(config, "seedNextPosition", ROUTING_SEED_NEXT_POSITION)),
    CONFIG_UPDATE_PERIOD(Utils::get_json(config, "updatePeriod", ROUTING_UPDATE_PERIOD)),

    logger(l),
    random(r),
    scheduler(s),
    command_manager(c),
    local_nid(n),
    delegate(d),
    routing_1d(nullptr),
    routing_2d(nullptr),
    node_state(LinkState::OFFLINE),
    seed_state(LinkState::OFFLINE),
    routing_countdown(CONFIG_FORCE_UPDATE_COUNT),
    seed_online_timestamp(0) {
  routing_1d = new Routing1D(logger, random, local_nid, *this);
  algorithms.push_back(std::unique_ptr<RoutingAlgorithm>(routing_1d));

  if (coord_system) {
    routing_2d = new Routing2D(logger, local_nid, *this, *coord_system, CONFIG_UPDATE_PERIOD);
    algorithms.push_back(std::unique_ptr<RoutingAlgorithm>(routing_2d));
  }

  command_manager.set_handler(
      proto::PacketContent::kRouting, std::bind(&Routing::recv_routing, this, std::placeholders::_1));

  // add task
  scheduler.repeat_task(this, std::bind(&Routing::update, this), CONFIG_UPDATE_PERIOD);
}

Routing::~Routing() {
  scheduler.remove(this);
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

void Routing::on_change_network_state(LinkState::Type seed_state, LinkState::Type node_state) {
  this->seed_state = seed_state;
  this->node_state = node_state;

  update_seed_route_by_state();
}

void Routing::on_change_online_links(const std::set<NodeID>& nids) {
  if (nids == online_links) {
    return;
  }

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
  delegate.routing_on_module_1d_change_nearby(prev_nid, next_nid);
}

void Routing::algorithm_2d_on_change_nearby(RoutingAlgorithm& algorithm, const std::set<NodeID>& nids) {
  delegate.routing_on_module_2d_change_nearby(nids);
}

void Routing::algorithm_2d_on_change_nearby_position(
    RoutingAlgorithm& algorithm, const std::map<NodeID, Coordinate>& positions) {
  delegate.routing_on_module_2d_change_nearby_position(positions);
}

void Routing::recv_routing(const Packet& packet) {
  const proto::Routing& content = packet.content->as_proto().routing();

  update_seed_route_by_info(packet.src_nid, content);

  routing_infos[packet.src_nid] = content;
}

/**
 * Send routing packet.
 */
void Routing::send_routing() {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::Routing& param                         = *content->mutable_routing();

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
    if (link_duration < CONFIG_SEED_DISCONNECT_THRESHOLD) {
      seed_nids.insert(std::make_pair(link_duration, it.first));
    }
  }
  unsigned int idx = 0;
  for (auto& it : seed_nids) {
    if (idx >= CONFIG_SEED_INFO_NIDS_COUNT) {
      break;
    }
    proto::RoutingSeedRecord* seed_record = param.add_seed_records();
    it.second.to_pb(seed_record->mutable_nid());
    seed_record->set_duration(it.first);
    idx++;
  }

  for (auto& algorithm : algorithms) {
    algorithm->send_routing_info(&param);
  }

  command_manager.send_packet_one_way(NodeID::NEXT, PacketMode::EXPLICIT, std::move(content));
}

void Routing::update() {
  update_seed_connection();

  if (node_state == LinkState::ONLINE) {
    for (auto& algorithm : algorithms) {
      if (algorithm->update_routing_info(online_links, has_update_online_links, routing_infos)) {
        log_info("force routing");
        routing_countdown = 0;
      }
    }

    update_node_connection();

    if (routing_countdown <= 0) {
      send_routing();
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
      delegate.routing_do_connect_node(nid);
    }
  }

  for (auto& nid : online_links) {
    if (required_nids.find(nid) == required_nids.end()) {
      delegate.routing_do_disconnect_node(nid);
    }
  }
}

void Routing::update_seed_connection() {
  int64_t current = Utils::get_current_msec();

  if (seed_state == LinkState::ONLINE) {
    if (seed_online_timestamp == 0) {
      seed_online_timestamp = Utils::get_current_msec();
      log_info("force routing");
      routing_countdown          = 0;
      seed_timestamps[local_nid] = current;
    }

    distance_from_seed[local_nid] = 1;

    int64_t online_duration = current - seed_online_timestamp;
    if (online_duration > CONFIG_SEED_DISCONNECT_THRESHOLD && node_state == LinkState::ONLINE) {
      delegate.routing_do_disconnect_seed();
    }

  } else if (seed_state == LinkState::OFFLINE) {
    if (seed_online_timestamp != 0) {
      seed_online_timestamp = 0;
      log_info("force routing");
      routing_countdown = 0;
    }
    distance_from_seed.erase(local_nid);

    if (node_state != LinkState::ONLINE) {
      delegate.routing_do_connect_seed();
    }

    int count            = 0;
    int64_t min_duration = INT64_MAX;
    NodeID min_nid;
    for (auto& it : seed_timestamps) {
      int64_t duration = current - it.second;
      if (duration < CONFIG_SEED_DISCONNECT_THRESHOLD) {
        count++;
      }
      if (duration < min_duration) {
        min_duration = duration;
        min_nid      = it.first;
      }
    }
    if (count == 0) {
      if (random.generate_u32() % CONFIG_SEED_CONNECT_RATE == 0) {
        delegate.routing_do_connect_seed();
      }
      return;
    }
    NodeID target_nid = min_nid + NodeID::NID_MAX * CONFIG_SEED_NEXT_POSITION;
    if (min_duration > CONFIG_SEED_CONNECT_INTERVAL && is_covered_range_1d(target_nid)) {
      delegate.routing_do_connect_seed();
      return;
    }
  }
}

void Routing::update_seed_route_by_info(const NodeID& src_nid, const proto::Routing& info) {
  const int64_t CURRENT       = Utils::get_current_msec();
  uint32_t distance           = info.seed_distance();
  distance_from_seed[src_nid] = distance;

  if (distance_from_seed.find(next_to_seed) == distance_from_seed.end() ||
      distance < distance_from_seed.at(next_to_seed)) {
    next_to_seed = src_nid;
    log_info("force routing");
    routing_countdown = 0;
  }

  auto it = seed_timestamps.begin();
  while (it != seed_timestamps.end()) {
    if (CURRENT - it->second >= CONFIG_SEED_INFO_KEEP_THRESHOLD) {
      it = seed_timestamps.erase(it);

    } else {
      it++;
    }
  }

  for (auto& seed_info : info.seed_records()) {
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
    log_info("force routing");
    routing_countdown = 0;
  }
}

void Routing::update_seed_route_by_state() {
  if (seed_state == LinkState::ONLINE) {
    if (next_to_seed != local_nid) {
      next_to_seed                     = local_nid;
      distance_from_seed[next_to_seed] = 1;
      log_info("force routing");
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
