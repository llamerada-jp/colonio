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

#include "api_base.hpp"
#include "module_bundler.hpp"
#include "definition.hpp"
#include "node_accessor.hpp"
#include "node_id.hpp"
#include "routing.hpp"
#include "seed_accessor.hpp"
#include "module_1d.hpp"
#include "module_2d.hpp"

namespace colonio {
class APIBundler;
class Context;

class ColonioImpl : public APIBase,
                    public ModuleDelegate,
                    public NodeAccessorDelegate,
                    public RoutingDelegate,
                    public SeedAccessorDelegate,
                    public Module1DDelegate,
                    public Module2DDelegate {
 public:
  ColonioImpl(Context& context_, APIDelegate& api_delegate_, APIBundler& api_bundler_);
  virtual ~ColonioImpl();

  LinkStatus::Type get_status();

 private:
  APIDelegate& api_delegate;
  APIBundler& api_bundler;
  ModuleBundler module_bundler;
  picojson::object config;
  bool enable_retry;

  uint32_t api_connect_id;
  std::unique_ptr<api::colonio::ConnectReply> api_connect_reply;

  std::unique_ptr<SeedAccessor> seed_accessor;
  std::unique_ptr<NodeAccessor> node_accessor;
  std::unique_ptr<Routing> routing;

  LinkStatus::Type node_status;
  LinkStatus::Type seed_status;

  void api_on_recv_call(const api::Call& call) override;

  void module_do_send_packet(ModuleBase& module, std::unique_ptr<const Packet> packet) override;
  void module_do_relay_packet(ModuleBase& module, const NodeID& dst_nid, std::unique_ptr<const Packet> packet) override;

  void node_accessor_on_change_online_links(NodeAccessor& na, const std::set<NodeID> nids) override;
  void node_accessor_on_change_status(NodeAccessor& na, LinkStatus::Type status) override;
  void node_accessor_on_recv_packet(NodeAccessor& na, const NodeID& nid, std::unique_ptr<const Packet> packet) override;

  void routing_do_connect_node(Routing& routing, const NodeID& nid) override;
  void routing_do_disconnect_node(Routing& routing, const NodeID& nid) override;
  void routing_do_connect_seed(Routing& route) override;
  void routing_do_disconnect_seed(Routing& route) override;
  void routing_on_module_1d_change_nearby(Routing& routing, const NodeID& prev_nid, const NodeID& next_nid) override;
  void routing_on_module_2d_change_nearby(Routing& routing, const std::set<NodeID>& nids) override;
  void routing_on_module_2d_change_nearby_position(
      Routing& routing, const std::map<NodeID, Coordinate>& positions) override;

  void seed_accessor_on_change_status(SeedAccessor& sa, LinkStatus::Type status) override;
  void seed_accessor_on_recv_config(SeedAccessor& sa, const picojson::object& config) override;
  void seed_accessor_on_recv_packet(SeedAccessor& sa, std::unique_ptr<const Packet> packet) override;
  void seed_accessor_on_recv_require_random(SeedAccessor& sa) override;

  bool module_1d_do_check_covered_range(Module1D& module_1d, const NodeID& nid) override;

  const NodeID& module_2d_do_get_relay_nid(Module2D& module_2d, const Coordinate& position) override;

  void api_connect(uint32_t id, const api::colonio::Connect& param);
  void api_get_local_nid(uint32_t id);
  void api_disconnect(uint32_t id);
  void api_set_position(uint32_t id, const api::colonio::SetPosition& param);
  void check_api_connect();
  void initialize_algorithms();
  void on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status);
  void relay_packet(std::unique_ptr<const Packet> packet, bool is_from_seed);
};
}  // namespace colonio
