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
#pragma once

#include "routing.hpp"

namespace colonio {
class Context;

class Routing2D : public RoutingAlgorithm {
 public:
  Routing2D(Context& context_, RoutingAlgorithm2DDelegate& delegate_);

  const std::set<NodeID>& get_required_nodes() override;
  void on_change_my_position(const Coordinate& position) override;
  void on_recv_packet(const NodeID& nid, const Packet& packet) override;
  void send_routing_info(RoutingProtocol::RoutingInfo* param) override;
  bool update_routing_info(const std::set<NodeID>& online_links, bool has_update_ol,
                           const std::map<NodeID, std::tuple<std::unique_ptr<const Packet>,
                           RoutingProtocol::RoutingInfo>>& routing_infos) override;

  bool on_change_online_links(const std::set<NodeID>& nids);
  bool on_recv_routing_info(const Packet& packet, const RoutingProtocol::RoutingInfo& routing_info);

  const NodeID& get_relay_nid(const Coordinate& position);

 private:
  Context& context;
  RoutingAlgorithm2DDelegate& delegate;

  std::set<NodeID> required_nodes;
  std::map<NodeID, Coordinate> nearby_nodes;

  class ConnectedNode {
   public:
    Coordinate position;
    std::map<NodeID, Coordinate> nexts;
  };
  std::map<NodeID, ConnectedNode> connected_nodes;

  class NodePoint {
   public:
    NodeID nid;
    Coordinate position;
    Coordinate shifted_position;

    NodePoint();
    NodePoint(const NodeID& nid_, const Coordinate& position_, const Coordinate& sposition);
    NodePoint& operator=(const NodePoint& r);
  };

  class Circle {
   public:
    Coordinate center;
    double r;
  };

  class Triangle {
   public:
    NodePoint p[3];

    Triangle(const NodePoint& p1, const NodePoint& p2, const NodePoint& p3);
    bool operator<(const Triangle& t) const;

    bool check_contain_node(const NodeID& nid) const;
    bool check_equal(const Triangle& t) const;
    Circle get_circumscribed_circle(Context& context);
  };

  void delaunay_add_tmp_triangle(std::map<Triangle, bool>& tmp_triangles, const Triangle& triangle);
  void delaunay_check_duplicate_point(std::map<NodeID, NodePoint>& nodes);
  Triangle delaunay_get_huge_triangle();
  void delaunay_make_nodes(std::map<NodeID, NodePoint>& nodes);
  void delaunay_shift_duplicate_point(NodePoint& np);
  void update_required_nodes();
  
};
}  // namespace colonio
