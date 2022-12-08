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

#include "colonio/colonio.hpp"
#include "command.hpp"

namespace colonio {
class CommandManager;
class Logger;
class Random;
class Scheduler;

typedef uint32_t PAXOS_N;

class KVSDelegate {
 public:
  virtual ~KVSDelegate();
  virtual bool kvs_on_check_covered_range(const NodeID& nid) = 0;
};

class KVS {
 public:
  KVS(Logger& l, Random& r, Scheduler& s, CommandManager& c, KVSDelegate& d, const picojson::object& config);
  virtual ~KVS();

  std::shared_ptr<std::map<std::string, Value>> get_local_data();
  void get(
      const std::string& key, std::function<void(const Value&)>&& on_success,
      std::function<void(const Error&)>&& on_failure);
  void set(
      const std::string& key, const Value& value, uint32_t opt, std::function<void()>&& on_success,
      std::function<void(const Error&)>&& on_failure);

  void balance_records(const NodeID& prev_nid, const NodeID& next_nid);

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
      KVS& parent;
      std::string key;
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

      Info(KVS& p, std::string k, int c);
    };

    Logger& logger;
    std::shared_ptr<Info> info;

    CommandGet(std::shared_ptr<Info> i);

    void on_response(const Packet& packet) override;
    void on_error(ErrorCode code, const std::string& message) override;
    const std::tuple<PAXOS_N, PAXOS_N, Value>* get_best();
    void procedure();
  };

  class CommandSet : public Command {
   public:
    class Info {
     public:
      KVS& parent;
      const std::string key;
      const Value value;
      std::function<void()> cb_on_success;
      std::function<void(const Error&)> cb_on_failure;
      const uint32_t opt;

      Info(
          KVS& p, const std::string& k, const Value& v, const std::function<void()> cb_on_success_,
          const std::function<void(const Error&)> cb_on_failure_, const uint32_t& o);
    };

    Logger& logger;
    std::unique_ptr<Info> info;

    explicit CommandSet(std::unique_ptr<Info> i);

    void on_response(const Packet& packet) override;
    void on_error(ErrorCode code, const std::string& message) override;
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
      const std::string key;
      PAXOS_N n_max;
      PAXOS_N i_max;
      uint32_t opt;
      KVS& parent;
      std::vector<Reply> replies;
      bool has_value;
      bool is_finished;
      Info(KVS& p, std::unique_ptr<const Packet> r, const std::string& k, uint32_t o);
      virtual ~Info();
    };

    Logger& logger;
    std::shared_ptr<Info> info;

    explicit CommandPrepare(std::shared_ptr<Info> i);

    void on_response(const Packet& packet) override;
    void on_error(ErrorCode code, const std::string& message) override;
    void procedure();
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
      const std::string key;
      PAXOS_N n_max;
      PAXOS_N i_max;
      uint32_t opt;
      KVS& parent;
      std::vector<Reply> replies;
      bool has_value;
      bool is_finished;

      Info(KVS& p, std::unique_ptr<const Packet> r, const std::string& k, uint32_t o);
      virtual ~Info();
    };

    Logger& logger;
    std::shared_ptr<Info> info;

    explicit CommandAccept(std::shared_ptr<Info> i);

    void on_response(const Packet& packet) override;
    void on_error(ErrorCode code, const std::string& message) override;
    void procedure();
  };

  const unsigned int CONF_RETRY_MAX;
  const uint32_t CONF_RETRY_INTERVAL_MIN;
  const uint32_t CONF_RETRY_INTERVAL_MAX;

  Logger& logger;
  Random& random;
  Scheduler& scheduler;
  CommandManager& command_manager;
  KVSDelegate& delegate;

  std::string salt;
  std::map<std::pair<std::string, uint32_t>, AcceptorInfo> acceptor_infos;
  std::map<std::string, ProposerInfo> proposer_infos;

  bool check_key_acceptor(const std::string& key, uint32_t member_idx);
  bool check_key_proposer(const std::string& key);
  void recv_packet_accept(const Packet& packet);
  void recv_packet_balance_acceptor(const Packet& packet);
  void recv_packet_balance_proposer(const Packet& packet);
  void recv_packet_get(const Packet& packet);
  void recv_packet_hint(const Packet& packet);
  void recv_packet_prepare(const Packet& packet);
  void recv_packet_set(const Packet& packet);
  void send_packet_accept(
      ProposerInfo& proposer, std::unique_ptr<const Packet> packet_reply, const std::string& key, uint32_t opt);
  void send_packet_balance_acceptor(const std::string& key, uint32_t member_idx, const AcceptorInfo& acceptor);
  void send_packet_balance_proposer(const std::string& key, const ProposerInfo& proposer);
  void send_packet_get(
      const std::string& key, int count_retry, int64_t interval, const std::function<void(const Value&)> on_success,
      const std::function<void(const Error&)> on_failure);
  void send_packet_hint(const std::string& key, const Value& value, PAXOS_N n, PAXOS_N i);
  void send_packet_prepare(
      ProposerInfo& proposer, std::unique_ptr<const Packet> packet_reply, const std::string& key, uint32_t opt);
  void send_packet_set(std::unique_ptr<CommandSet::Info> info);

#ifndef NDEBUG
  void debug_on_change_set();
#endif
};
}  // namespace colonio
