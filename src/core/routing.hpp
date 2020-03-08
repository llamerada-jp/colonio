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

#include <list>
#include <mutex>
#include <set>

#include "module_base.hpp"
#include "coordinate.hpp"
#include "node_id.hpp"
#include "routing_protocol.pb.h"

namespace colonio {
class Context;
class Packet;
class Routing;
class Routing1D;
class Routing2D;

class RoutingDelegate {
 public:
  virtual ~RoutingDelegate();
  virtual void routing_do_connect_node(Routing& routing, const NodeID& nid)                                         = 0;
  virtual void routing_do_disconnect_node(Routing& routing, const NodeID& nid)                                      = 0;
  virtual void routing_do_connect_seed(Routing& route)                                                              = 0;
  virtual void routing_do_disconnect_seed(Routing& route)                                                           = 0;
  virtual void routing_on_module_1d_change_nearby(Routing& routing, const NodeID& prev_nid, const NodeID& next_nid) = 0;
  virtual void routing_on_module_2d_change_nearby(Routing& routing, const std::set<NodeID>& nids)                   = 0;
  virtual void routing_on_module_2d_change_nearby_position(
      Routing& routing, const std::map<NodeID, Coordinate>& positions) = 0;
};

class RoutingAlgorithm {
 public:
  const std::string name;

  RoutingAlgorithm(const std::string& name_);
  virtual ~RoutingAlgorithm();
  virtual const std::set<NodeID>& get_required_nodes()                 = 0;
  virtual void on_change_my_position(const Coordinate& position)       = 0;
  virtual void on_recv_packet(const NodeID& nid, const Packet& packet) = 0;
  virtual void send_routing_info(RoutingProtocol::RoutingInfo* param)  = 0;
  virtual bool update_routing_info(
      const std::set<NodeID>& online_links, bool has_update_ol,
      const std::map<NodeID, std::tuple<std::unique_ptr<const Packet>, RoutingProtocol::RoutingInfo>>&
          routing_infos) = 0;
};

class RoutingAlgorithm1DDelegate {
 public:
  virtual ~RoutingAlgorithm1DDelegate();
  virtual void algorithm_1d_on_change_nearby(
      RoutingAlgorithm& algorithm, const NodeID& prev_nid, const NodeID& next_nid) = 0;
};

class RoutingAlgorithm2DDelegate {
 public:
  virtual ~RoutingAlgorithm2DDelegate();
  virtual void algorithm_2d_on_change_nearby(RoutingAlgorithm& algorithm, const std::set<NodeID>& nids) = 0;
  virtual void algorithm_2d_on_change_nearby_position(
      RoutingAlgorithm& algorithm, const std::map<NodeID, Coordinate>& positions) = 0;
};

class Routing : public ModuleBase, public RoutingAlgorithm1DDelegate, public RoutingAlgorithm2DDelegate {
 public:
  Routing(
      Context& context, ModuleDelegate& module_delegate, RoutingDelegate& routing_delegate, APIChannel::Type channel,
      const picojson::object& config);
  virtual ~Routing();

  const NodeID& get_relay_nid_1d(const Packet& packet);
  bool is_covered_range_1d(const NodeID& nid);

  const NodeID& get_relay_nid_2d(const Coordinate& dest);
  bool is_covered_range_2d(const Coordinate& position);

  // next, seed, steps
  std::tuple<const NodeID&, const NodeID&, uint32_t> get_route_to_seed();
  bool is_direct_connect(const NodeID& nid);
  void on_change_my_position(const Coordinate& position);
  void on_change_online_links(const std::set<NodeID>& nids);
  void on_recv_packet(const NodeID& nid, const Packet& packet);

 private:
  const unsigned int CONFIG_UPDATE_PERIOD;
  const unsigned int CONFIG_FORCE_UPDATE_TIMES;
  RoutingDelegate& delegate;

  std::vector<std::unique_ptr<RoutingAlgorithm>> algorithms;
  Routing1D* routing_1d;
  Routing2D* routing_2d;

  LinkStatus::Type node_status;
  LinkStatus::Type seed_status;

  std::set<NodeID> online_links;
  bool has_update_online_links;

  std::map<NodeID, std::tuple<std::unique_ptr<const Packet>, RoutingProtocol::RoutingInfo>> routing_infos;

  // next, seed, distance
  std::map<NodeID, std::pair<NodeID, uint32_t>> dists_from_seed;
  NodeID next_to_seed;
  const int64_t RANDOM_WAIT_SEED_CONNECTION;
  int64_t passed_seed_connection;
  int64_t routing_countdown;

  void algorithm_1d_on_change_nearby(
      RoutingAlgorithm& algorithm, const NodeID& prev_nid, const NodeID& next_nid) override;

  void algorithm_2d_on_change_nearby(RoutingAlgorithm& algorithm, const std::set<NodeID>& nids) override;
  void algorithm_2d_on_change_nearby_position(
      RoutingAlgorithm& algorithm, const std::map<NodeID, Coordinate>& positions) override;

  void module_on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status) override;
  void module_process_command(std::unique_ptr<const Packet> packet) override;

  void recv_routing_info(std::unique_ptr<const Packet> packet);
  void send_routing_info();
  void update();
  void update_node_connection();
  void update_route_to_seed();
  void update_seed_connection();
};
}  // namespace colonio
