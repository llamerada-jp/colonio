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

#include "core/system_2d.hpp"
#include "colonio/pubsub_2d.hpp"

namespace colonio {
class PubSub2DImpl : public System2D<PubSub2D> {
 public:
  PubSub2DImpl(Context& context, ModuleDelegate& module_delegate,
               System2DDelegate& system_delegate, const picojson::object& config);
  virtual ~PubSub2DImpl();

  void publish(const std::string& name, double x, double y, double r, const Value& value,
               const std::function<void()>& on_success,
               const std::function<void(PubSub2DFailureReason)>& on_failure) override;
  void on(const std::string& name,
          const std::function<void(const Value&)>& subscriber) override;
  void off(const std::string& name) override;

  void module_process_command(std::unique_ptr<const Packet> packet) override;

  void system_2d_on_change_my_position(const Coordinate& position) override;
  void system_2d_on_change_nearby(const std::set<NodeID>& nids) override;
  void system_2d_on_change_nearby_position(const std::map<NodeID, Coordinate>& positions) override;

 private:
  unsigned int conf_cache_time;

  std::map<std::string, std::function<void(const Value&)>> funcs_subscriber;
  std::map<NodeID, Coordinate> next_positions;

  struct Cache {
    std::string name;
    Coordinate center;
    double r;
    uint64_t uid;
    int64_t create_time;
    picojson::value data;
  };
  // Pair of uid and cache data.
  std::map<uint64_t, Cache> cache;

  class CommandKnock : public Command {
   public:
    CommandKnock(PubSub2DImpl& parent_, uint64_t uid_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;

   private:
    PubSub2DImpl& parent;
    const uint64_t uid;
  };

  class CommandPass : public Command {
   public:
    CommandPass(PubSub2DImpl& parent_, uint64_t uid_,
                const std::function<void()>& cb_on_success_,
                const std::function<void(PubSub2DFailureReason)>& cb_on_failure_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;

   private:
    PubSub2DImpl& parent;
    const uint64_t uid;
    const std::function<void()> cb_on_success;
    const std::function<void(PubSub2DFailureReason)> cb_on_failure;
  };

  uint64_t assign_uid();
  void clear_cache();

  void recv_packet_knock(std::unique_ptr<const Packet> packet);
  void recv_packet_deffuse(std::unique_ptr<const Packet> packet);
  void recv_packet_pass(std::unique_ptr<const Packet> packet);

  void send_packet_knock(const NodeID& exclude, const Cache& cache);
  void send_packet_deffuse(const NodeID& dst_nid, const Cache& cache);
  void send_packet_pass(const Cache& cache,
                        const std::function<void()>& on_success,
                        const std::function<void(PubSub2DFailureReason)>& on_failure);

};
}  // namespace colonio
