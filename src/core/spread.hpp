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
#include <map>
#include <string>

#include "colonio/colonio.hpp"
#include "command.hpp"
#include "coordinate.hpp"

namespace colonio {
class CommandManager;
class CoordSystem;
class Logger;
class NodeID;
class Random;
class Scheduler;

class SpreadDelegate {
 public:
  virtual ~SpreadDelegate();
  virtual const NodeID& spread_do_get_relay_nid(const Coordinate& position) = 0;
};

class Spread {
 public:
  Spread(
      Logger& l, Random& r, Scheduler& s, CommandManager& c, const NodeID& n, CoordSystem& cs, SpreadDelegate& d,
      const picojson::object& config);

  virtual ~Spread();

  void post(
      double x, double y, double r, const std::string& name, const Value& message, uint32_t opt,
      std::function<void()>&& on_success, std::function<void(const Error&)>&& on_failure);

  void set_handler(const std::string& name, std::function<void(const Colonio::SpreadRequest&)>&& handler);
  void unset_handler(const std::string& name);

  void update_next_positions(const std::map<NodeID, Coordinate>& positions);

 private:
  struct Cache {
    NodeID src;
    std::string name;
    Coordinate center;
    double r;
    uint64_t uid;
    int64_t create_time;
    Value message;
    uint32_t opt;
  };

  class CommandKnock : public Command {
   public:
    CommandKnock(Spread& p, uint64_t u);

    void on_response(const Packet& packet) override;
    void on_error(ErrorCode code, const std::string& message) override;

   private:
    Logger& logger;
    Spread& parent;
    const uint64_t uid;
  };

  class CommandRelay : public Command {
   public:
    CommandRelay(Spread& p, std::function<void()>& s, std::function<void(const Error&)>& f);

    void on_response(const Packet& packet) override;
    void on_error(ErrorCode code, const std::string& message) override;

   private:
    Logger& logger;
    Spread& parent;
    std::function<void()> cb_on_success;
    std::function<void(const Error&)> cb_on_failure;
  };

  unsigned int CONF_CACHE_TIME;

  Logger& logger;
  Random& random;
  Scheduler& scheduler;
  CommandManager& command_manager;
  const NodeID& local_nid;
  CoordSystem& coord_system;
  SpreadDelegate& delegate;

  // Pair of uid and cache data.
  std::map<uint64_t, Cache> cache;

  std::map<NodeID, Coordinate> next_positions;
  std::map<std::string, std::function<void(const Colonio::SpreadRequest&)>> handlers;

  uint64_t assign_uid();
  void clear_cache();

  void recv_spread(const Packet& packet);
  void recv_knock(const Packet& packet);
  void recv_relay(const Packet& packet);

  void send_spread(const NodeID& dst_nid, const Cache& cache);
  void send_knock(const NodeID& exclude, const Cache& cache);
  void send_relay(const Cache& cache, std::function<void()> on_success, std::function<void(const Error&)> on_failure);
  void send_response(const Packet& packet, bool success, ErrorCode code, const std::string& message);
};
}  // namespace colonio
