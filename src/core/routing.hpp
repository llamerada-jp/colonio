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

#include <map>
#include <memory>
#include <set>
#include <tuple>

#include "coordinate.hpp"
#include "definition.hpp"
#include "node_id.hpp"

namespace colonio {
namespace proto {
class Routing;
}  // namespace proto

class CommandManager;
class CoordSystem;
class Logger;
class Packet;
class Routing;
class Routing1D;
class Routing2D;
class Scheduler;

class RoutingDelegate {
 public:
  virtual ~RoutingDelegate();
  virtual void routing_do_connect_node(const NodeID& nid)                                                 = 0;
  virtual void routing_do_disconnect_node(const NodeID& nid)                                              = 0;
  virtual void routing_do_connect_seed()                                                                  = 0;
  virtual void routing_do_disconnect_seed()                                                               = 0;
  virtual void routing_on_module_1d_change_nearby(const NodeID& prev_nid, const NodeID& next_nid)         = 0;
  virtual void routing_on_module_2d_change_nearby(const std::set<NodeID>& nids)                           = 0;
  virtual void routing_on_module_2d_change_nearby_position(const std::map<NodeID, Coordinate>& positions) = 0;
};

class RoutingAlgorithm {
 public:
  const std::string name;

  RoutingAlgorithm(const std::string& name_);
  virtual ~RoutingAlgorithm();
  virtual const std::set<NodeID>& get_required_nodes()                 = 0;
  virtual void on_change_local_position(const Coordinate& position)    = 0;
  virtual void on_recv_packet(const NodeID& nid, const Packet& packet) = 0;
  virtual void send_routing_info(proto::Routing* param)                = 0;
  virtual bool update_routing_info(
      const std::set<NodeID>& online_links, bool has_update_ol,
      const std::map<NodeID, proto::Routing>& routing_infos) = 0;
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

class Routing : public RoutingAlgorithm1DDelegate, public RoutingAlgorithm2DDelegate {
 public:
  Routing(
      Logger& l, Random& r, Scheduler& s, CommandManager& c, const NodeID& n, RoutingDelegate& d,
      const CoordSystem* coord_system, const picojson::object& config);
  virtual ~Routing();

  const NodeID& get_relay_nid_1d(const Packet& packet);
  bool is_covered_range_1d(const NodeID& nid);

  const NodeID& get_relay_nid_2d(const Coordinate& dst);
  bool is_covered_range_2d(const Coordinate& position);

  const NodeID& get_route_to_seed();
  bool is_direct_connect(const NodeID& nid);
  void on_change_local_position(const Coordinate& position);
  void on_change_network_state(LinkState::Type seed_state, LinkState::Type node_state);
  void on_change_online_links(const std::set<NodeID>& nids);
  void on_recv_packet(const NodeID& nid, const Packet& packet);

 private:
  const unsigned int CONFIG_FORCE_UPDATE_COUNT;
  const unsigned int CONFIG_SEED_CONNECT_INTERVAL;
  const unsigned int CONFIG_SEED_CONNECT_RATE;

  const unsigned int CONFIG_SEED_DISCONNECT_THRESHOLD;
  const unsigned int CONFIG_SEED_INFO_KEEP_THRESHOLD;
  const unsigned int CONFIG_SEED_INFO_NIDS_COUNT;
  const double CONFIG_SEED_NEXT_POSITION;
  const unsigned int CONFIG_UPDATE_PERIOD;

  Logger& logger;
  Random& random;
  Scheduler& scheduler;
  CommandManager& command_manager;
  const NodeID& local_nid;

  RoutingDelegate& delegate;

  std::vector<std::unique_ptr<RoutingAlgorithm>> algorithms;
  Routing1D* routing_1d;
  Routing2D* routing_2d;

  LinkState::Type node_state;
  LinkState::Type seed_state;

  std::set<NodeID> online_links;
  bool has_update_online_links;

  // map of source node-id and Routing packet content
  std::map<NodeID, proto::Routing> routing_infos;
  int64_t routing_countdown;

  // next, distance
  std::map<NodeID, uint32_t> distance_from_seed;
  std::map<NodeID, int64_t> seed_timestamps;
  NodeID next_to_seed;
  int64_t seed_online_timestamp;

  void algorithm_1d_on_change_nearby(
      RoutingAlgorithm& algorithm, const NodeID& prev_nid, const NodeID& next_nid) override;

  void algorithm_2d_on_change_nearby(RoutingAlgorithm& algorithm, const std::set<NodeID>& nids) override;
  void algorithm_2d_on_change_nearby_position(
      RoutingAlgorithm& algorithm, const std::map<NodeID, Coordinate>& positions) override;

  void recv_routing(const Packet& packet);
  void send_routing();
  void update();
  void update_node_connection();
  void update_seed_connection();
  void update_seed_route_by_info(const NodeID& src_nid, const proto::Routing& info);
  void update_seed_route_by_links();
  void update_seed_route_by_state();
};
}  // namespace colonio
