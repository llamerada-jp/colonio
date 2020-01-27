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
#include "routing_1d.hpp"

#include <cassert>

#include "context.hpp"
#include "convert.hpp"
#include "utils.hpp"

namespace colonio {

static const int LEVELS                 = 8;
static const NodeID LEVEL_RANGE[LEVELS] = {NodeID::RANGE_0, NodeID::RANGE_1, NodeID::RANGE_2, NodeID::RANGE_3,
                                           NodeID::RANGE_4, NodeID::RANGE_5, NodeID::RANGE_6, NodeID::RANGE_7};

Routing1D::ConnectedNode::ConnectedNode(int level_) :
    connected_time(Utils::get_current_msec()),
    level(level_),
    odd_score(0),
    raw_score(0) {
}

Routing1D::RouteInfo::RouteInfo(const NodeID& root_nid_, int level_) :
    root_nid(root_nid_),
    level(level_),
    raw_score(0) {
}

Routing1D::Routing1D(Context& context_, RoutingAlgorithm1DDelegate& delegate_) :
    RoutingAlgorithm("1D"),
    context(context_),
    delegate(delegate_) {
}

const std::set<NodeID>& Routing1D::get_required_nodes() {
  return required_nodes;
}

void Routing1D::on_change_my_position(const Coordinate& position) {
  // Do nothing.
}

bool Routing1D::on_change_online_links(const std::set<NodeID>& nids) {
  bool is_changed = false;
  // Add some nid to nid_map if nid exist in nids and do not exist in nid_map.
  for (auto& nid : nids) {
    if (connected_nodes.find(nid) == connected_nodes.end()) {
      is_changed = true;
      connected_nodes.insert(std::make_pair(nid, ConnectedNode(get_level(nid))));
    }
  }
  // Remove some nid from nid_map if nid exist int nid_map and do not exist in nids.
  auto it = connected_nodes.begin();
  while (it != connected_nodes.end()) {
    const NodeID& nid = it->first;
    if (nids.find(nid) == nids.end()) {
      is_changed = true;
      it         = connected_nodes.erase(it);
    } else {
      it++;
    }
  }

  assert(nids.size() == connected_nodes.size());
  return is_changed;
}

void Routing1D::on_recv_packet(const NodeID& nid, const Packet& packet) {
  auto find = connected_nodes.find(nid);
  if (find != connected_nodes.end()) {
    ConnectedNode& cn = find->second;
    cn.raw_score++;
  }

  RouteInfo* ri = std::get<1>(get_nearest_info(packet.src_nid));
  if (ri != nullptr) {
    ri->raw_score++;
  }
}

bool Routing1D::on_recv_routing_info(const Packet& packet, const RoutingProtocol::RoutingInfo& routing_info) {
  // Ignore routing packet when source node has disconnected.
  // assert(connected_nodes.find(packet.src_nid) != connected_nodes.end());
  if (connected_nodes.find(packet.src_nid) == connected_nodes.end()) {
    printf("disconnected:%s\n", packet.src_nid.to_str().c_str());
    return false;
  }

  ConnectedNode& cn = connected_nodes.at(packet.src_nid);

  std::set<NodeID> nexts;
  int odd_score = 0;

  for (auto& it : routing_info.nodes()) {
    NodeID nid = NodeID::from_str(it.first);
    nexts.insert(nid);
    if (nid == context.my_nid) {
      odd_score = it.second.r1d_score();
    }
  }

  cn.odd_score = odd_score;

  if (cn.nexts != nexts) {
    cn.nexts = nexts;
    return true;

  } else {
    return false;
  }
}

void Routing1D::send_routing_info(RoutingProtocol::RoutingInfo* param) {
  normalize_score();

  for (auto& it : connected_nodes) {
    std::string nid_str = it.first.to_str();
    (*param->mutable_nodes())[nid_str].set_r1d_score(it.second.raw_score);
  }
}

bool Routing1D::update_routing_info(
    const std::set<NodeID>& online_links, bool has_update_ol,
    const std::map<NodeID, std::tuple<std::unique_ptr<const Packet>, RoutingProtocol::RoutingInfo>>& routing_infos) {
  bool is_changed = false;

  if (has_update_ol) {
    is_changed = is_changed || on_change_online_links(online_links);
  }
  for (auto& it : routing_infos) {
    is_changed = is_changed || on_recv_routing_info(*std::get<0>(it.second), std::get<1>(it.second));
  }

  if (is_changed) {
    update_route_infos();
  }
  update_required_nodes();

  return has_update_ol;
}

/**
 * Get node-id for relaying packet to send a packet to target node.
 * @retrn THIS for local node, NORMAL node-id for another node, NONE if target node has not exist.
 */
const NodeID& Routing1D::get_relay_nid(const Packet& packet) {
  bool is_explicit = packet.mode & PacketMode::EXPLICIT;

  if (packet.dst_nid == context.my_nid || packet.dst_nid == NodeID::THIS) {
    return NodeID::THIS;
  }

  if (packet.dst_nid == NodeID::NEXT) {
    return NodeID::NEXT;
  }

  if (prev_nid == NodeID::NONE) {
    if (is_explicit) {
      return NodeID::NONE;
    } else {
      return NodeID::THIS;
    }

  } else {
    if (packet.dst_nid.is_between(range_min_nid, range_max_nid)) {
      if (is_explicit) {
        return NodeID::NONE;
      } else {
        return NodeID::THIS;
      }

    } else {
      RouteInfo* nearest_info;
      const NodeID* nearest_nid;
      std::tie(nearest_nid, nearest_info) = get_nearest_info(packet.dst_nid);

      assert(nearest_info != nullptr);
      nearest_info->raw_score++;

      NodeID nearest_dist = context.my_nid.distance_from(packet.dst_nid);
      ConnectedNode* cn   = nullptr;

      for (auto& it : connected_nodes) {
        const NodeID& it_nid = it.first;
        NodeID dist          = it_nid.distance_from(packet.dst_nid);
        if (dist < nearest_dist) {
          nearest_dist = dist;
          cn           = &it.second;
          nearest_nid  = &it.first;
        }
      }

      if (cn != nullptr) {
        connected_nodes.at(*nearest_nid).raw_score++;
      }

      return nearest_info->root_nid;
    }
  }
}

bool Routing1D::is_coverd_range(const NodeID& nid) {
  if (range_min_nid == NodeID::NONE) {
    return true;
  } else {
    return nid.is_between(range_min_nid, range_max_nid);
  }
}

bool Routing1D::is_orphan(unsigned int nodes_count) {
  std::set<NodeID> nids;
  for (auto& it : connected_nodes) {
    for (auto& nid : it.second.nexts) {
      nids.insert(nid);
    }
  }

  if (nids.size() < ORPHAN_NODES_MAX && nids.size() < nodes_count / 2) {
    return true;

  } else {
    return false;
  }
}

int Routing1D::get_level(const NodeID& nid) {
  NodeID sub = nid - context.my_nid;

  for (int i = 0; i < sizeof(LEVEL_RANGE) / sizeof(LEVEL_RANGE[0]); i++) {
    if (sub < LEVEL_RANGE[i]) {
      return i;
    }
  }

  return -1;
}

std::tuple<NodeID, NodeID> Routing1D::get_nearby_nid(const NodeID& nid, const std::set<NodeID>& nids) {
  NodeID p_nid = NodeID::NONE;
  NodeID n_nid = NodeID::NONE;

  auto find = nids.lower_bound(nid);
  if (find != nids.end() && *find == nid) {
    if (nids.size() == 1) {
      // set NONE

    } else if (find == nids.begin()) {
      p_nid = *nids.rbegin();
      n_nid = *std::next(find);

    } else if (std::next(find) == nids.end()) {
      p_nid = *std::prev(find);
      n_nid = *nids.begin();

    } else {
      p_nid = *std::prev(find);
      n_nid = *std::next(find);
    }

  } else {
    if (nids.size() == 0) {
      // set NONE

    } else if (find == nids.begin()) {
      p_nid = *nids.rbegin();
      n_nid = *find;

    } else if (find == nids.end()) {
      p_nid = *std::prev(find);
      n_nid = *nids.begin();

    } else {
      p_nid = *std::prev(find);
      n_nid = *find;
    }
  }

  return std::make_tuple(p_nid, n_nid);
}

std::tuple<const NodeID*, Routing1D::RouteInfo*> Routing1D::get_nearest_info(const NodeID& nid) {
  NodeID nearest_dist       = context.my_nid.distance_from(nid);
  const NodeID* nearest_nid = nullptr;
  RouteInfo* nearest_info   = nullptr;

  for (auto& it : route_infos) {
    const NodeID& it_nid = it.first;
    NodeID dist          = it_nid.distance_from(nid);
    if (dist < nearest_dist) {
      nearest_dist = dist;
      nearest_nid  = &it_nid;
      nearest_info = &it.second;
    }
  }

  return std::make_tuple(nearest_nid, nearest_info);
}

void Routing1D::normalize_score() {
  int sum = 0;
  for (auto& it : connected_nodes) {
    sum += it.second.raw_score;
  }

  int rate = (sum < 1024 * 1024 ? 1 : 1024 * 1024 / sum);
  for (auto& it : connected_nodes) {
    it.second.raw_score *= rate;
  }

  sum = 0;
  for (auto& it : route_infos) {
    sum += it.second.raw_score;
  }

  rate = (sum < 1024 * 1024 ? 1 : 1024 * 1024 / sum);
  for (auto& it : route_infos) {
    it.second.raw_score *= rate;
  }
}

void Routing1D::update_required_nodes() {
  required_nodes.clear();

  if (prev_nid != NodeID::NONE) {
    required_nodes.insert(prev_nid);
    required_nodes.insert(next_nid);
  }

  int64_t current_msec = Utils::get_current_msec();
  normalize_score();

  std::set<NodeID> next_nids;
  NodeID now_prev_nid;
  NodeID now_next_nid;
  {
    std::set<NodeID> connected_nids;
    for (auto& it : connected_nodes) {
      connected_nids.insert(it.first);
    }
    std::tie(now_prev_nid, now_next_nid) = get_nearby_nid(context.my_nid, connected_nids);
    next_nids.insert(now_prev_nid);
    next_nids.insert(now_next_nid);
  }

  for (auto& it : connected_nodes) {
    const NodeID& root = it.first;
    ConnectedNode& cn  = it.second;

    if (cn.nexts.size() < LINKS_MIN || cn.connected_time + LINK_TRIAL_TIME_MIN > current_msec) {
      required_nodes.insert(root);

    } else {
      NodeID prev;
      NodeID next;
      std::set<NodeID> nids;
      for (auto& nid : cn.nexts) {
        nids.insert(nid);
      }
      nids.insert(context.my_nid);
      std::tie(prev, next) = get_nearby_nid(root, nids);
      if (prev == context.my_nid || next == context.my_nid) {
        required_nodes.insert(root);
      }
    }
  }

  std::list<NodeID> connected_nids[LEVELS];
  std::vector<NodeID> route_nids[LEVELS];

  for (auto it : connected_nodes) {
    const NodeID& nid = it.first;
    ConnectedNode& cn = it.second;

    if (cn.level != -1 && next_nids.find(nid) == next_nids.end()) {
      connected_nids[cn.level].push_back(nid);
    }
  }

  for (auto it : route_infos) {
    const NodeID& nid = it.first;
    RouteInfo& ri     = it.second;

    if (ri.level != -1 && connected_nodes.find(nid) == connected_nodes.end()) {
      route_nids[ri.level].push_back(nid);
    }
  }

  for (int level = 0; level < LEVELS; level++) {
    std::list<NodeID>& cnids   = connected_nids[level];
    std::vector<NodeID>& rnids = route_nids[level];

    cnids.sort([this](NodeID& a, NodeID& b) {
      ConnectedNode& a_cn = connected_nodes.at(a);
      ConnectedNode& b_cn = connected_nodes.at(b);
      int a_score         = a_cn.odd_score + a_cn.raw_score;
      int b_score         = b_cn.odd_score + b_cn.raw_score;

      if (a_score == b_score) {
        return a_cn.connected_time < b_cn.connected_time;

      } else {
        return a_score > b_score;
      }
    });

    bool need_connect = true;
    if (cnids.size() >= 2) {
      auto it = cnids.begin();
      for (it++; it != cnids.end(); it++) {
        const NodeID& nid = *it;
        ConnectedNode& cn = connected_nodes.at(nid);

        if (required_nodes.find(nid) != required_nodes.end()) {
          need_connect = false;
        }
      }
    }

    if (need_connect && rnids.size() > 0) {
      int idx           = context.get_rnd_32() % rnids.size();
      const NodeID& nid = rnids[idx];
      required_nodes.insert(nid);
    }
  }
#ifndef NDEBUG
  {
    picojson::array a;
    for (auto& nid : required_nodes) {
      a.push_back(nid.to_json());
    }
    context.debug_event(DebugEvent::REQUIRED1D, picojson::value(a));
  }
#endif
}

/**
 * Update route_nodes by connected_nodes.
 */
void Routing1D::update_route_infos() {
  // Make a map that is pair of node and it's root(relay node between this node) node-id.
  std::map<NodeID, NodeID> known_nids;

  for (auto& it : connected_nodes) {
    const NodeID& root_nid = it.first;
    if (known_nids.find(root_nid) == known_nids.end()) {
      known_nids.insert(std::make_pair(root_nid, root_nid));

    } else {
      known_nids.at(root_nid) = root_nid;
    }

    for (const auto& nid : it.second.nexts) {
      auto find = known_nids.find(nid);
      if (find == known_nids.end()) {
        known_nids.insert(std::make_pair(nid, root_nid));

      } else {
        if (nid.distance_from(root_nid) < nid.distance_from(find->second)) {
          known_nids.at(nid) = root_nid;
        }
      }
    }
  }

  // Find the prev and the next node.
  known_nids.insert(std::make_pair(context.my_nid, NodeID::NONE));
  if (known_nids.size() == 1) {
    prev_nid      = NodeID::NONE;
    next_nid      = NodeID::NONE;
    range_min_nid = NodeID::NID_MIN;
    range_max_nid = NodeID::NID_MAX;

  } else {
    std::set<NodeID> nids;
    for (auto& it : known_nids) {
      nids.insert(it.first);
    }
    NodeID prev_nid_bk           = prev_nid;
    NodeID next_nid_bk           = next_nid;
    std::tie(prev_nid, next_nid) = get_nearby_nid(context.my_nid, nids);
    assert(prev_nid != NodeID::NONE);
    assert(next_nid != NodeID::NONE);

    if (prev_nid_bk != prev_nid || next_nid_bk != next_nid) {
      delegate.algorithm_1d_on_change_nearby(*this, prev_nid, next_nid);
    }

    range_min_nid = NodeID::center_mod(prev_nid, context.my_nid);
    range_max_nid = NodeID::center_mod(context.my_nid, next_nid);
  }

  // Update route_nodes.
  known_nids.erase(context.my_nid);
  auto it = route_infos.begin();
  while (it != route_infos.end()) {
    if (known_nids.find(it->first) == known_nids.end()) {
      it = route_infos.erase(it);
    } else {
      it++;
    }
  }

  for (auto& it : known_nids) {
    const NodeID& known_nid = it.first;
    const NodeID& root_nid  = it.second;

    if (route_infos.find(known_nid) == route_infos.end()) {
      route_infos.insert(std::make_pair(known_nid, RouteInfo(root_nid, get_level(known_nid))));

    } else if (route_infos.at(known_nid).root_nid != root_nid) {
      route_infos.erase(known_nid);
      route_infos.insert(std::make_pair(known_nid, RouteInfo(root_nid, get_level(known_nid))));
    }
  }

#ifndef NDEBUG
  {
    picojson::array a;
    for (auto& it : known_nids) {
      a.push_back(it.first.to_json());
    }
    context.debug_event(DebugEvent::KNOWN1D, picojson::value(a));
  }
  {
    picojson::array a;
    a.push_back(next_nid.to_json());
    a.push_back(prev_nid.to_json());
    context.debug_event(DebugEvent::NEXTS, picojson::value(a));
  }
#endif
}

#ifndef NDEBUG
void Routing1D::show_debug_info() {
  std::cerr << "prev " << prev_nid.to_str() << std::endl
            << "next " << next_nid.to_str() << std::endl
            << "min  " << range_min_nid.to_str() << std::endl
            << "max  " << range_max_nid.to_str() << std::endl;
  std::cerr << "connected_nodes" << std::endl;
  for (auto& it : connected_nodes) {
    std::cerr << "  " << it.first.to_str() << std::endl;
    for (auto& next : it.second.nexts) {
      std::cerr << "    " << next.to_str() << std::endl;
    }
  }
  std::cerr << "route_infos" << std::endl;
  for (auto& it : route_infos) {
    std::cerr << "  " << it.first.to_str() << " -> " << it.second.root_nid.to_str() << std::endl;
  }
}
#endif
}  // namespace colonio
