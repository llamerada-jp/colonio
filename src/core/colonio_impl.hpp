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

#include <functional>
#include <string>

#include "colonio/colonio.hpp"
#include "context.hpp"
#include "definition.hpp"
#include "logger.hpp"
#include "map_paxos/map_paxos.hpp"
#include "node_accessor.hpp"
#include "node_id.hpp"
#include "pubsub_2d/pubsub_2d_impl.hpp"
#include "routing.hpp"
#include "seed_accessor.hpp"
#include "system_1d.hpp"
#include "system_2d.hpp"

namespace colonio {
class ColonioImpl : public LoggerDelegate,
                    public ModuleDelegate,
                    public NodeAccessorDelegate,
                    public RoutingDelegate,
                    public SchedulerDelegate,
                    public SeedAccessorDelegate,
                    public System1DDelegate,
                    public System2DDelegate {
 public:
  Context context;
  picojson::object config;

  ColonioImpl(Colonio& colonio_);
  virtual ~ColonioImpl();

  template<typename T>
  T& access(const std::string& name) {
    auto it = modules_named.find(name);
    if (it == modules_named.end()) {
      // @todo error
      assert(false);
      T* ptr = nullptr;
      return *ptr;

    } else {
      return dynamic_cast<T&>(*it->second);
    }
  }

  void connect(
      const std::string& url, const std::string& token, std::function<void()> on_success,
      std::function<void()> on_failure);
  const NodeID& get_local_nid();
  LinkStatus::Type get_status();
  void disconnect();
  Coordinate set_position(const Coordinate& pos);

 private:
  Colonio& colonio;
  bool is_first_link;
  bool enable_retry;

  std::map<uint32_t, std::unique_ptr<Module>> modules;
  std::map<std::string, Module*> modules_named;
  std::set<System1DBase*> modules_1d;
  std::set<System2DBase*> modules_2d;

  std::unique_ptr<SeedAccessor> seed_accessor;
  NodeAccessor* node_accessor;
  Routing* routing;

  std::function<void()> on_connect_success;
  std::function<void()> on_connect_failure;

  LinkStatus::Type node_status;
  LinkStatus::Type seed_status;

  void logger_on_output(Logger& logger, LogLevel::Type level, const std::string& message) override;

  void module_do_send_packet(Module& module, std::unique_ptr<const Packet> packet) override;
  void module_do_relay_packet(Module& module, const NodeID& dst_nid, std::unique_ptr<const Packet> packet) override;

  void node_accessor_on_change_online_links(NodeAccessor& na, const std::set<NodeID> nids) override;
  void node_accessor_on_change_status(NodeAccessor& na, LinkStatus::Type status) override;
  void node_accessor_on_recv_packet(NodeAccessor& na, const NodeID& nid, std::unique_ptr<const Packet> packet) override;

  void routing_do_connect_node(Routing& routing, const NodeID& nid) override;
  void routing_do_disconnect_node(Routing& routing, const NodeID& nid) override;
  void routing_do_connect_seed(Routing& route) override;
  void routing_do_disconnect_seed(Routing& route) override;
  void routing_on_system_1d_change_nearby(Routing& routing, const NodeID& prev_nid, const NodeID& next_nid) override;
  void routing_on_system_2d_change_nearby(Routing& routing, const std::set<NodeID>& nids) override;
  void routing_on_system_2d_change_nearby_position(
      Routing& routing, const std::map<NodeID, Coordinate>& positions) override;

  void seed_accessor_on_change_status(SeedAccessor& sa, LinkStatus::Type status) override;
  void seed_accessor_on_recv_config(SeedAccessor& sa, const picojson::object& config) override;
  void seed_accessor_on_recv_packet(SeedAccessor& sa, std::unique_ptr<const Packet> packet) override;
  void seed_accessor_on_recv_require_random(SeedAccessor& sa) override;

  bool system_1d_do_check_coverd_range(const NodeID& nid) override;

  const NodeID& system_2d_do_get_relay_nid(const Coordinate& position) override;

  void scheduler_on_require_invoke(Scheduler& sched, unsigned int msec) override;

  void add_module(Module* module, const std::string& name = "");
  void initialize_algorithms();
  void on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status);
  void relay_packet(std::unique_ptr<const Packet> packet, bool is_from_seed);
};
}  // namespace colonio
