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

#include "colonio/exception.hpp"
#include "colonio/value.hpp"
#include "core/command.hpp"
#include "core/coordinate.hpp"
#include "core/module_2d.hpp"

namespace colonio {
class PubSub2DModule;

class PubSub2DModuleDelegate {
 public:
  virtual ~PubSub2DModuleDelegate();

  virtual void pubsub2d_module_on_on(PubSub2DModule& ps2_module, const std::string& name, const Value& value) = 0;
};

class PubSub2DModule : public Module2D {
 public:
  PubSub2DModule(
      Context& context, ModuleDelegate& module_delegate, Module2DDelegate& module_2d_delegate,
      PubSub2DModuleDelegate& delegate_, APIChannel::Type channel, ModuleChannel::Type module_channel,
      uint32_t cache_time);
  virtual ~PubSub2DModule();

  void publish(
      const std::string& name, double x, double y, double r, const Value& value, uint32_t opt,
      const std::function<void()>& on_success, const std::function<void(Exception::Code)>& on_failure);

  void module_process_command(std::unique_ptr<const Packet> packet) override;

  void module_2d_on_change_my_position(const Coordinate& position) override;
  void module_2d_on_change_nearby(const std::set<NodeID>& nids) override;
  void module_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) override;

 private:
  unsigned int CONF_CACHE_TIME;

  PubSub2DModuleDelegate& delegate;

  std::map<NodeID, Coordinate> next_positions;

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
    CommandKnock(PubSub2DModule& parent_, uint64_t uid_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;

   private:
    PubSub2DModule& parent;
    const uint64_t uid;
  };

  class CommandPass : public Command {
   public:
    CommandPass(
        PubSub2DModule& parent_, uint64_t uid_, const std::function<void()>& cb_on_success_,
        const std::function<void(Exception::Code)>& cb_on_failure_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;

   private:
    PubSub2DModule& parent;
    const uint64_t uid;
    const std::function<void()> cb_on_success;
    const std::function<void(Exception::Code)> cb_on_failure;
  };

  uint64_t assign_uid();
  void clear_cache();

  void recv_packet_knock(std::unique_ptr<const Packet> packet);
  void recv_packet_deffuse(std::unique_ptr<const Packet> packet);
  void recv_packet_pass(std::unique_ptr<const Packet> packet);

  void send_packet_knock(const NodeID& exclude, const Cache& cache);
  void send_packet_deffuse(const NodeID& dst_nid, const Cache& cache);
  void send_packet_pass(
      const Cache& cache, const std::function<void()>& on_success,
      const std::function<void(Exception::Code)>& on_failure);
};
}  // namespace colonio
