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

#include <functional>
#include <memory>

#include "colonio/colonio.hpp"
#include "definition.hpp"
#include "node_accessor.hpp"
#include "routing.hpp"
#include "seed_accessor.hpp"

namespace colonio {
class CommandManager;
class CoordSystem;
class Logger;
class NodeID;
class Scheduler;

class NetworkDelegate {
 public:
  virtual ~NetworkDelegate();
  virtual void network_on_change_global_config(const picojson::object& config)                  = 0;
  virtual void network_on_change_nearby_1d(const NodeID& prev_nid, const NodeID& next_nid)      = 0;
  virtual void network_on_change_nearby_2d(const std::set<NodeID>& nids)                        = 0;
  virtual void network_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) = 0;
  virtual const CoordSystem* network_on_require_coord_system(const picojson::object& config)    = 0;
};

class Network : public NodeAccessorDelegate, public RoutingDelegate, public SeedAccessorDelegate {
 public:
  Network(Logger& l, Random& r, Scheduler& s, CommandManager& c, const NodeID& n, NetworkDelegate&);
  virtual ~Network();

  void connect(
      const std::string& url, const std::string& token, std::function<void()>&& on_success,
      std::function<void(const Error&)>&& on_failure);
  void disconnect(std::function<void()>&& on_success, std::function<void(const Error&)>&& on_failure);
  bool is_connected();

  void switch_packet(std::unique_ptr<const Packet> packet, bool is_from_seed);
  void relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet);
  void change_local_position(const Coordinate& position);

 private:
  Logger& logger;
  Random& random;
  Scheduler& scheduler;
  CommandManager& command_manager;
  const NodeID& local_nid;

  NetworkDelegate& delegate;

  bool enforce_online;
  LinkState::Type link_state;
  std::function<void()> cb_connect_success;
  std::function<void(const Error&)> cb_connect_failure;

  std::unique_ptr<SeedAccessor> seed_accessor;
  std::unique_ptr<NodeAccessor> node_accessor;
  std::unique_ptr<Routing> routing;

  void node_accessor_on_change_online_links(const std::set<NodeID>& nids) override;
  void node_accessor_on_change_state() override;
  void node_accessor_on_recv_packet(const NodeID& nid, std::unique_ptr<const Packet> packet) override;

  void routing_do_connect_node(const NodeID& nid) override;
  void routing_do_disconnect_node(const NodeID& nid) override;
  void routing_do_connect_seed() override;
  void routing_do_disconnect_seed() override;
  void routing_on_module_1d_change_nearby(const NodeID& prev_nid, const NodeID& next_nid) override;
  void routing_on_module_2d_change_nearby(const std::set<NodeID>& nids) override;
  void routing_on_module_2d_change_nearby_position(const std::map<NodeID, Coordinate>& positions) override;

  void seed_accessor_on_change_state() override;
  void seed_accessor_on_recv_config(const picojson::object& config) override;
  void seed_accessor_on_recv_packet(std::unique_ptr<const Packet> packet) override;
  void seed_accessor_on_recv_require_random() override;

  void update_accessor_state();
};
}  // namespace colonio
