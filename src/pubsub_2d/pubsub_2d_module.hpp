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

#include "colonio/value.hpp"
#include "core/command.hpp"
#include "core/coordinate.hpp"
#include "core/pubsub_2d_base.hpp"

namespace colonio {
class Pubsub2DModule : public Pubsub2DBase {
 public:
  static Pubsub2DModule* new_instance(
      ModuleParam& param, Module2DDelegate& module_2d_delegate, const CoordSystem& coord_system,
      const picojson::object& config);

  virtual ~Pubsub2DModule();

  void publish(
      const std::string& name, double x, double y, double r, const Value& value, uint32_t opt,
      std::function<void()>&& on_success, std::function<void(const Error&)>&& on_failure) override;

  void on(const std::string& name, std::function<void(const Value&)>&& subscriber) override;
  void off(const std::string& name) override;

  void module_process_command(std::unique_ptr<const Packet> packet) override;

  void module_2d_on_change_local_position(const Coordinate& position) override;
  void module_2d_on_change_nearby(const std::set<NodeID>& nids) override;
  void module_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) override;

 private:
  unsigned int CONF_CACHE_TIME;

  std::map<NodeID, Coordinate> next_positions;
  std::map<std::string, std::function<void(const Value&)>> subscribers;

  struct Cache {
    std::string name;
    Coordinate center;
    double r;
    uint64_t uid;
    int64_t create_time;
    Value data;
    uint32_t opt;
  };
  // Pair of uid and cache data.
  std::map<uint64_t, Cache> cache;

  class CommandKnock : public Command {
   public:
    CommandKnock(Pubsub2DModule& parent_, uint64_t uid_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;

   private:
    Pubsub2DModule& parent;
    const uint64_t uid;
  };

  class CommandPass : public Command {
   public:
    CommandPass(
        Pubsub2DModule& parent_, uint64_t uid_, std::function<void()>& cb_on_success_,
        std::function<void(const Error&)>& cb_on_failure_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;

   private:
    Pubsub2DModule& parent;
    const uint64_t uid;
    std::function<void()> cb_on_success;
    std::function<void(const Error&)> cb_on_failure;
  };

  Pubsub2DModule(
      ModuleParam& param, Module2DDelegate& module_2d_delegate, const CoordSystem& coord_system, Channel::Type channel,
      uint32_t cache_time);

  uint64_t assign_uid();
  void clear_cache();

  void recv_packet_knock(std::unique_ptr<const Packet> packet);
  void recv_packet_deffuse(std::unique_ptr<const Packet> packet);
  void recv_packet_pass(std::unique_ptr<const Packet> packet);

  void send_packet_knock(const NodeID& exclude, const Cache& cache);
  void send_packet_deffuse(const NodeID& dst_nid, const Cache& cache);
  void send_packet_pass(
      const Cache& cache, std::function<void()> on_success, std::function<void(const Error&)> on_failure);
};
}  // namespace colonio
