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
#pragma once

#include "routing.hpp"

namespace colonio {
class CoordSystem;

class Routing2D : public RoutingAlgorithm {
 public:
  Routing2D(ModuleParam& param, RoutingAlgorithm2DDelegate& delegate_, const CoordSystem& coord_system_);

  const std::set<NodeID>& get_required_nodes() override;
  void on_change_local_position(const Coordinate& position) override;
  void on_recv_packet(const NodeID& nid, const Packet& packet) override;
  void send_routing_info(RoutingProtocol::RoutingInfo* param) override;
  bool update_routing_info(
      const std::set<NodeID>& online_links, bool has_update_ol,
      const std::map<NodeID, std::tuple<std::unique_ptr<const Packet>, RoutingProtocol::RoutingInfo>>& routing_infos)
      override;

  const NodeID& get_relay_nid(const Coordinate& position);

 private:
  struct RoutingInfo {
    int64_t timestamp;
    Coordinate position;
    std::map<NodeID, Coordinate> nodes;
  };

  Logger& logger;
  const NodeID& local_nid;
  RoutingAlgorithm2DDelegate& delegate;
  const CoordSystem& coord_system;

  std::set<NodeID> connected_nodes;
  std::set<NodeID> nearby_nids;
  std::map<NodeID, Coordinate> nearby_nodes;
  std::map<NodeID, Coordinate> known_nodes;
  std::map<NodeID, RoutingInfo> routing_info_cache;

  void check_duplicate_point(const std::vector<NodeID>& nids, std::vector<double>* x_vec, std::vector<double>* y_vec);
  void shift_duplicate_point(const NodeID& nid, double* x, double* y);
  void update_node_infos();
};
}  // namespace colonio
