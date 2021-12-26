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
#include <string>

#include "colonio/colonio.hpp"
#include "definition.hpp"
#include "logger.hpp"
#include "module_1d.hpp"
#include "module_2d.hpp"
#include "node_accessor.hpp"
#include "node_id.hpp"
#include "random.hpp"
#include "routing.hpp"
#include "scheduler.hpp"
#include "seed_accessor.hpp"

namespace colonio {
class ColonioModule;
class MapImpl;
class Pubsub2DImpl;

class ColonioImpl : public Colonio,
                    public LoggerDelegate,
                    public ModuleDelegate,
                    public NodeAccessorDelegate,
                    public RoutingDelegate,
                    public SeedAccessorDelegate,
                    public Module1DDelegate,
                    public Module2DDelegate {
 public:
  ColonioImpl(std::function<void(Colonio&, const std::string&)> log_receiver_, uint32_t opt);
  virtual ~ColonioImpl();

  void connect(const std::string& url, const std::string& token) override;
  void connect(
      const std::string& url, const std::string& token, std::function<void(Colonio&)>&& on_success,
      std::function<void(Colonio&, const Error&)>&& on_failure) override;
  void disconnect() override;
  void disconnect(
      std::function<void(Colonio&)>&& on_success, std::function<void(Colonio&, const Error&)>&& on_failure) override;
  bool is_connected() override;
  std::string get_local_nid() override;

  Map& access_map(const std::string& name) override;
  Pubsub2D& access_pubsub_2d(const std::string& name) override;

  std::tuple<double, double> set_position(double x, double y) override;
  void set_position(
      double x, double y, std::function<void(Colonio&, double, double)> on_success,
      std::function<void(Colonio&, const Error&)> on_failure) override;

  Value call_by_nid(
      const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt = 0x00) override;
  void call_by_nid(
      const std::string& dst_nid, const std::string& name, const Value& value, uint32_t opt,
      std::function<void(Colonio&, const Value&)>&& on_success, std::function<void(Colonio&, const Error&)>&& on_failure) override;
  void on_call(const std::string& name, std::function<Value(Colonio&, const CallParameter&)>&& func) override;
  void off_call(const std::string& name) override;

  void start_on_event_thread() override;
  void start_on_controller_thread() override;

 private:
  std::function<void(Colonio&, const std::string&)> log_receiver;
  Logger logger;
  Random random;
  std::unique_ptr<Scheduler> scheduler;
  const NodeID local_nid;
  ModuleParam module_param;
  picojson::object config;
  bool enable_retry;

  std::unique_ptr<SeedAccessor> seed_accessor;
  NodeAccessor* node_accessor;
  std::unique_ptr<CoordSystem> coord_system;
  Routing* routing;
  ColonioModule* colonio_module;

  std::map<Channel::Type, std::unique_ptr<ModuleBase>> modules;
  std::map<std::string, Channel::Type> module_names;
  std::set<Module1D*> modules_1d;
  std::set<Module2D*> modules_2d;
  std::map<std::string, std::unique_ptr<MapImpl>> if_map;
  std::map<std::string, std::unique_ptr<Pubsub2DImpl>> if_pubsub2d;

  std::unique_ptr<std::pair<std::function<void(Colonio&)>, std::function<void(Colonio&, const Error&)>>> connect_cb;
  LinkState::Type link_state;

  void logger_on_output(Logger& logger, const std::string& json) override;

  void module_do_send_packet(ModuleBase& module, std::unique_ptr<const Packet> packet) override;
  void module_do_relay_packet(ModuleBase& module, const NodeID& dst_nid, std::unique_ptr<const Packet> packet) override;

  void node_accessor_on_change_online_links(NodeAccessor& na, const std::set<NodeID>& nids) override;
  void node_accessor_on_change_state(NodeAccessor& na) override;
  void node_accessor_on_recv_packet(NodeAccessor& na, const NodeID& nid, std::unique_ptr<const Packet> packet) override;

  void routing_do_connect_node(Routing& routing, const NodeID& nid) override;
  void routing_do_disconnect_node(Routing& routing, const NodeID& nid) override;
  void routing_do_connect_seed(Routing& route) override;
  void routing_do_disconnect_seed(Routing& route) override;
  void routing_on_module_1d_change_nearby(Routing& routing, const NodeID& prev_nid, const NodeID& next_nid) override;
  void routing_on_module_2d_change_nearby(Routing& routing, const std::set<NodeID>& nids) override;
  void routing_on_module_2d_change_nearby_position(
      Routing& routing, const std::map<NodeID, Coordinate>& positions) override;

  void seed_accessor_on_change_state(SeedAccessor& sa) override;
  void seed_accessor_on_recv_config(SeedAccessor& sa, const picojson::object& config) override;
  void seed_accessor_on_recv_packet(SeedAccessor& sa, std::unique_ptr<const Packet> packet) override;
  void seed_accessor_on_recv_require_random(SeedAccessor& sa) override;

  bool module_1d_do_check_covered_range(Module1D& module_1d, const NodeID& nid) override;

  const NodeID& module_2d_do_get_relay_nid(Module2D& module_2d, const Coordinate& position) override;

  void check_api_connect();
  void clear_modules();
  void initialize_algorithms();
  void register_module(ModuleBase* module, const std::string* name, bool is_1d, bool is_2d);
  void relay_packet(std::unique_ptr<const Packet> packet, bool is_from_seed);
  void update_accessor_state();
};
}  // namespace colonio
