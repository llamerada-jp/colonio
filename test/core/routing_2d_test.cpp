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

#include "core/routing_2d.hpp"

#include <gtest/gtest.h>

#include "core/context.hpp"
#include "core/coord_system_plane.hpp"
#include "core/logger.hpp"
#include "core/packet.hpp"
#include "core/scheduler.hpp"

using namespace colonio;

class DummyDelegate : public RoutingAlgorithm2DDelegate {
 public:
  std::set<NodeID> neighbors;
  std::map<NodeID, Coordinate> nearby_position;

  void algorithm_2d_on_change_nearby(RoutingAlgorithm& algorithm, const std::set<NodeID>& nids) override {
    neighbors = nids;
  }

  void algorithm_2d_on_change_nearby_position(
      RoutingAlgorithm& algorithm, const std::map<NodeID, Coordinate>& positions) override {
    nearby_position = positions;
  }
};

class DummyLoggerDelegate : public LoggerDelegate {
 public:
  void logger_on_output(Logger& logger, const std::string& json) override {
    std::cout << json << std::endl;
  }
};

class DummySchedulerDelegate : public SchedulerDelegate {
 public:
  void scheduler_on_require_invoke(Scheduler& sched, unsigned int msec) override {
    // Routing2D dose not use a scheduler.
  }
};

std::shared_ptr<CoordSystem> cerate_coord_system_plane(double x_min, double x_max, double y_min, double y_max) {
  picojson::object conf;
  conf.insert(std::make_pair("xMin", picojson::value(x_min)));
  conf.insert(std::make_pair("xMax", picojson::value(x_max)));
  conf.insert(std::make_pair("yMin", picojson::value(y_min)));
  conf.insert(std::make_pair("yMax", picojson::value(y_max)));
  return std::make_shared<CoordSystemPlane>(conf);
}

TEST(Routing2DTest, test) {
  DummyLoggerDelegate logger_delegate;
  Logger logger(logger_delegate);
  DummySchedulerDelegate scheduler_delegate;
  Scheduler scheduler(scheduler_delegate);
  Context context(logger, scheduler);
  DummyDelegate routing_2d_delegate;
  std::shared_ptr<CoordSystem> coord_system_plane = cerate_coord_system_plane(-1.0, 1.0, -1.0, 1.0);

  Routing2D routing_2d(context, routing_2d_delegate, *coord_system_plane);

  coord_system_plane->set_local_position(Coordinate(0.0, 0.0));
  routing_2d.on_change_local_position(Coordinate(0.0, 0.0));

  EXPECT_EQ(routing_2d.get_required_nodes().size(), 0);

  std::set<NodeID> online_links;
  static const int NID_COUNT = 6;
  NodeID nids[NID_COUNT - 1];
  for (int idx = 0; idx < NID_COUNT; idx++) {
    char buf[34];
    snprintf(buf, sizeof(buf), "0000000000000000000000000000%04d", idx);
    nids[idx] = NodeID::from_str(buf);
  }
  online_links.insert(nids[0]);

  std::map<NodeID, std::tuple<std::unique_ptr<const Packet>, RoutingProtocol::RoutingInfo>> routing_infos;
  std::unique_ptr<Packet> packet = std::make_unique<Packet>(Packet{nids[0], nids[0], 0, 0, nullptr, 0, 0, 0, 0});
  RoutingProtocol::RoutingInfo info;
  info.set_seed_distance(0);
  nids[0].to_pb(info.mutable_seed_nid());
  routing_2d.update_routing_info(online_links, true, routing_infos);
  std::set<NodeID> required = routing_2d.get_required_nodes();
}
