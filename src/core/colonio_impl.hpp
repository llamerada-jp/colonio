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
#include "command_manager.hpp"
#include "definition.hpp"
#include "kvs.hpp"
#include "logger.hpp"
#include "messaging.hpp"
#include "network.hpp"
#include "node_id.hpp"
#include "random.hpp"
#include "scheduler.hpp"
#include "user_thread_pool.hpp"

namespace colonio {
class ColonioImpl : public Colonio, public NetworkDelegate, public CommandManagerDelegate, public KVSDelegate {
 public:
  ColonioImpl(const ColonioConfig& config);
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

  std::tuple<double, double> set_position(double x, double y) override;

  Value messaging_post(
      const std::string& dst_nid, const std::string& name, const Value& message, uint32_t opt = 0x0) override;
  void messaging_post(
      const std::string& dst_nid, const std::string& name, const Value& message, uint32_t opt,
      std::function<void(Colonio&, const Value&)>&& on_response,
      std::function<void(Colonio&, const Error&)>&& on_failure) override;

  void messaging_set_handler(
      const std::string& name, std::function<Value(Colonio&, const MessagingRequest&)>&& func) override;
  void messaging_set_handler(
      const std::string& name,
      std::function<void(Colonio&, const MessagingRequest&, std::shared_ptr<MessagingResponseWriter>)>&& func) override;
  void messaging_unset_handler(const std::string& name) override;

  std::shared_ptr<std::map<std::string, Value>> kvs_get_local_data() override;
  void kvs_get_local_data(
      std::function<void(Colonio&, std::shared_ptr<std::map<std::string, Value>>)> handler) override;
  Value kvs_get(const std::string& key) override;
  void kvs_get(
      const std::string& key, std::function<void(Colonio&, const Value&)>&& on_success,
      std::function<void(Colonio&, const Error&)>&& on_failure) override;
  void kvs_set(const std::string& key, const Value& value, uint32_t opt = 0x0) override;
  void kvs_set(
      const std::string& key, const Value& value, uint32_t opt, std::function<void(Colonio&)>&& on_success,
      std::function<void(Colonio&, const Error&)>&& on_failure) override;

 private:
  Logger logger;
  Random random;
  std::unique_ptr<Scheduler> scheduler;
  std::unique_ptr<UserThreadPool> user_thread_pool;
  std::unique_ptr<CommandManager> command_manager;
  NodeID local_nid;
  std::unique_ptr<Network> network;

  picojson::object global_config;
  const ColonioConfig local_config;

  std::unique_ptr<CoordSystem> coord_system;
  std::unique_ptr<Messaging> messaging;
  std::unique_ptr<KVS> kvs;

  void command_manager_do_send_packet(std::unique_ptr<const Packet> packet) override;
  void command_manager_do_relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet) override;

  void network_on_change_global_config(const picojson::object& config) override;
  void network_on_change_nearby_1d(const NodeID& prev_nid, const NodeID& next_nid) override;
  void network_on_change_nearby_2d(const std::set<NodeID>& nids) override;
  void network_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) override;
  const CoordSystem* network_on_require_coord_system(const picojson::object& config) override;

  bool kvs_on_check_covered_range(const NodeID& nid) override;

  void allocate_resources();
  void release_resources();
};
}  // namespace colonio
