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
#pragma once

#include <set>

#include "routing.hpp"

namespace colonio {
class Routing1D : public RoutingAlgorithm {
 public:
  Routing1D(Context& context_, RoutingAlgorithm1DDelegate& delegate_);

  // RoutingAlgorithm
  const std::set<NodeID>& get_required_nodes() override;
  void on_change_local_position(const Coordinate& position) override;
  void on_recv_packet(const NodeID& nid, const Packet& packet) override;
  void send_routing_info(RoutingProtocol::RoutingInfo* param) override;
  bool update_routing_info(
      const std::set<NodeID>& online_links, bool has_update_ol,
      const std::map<NodeID, std::tuple<std::unique_ptr<const Packet>, RoutingProtocol::RoutingInfo>>& routing_infos)
      override;

  bool on_change_online_links(const std::set<NodeID>& nids);
  bool on_recv_routing_info(const Packet& packet, const RoutingProtocol::RoutingInfo& routing_info);

  const NodeID& get_relay_nid(const Packet& packet);
  bool is_covered_range(const NodeID& nid);
  bool is_orphan(unsigned int nodes_count);

 private:
  Context& context;
  RoutingAlgorithm1DDelegate& delegate;
  /** Most nearby nodes. */
  NodeID prev_nid;
  NodeID next_nid;
  /** Node-id range of supported by this node. */
  NodeID range_min_nid;
  NodeID range_max_nid;

  std::set<NodeID> required_nodes;

  /** Map of direct connect node-id and node-ids are connected to itself. */
  class ConnectedNode {
   public:
    const int64_t connected_time;
    const int level;
    // Next node's nid.
    std::set<NodeID> next_nids;
    int odd_score;
    int raw_score;

    explicit ConnectedNode(int level_);
  };
  std::map<NodeID, ConnectedNode> connected_nodes;

  class RouteInfo {
   public:
    const NodeID root_nid;
    const int level;
    int raw_score;

    RouteInfo(const NodeID& root_nid_, int level_);
  };
  std::map<NodeID, RouteInfo> route_infos;

  int get_level(const NodeID& nid);
  std::tuple<NodeID, NodeID> get_nearby_nid(const NodeID& nid, const std::set<NodeID>& nids);
  std::tuple<const NodeID*, RouteInfo*> get_nearest_info(const NodeID& nid);
  void normalize_score();
  void update_required_nodes();
  void update_route_infos();

#ifndef NDEBUG
  void show_debug_info();
#endif
};
}  // namespace colonio
