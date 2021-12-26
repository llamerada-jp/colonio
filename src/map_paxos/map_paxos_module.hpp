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

#include "colonio/map.hpp"
#include "colonio/value.hpp"
#include "core/command.hpp"
#include "core/map_base.hpp"

namespace colonio {
class Random;
typedef uint32_t PAXOS_N;

class MapPaxosModule : public MapBase {
 public:
  static MapPaxosModule* new_instance(
      ModuleParam& param, Module1DDelegate& module_1d_delegate, const picojson::object& config);

  virtual ~MapPaxosModule();
  MapPaxosModule(const MapPaxosModule&) = delete;
  MapPaxosModule& operator=(const MapPaxosModule&) = delete;

  void foreach_local_value(std::function<void(const Value&, const Value&, uint32_t)>&& func) override;
  void get(
      const Value& key, std::function<void(const Value&)>&& on_success,
      std::function<void(const Error&)>&& on_failure) override;
  void set(
      const Value& key, const Value& value, uint32_t opt, std::function<void()>&& on_success,
      std::function<void(const Error&)>&& on_failure) override;

  void module_1d_on_change_nearby(const NodeID& prev_nid, const NodeID& next_nid) override;

 private:
  class AcceptorInfo {
   public:
    PAXOS_N na;
    PAXOS_N np;
    PAXOS_N ia;
    Value value;
    NodeID last_nid;

    AcceptorInfo();
    AcceptorInfo(PAXOS_N na_, PAXOS_N np_, PAXOS_N ia_, const Value& value_);
  };

  struct ProposerInfo {
    PAXOS_N np;
    PAXOS_N ip;
    bool reset;
    Value value;
    uint32_t processing_packet_id;

    ProposerInfo();
    ProposerInfo(PAXOS_N np_, PAXOS_N ip_, const Value& value_);
  };

  class CommandGet : public Command {
   public:
    class Info {
     public:
      MapPaxosModule& parent;
      std::unique_ptr<Value> key;
      unsigned int count_retry;
      int64_t time_send;

      // (n, i), value
      std::map<std::tuple<PAXOS_N, PAXOS_N>, Value> ok_values;
      // (n, i), count
      std::map<std::tuple<PAXOS_N, PAXOS_N>, unsigned int> ok_counts;

      int count_ng;
      bool is_finished;
      std::function<void(const Value&)> cb_on_success;
      std::function<void(const Error&)> cb_on_failure;

      Info(MapPaxosModule& parent_, std::unique_ptr<Value> key_, int count_retry_);
    };

    Random& random;
    std::shared_ptr<Info> info;

    CommandGet(Random& random_, std::shared_ptr<Info> info_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;
    const std::tuple<PAXOS_N, PAXOS_N, Value>* get_best();
    void postprocess();
  };

  class CommandSet : public Command {
   public:
    class Info {
     public:
      MapPaxosModule& parent;
      const Value key;
      const Value value;
      std::function<void()> cb_on_success;
      std::function<void(const Error&)> cb_on_failure;
      const uint32_t opt;

      Info(
          MapPaxosModule& parent_, const Value& key_, const Value& value_, const std::function<void()> cb_on_success_,
          const std::function<void(const Error&)> cb_on_failure_, const uint32_t& opt_);
    };
    std::unique_ptr<Info> info;

    explicit CommandSet(std::unique_ptr<Info> info_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;
  };

  class CommandPrepare : public Command {
   public:
    class Reply {
     public:
      NodeID src_nid;
      PAXOS_N n;
      PAXOS_N i;
      bool is_success;

      Reply(const NodeID& src_nid_, PAXOS_N n_, PAXOS_N i_, bool is_success_);
    };

    class Info {
     public:
      std::unique_ptr<const Packet> packet_reply;
      std::unique_ptr<Value> key;
      PAXOS_N n_max;
      PAXOS_N i_max;
      uint32_t opt;
      MapPaxosModule& parent;
      std::vector<Reply> replies;
      bool has_value;
      bool is_finished;
      Info(
          MapPaxosModule& parent_, std::unique_ptr<const Packet> packet_reply_, std::unique_ptr<Value> key_,
          uint32_t opt_);
      virtual ~Info();
    };

    std::shared_ptr<Info> info;

    explicit CommandPrepare(std::shared_ptr<Info> info_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;
    void postprocess();
  };

  class CommandAccept : public Command {
   public:
    class Reply {
     public:
      NodeID src_nid;
      PAXOS_N n;
      PAXOS_N i;
      bool is_success;

      Reply(const NodeID& src_nid_, PAXOS_N n_, PAXOS_N i_, bool is_success_);
    };

    class Info {
     public:
      std::unique_ptr<const Packet> packet_reply;
      std::unique_ptr<Value> key;
      PAXOS_N n_max;
      PAXOS_N i_max;
      uint32_t opt;
      MapPaxosModule& parent;
      std::vector<Reply> replies;
      bool has_value;
      bool is_finished;

      Info(
          MapPaxosModule& parent_, std::unique_ptr<const Packet> packet_reply_, std::unique_ptr<Value> key_,
          uint32_t opt_);
      virtual ~Info();
    };

    std::shared_ptr<Info> info;

    explicit CommandAccept(std::shared_ptr<Info> info_);

    void on_error(const std::string& message) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_success(std::unique_ptr<const Packet> packet) override;
    void postprocess();
  };

  const unsigned int CONF_RETRY_MAX;
  const uint32_t CONF_RETRY_INTERVAL_MIN;
  const uint32_t CONF_RETRY_INTERVAL_MAX;

  std::string salt;
  std::map<std::pair<Value, uint32_t>, AcceptorInfo> acceptor_infos;
  std::map<Value, ProposerInfo> proposer_infos;

  MapPaxosModule(
      ModuleParam& param, Module1DDelegate& module_1d_delegate, Channel::Type channel, unsigned int retry_max,
      uint32_t retry_interval_min, uint32_t retry_interval_max);

  void module_process_command(std::unique_ptr<const Packet> packet) override;

  bool check_key_acceptor(const Value& key, uint32_t member_idx);
  bool check_key_proposer(const Value& key);
  void recv_packet_accept(std::unique_ptr<const Packet> packet);
  void recv_packet_balance_acceptor(std::unique_ptr<const Packet> packet);
  void recv_packet_balance_proposer(std::unique_ptr<const Packet> packet);
  void recv_packet_get(std::unique_ptr<const Packet> packet);
  void recv_packet_hint(std::unique_ptr<const Packet> packet);
  void recv_packet_prepare(std::unique_ptr<const Packet> packet);
  void recv_packet_set(std::unique_ptr<const Packet> packet);
  void send_packet_accept(
      ProposerInfo& proposer, std::unique_ptr<const Packet> packet_reply, std::unique_ptr<Value> key, uint32_t opt);
  void send_packet_balance_acceptor(const Value& key, uint32_t member_idx, const AcceptorInfo& acceptor);
  void send_packet_balance_proposer(const Value& key, const ProposerInfo& proposer);
  void send_packet_get(
      std::unique_ptr<Value> key, int count_retry, int64_t interval, const std::function<void(const Value&)> on_success,
      const std::function<void(const Error&)> on_failure);
  void send_packet_hint(const Value& key, const Value& value, PAXOS_N n, PAXOS_N i);
  void send_packet_prepare(
      ProposerInfo& proposer, std::unique_ptr<const Packet> packet_reply, std::unique_ptr<Value> key, uint32_t opt);
  void send_packet_set(std::unique_ptr<CommandSet::Info> info);

#ifndef NDEBUG
  void debug_on_change_set();
#endif
};
}  // namespace colonio
