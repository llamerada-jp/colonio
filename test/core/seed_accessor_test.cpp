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
#include "core/seed_accessor.hpp"

#include <gtest/gtest.h>

#include <memory>

#include "core/colonio.pb.h"
#include "core/node_id.hpp"
#include "core/packet.hpp"
#include "core/random.hpp"
#include "core/scheduler.hpp"
#include "test_utils/all.hpp"

using namespace colonio;

const std::string SEED_URL("https://localhost:8080/test");

class SeedAccessorDelegateStub : public SeedAccessorDelegate {
  std::mutex mtx;
  bool _on_change_state;
  bool _on_error;
  bool _on_recv_config;
  std::unique_ptr<const Packet> last_packet;
  bool _on_require_random;

 public:
  SeedAccessorDelegateStub() {
    reset();
  }

  void reset() {
    _on_change_state = false;
    _on_error        = false;
    _on_recv_config  = false;
    last_packet.reset();
    _on_require_random = false;
  }

  bool called_on_change_state() {
    std::lock_guard<std::mutex> lock(mtx);
    return _on_change_state;
  }

  bool called_on_error() {
    std::lock_guard<std::mutex> lock(mtx);
    return _on_error;
  }

  bool called_on_recv_config() {
    std::lock_guard<std::mutex> lock(mtx);
    return _on_recv_config;
  }

  bool called_on_recv_packet() {
    std::lock_guard<std::mutex> lock(mtx);
    return static_cast<bool>(last_packet);
  }

  const Packet* packet() {
    std::lock_guard<std::mutex> lock(mtx);
    return last_packet.get();
  }

  void seed_accessor_on_change_state() override {
    std::lock_guard<std::mutex> lock(mtx);
    _on_change_state = true;
  }

  void seed_accessor_on_error(const std::string& message) override {
    std::lock_guard<std::mutex> lock(mtx);
    _on_error = true;
  }

  void seed_accessor_on_recv_config(const picojson::object& config) override {
    std::lock_guard<std::mutex> lock(mtx);
    _on_recv_config = true;
  }

  void seed_accessor_on_recv_packet(std::unique_ptr<const Packet> packet) override {
    std::lock_guard<std::mutex> lock(mtx);
    last_packet = std::move(packet);
  }

  void seed_accessor_on_require_random() override {
    std::lock_guard<std::mutex> lock(mtx);
    _on_require_random = true;
  }
};

TEST(SeedAccessorTest, single_node) {
  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  Random random;
  std::unique_ptr<Scheduler> scheduler(Scheduler::new_instance(logger));

  SeedAccessorDelegateStub delegate;
  SeedAccessor accessor(
      logger, *scheduler, NodeID::make_random(random), delegate, SEED_URL, std::string(""), 30 * 1000, true);

  scheduler->add_task(nullptr, [&]() {
    accessor.enable_polling(true);
    helper.pass_signal("polling");
  });
  helper.wait_signal("polling");
  EXPECT_TRUE(delegate.called_on_change_state());
  EXPECT_EQ(accessor.get_auth_status(), AuthStatus::NONE);
  EXPECT_EQ(accessor.get_link_state(), LinkState::CONNECTING);
  EXPECT_FALSE(delegate.called_on_error());
  EXPECT_FALSE(accessor.last_request_had_error());

  // wait for authenticate
  EXPECT_TRUE(helper.eventually(
      [&]() {
        // call `called_on_change_state` for pass mutex
        return delegate.called_on_change_state() && accessor.get_link_state() == LinkState::ONLINE;
      },
      300 * 1000, 100));
  EXPECT_EQ(accessor.get_auth_status(), AuthStatus::SUCCESS);
  EXPECT_TRUE(delegate.called_on_recv_config());
  EXPECT_TRUE(accessor.is_only_one());
  EXPECT_FALSE(accessor.last_request_had_error());

  // wait for disconnect
  delegate.reset();
  scheduler->add_task(nullptr, [&]() {
    accessor.disconnect();
  });

  EXPECT_TRUE(helper.eventually(
      [&]() {
        return delegate.called_on_change_state() && accessor.get_link_state() == LinkState::OFFLINE;
      },
      300 * 1000, 100));
  EXPECT_EQ(accessor.get_auth_status(), AuthStatus::SUCCESS);
  EXPECT_FALSE(accessor.last_request_had_error());
}

TEST(SeedAccessorTest, relay_poll) {
  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  Random random;
  std::unique_ptr<Scheduler> scheduler(Scheduler::new_instance(logger));

  SeedAccessorDelegateStub delegate1;
  SeedAccessor accessor1(
      logger, *scheduler, NodeID::make_random(random), delegate1, SEED_URL, std::string(""), 30 * 1000, true);
  accessor1.tell_online_state(true);

  SeedAccessorDelegateStub delegate2;
  SeedAccessor accessor2(
      logger, *scheduler, NodeID::make_random(random), delegate2, SEED_URL, std::string(""), 30 * 1000, true);
  accessor2.tell_online_state(true);

  scheduler->add_task(nullptr, [&]() {
    accessor1.enable_polling(true);
    accessor2.enable_polling(true);
  });

  // wait for both of accessor will be online
  EXPECT_TRUE(helper.eventually(
      [&]() {
        return delegate1.called_on_change_state() && accessor1.get_link_state() == LinkState::ONLINE &&
               !accessor1.is_only_one() && delegate2.called_on_change_state() &&
               accessor2.get_link_state() == LinkState::ONLINE && !accessor2.is_only_one();
      },
      300 * 1000, 100));

  // relay a packet from accessor1
  NodeID dst_nid = NodeID::make_random(random);
  NodeID src_nid = NodeID::make_random(random);
  uint32_t id    = random.generate_u32();
  scheduler->add_task(nullptr, [&]() {
    std::unique_ptr<proto::PacketContent> p = std::make_unique<proto::PacketContent>();
    proto::Error* e                         = p->mutable_error();
    e->set_code(100);
    e->set_message("test");
    std::unique_ptr<Packet> packet = std::make_unique<Packet>(dst_nid, src_nid, id, std::move(p), PacketMode::NO_RETRY);
    accessor1.relay_packet(std::move(packet));
  });

  // accessor2 should receive the packet
  EXPECT_TRUE(helper.eventually(
      [&]() {
        return delegate2.called_on_recv_packet();
      },
      300 * 1000, 100));
  const Packet* recv = delegate2.packet();
  EXPECT_EQ(recv->dst_nid, dst_nid);
  EXPECT_EQ(recv->src_nid, src_nid);
  EXPECT_EQ(recv->id, id);
  EXPECT_EQ(recv->hop_count, uint32_t(0));
  EXPECT_EQ(recv->mode, PacketMode::NO_RETRY);
  EXPECT_EQ(recv->content->as_proto().error().code(), uint32_t(100));
  EXPECT_EQ(recv->content->as_proto().error().message(), "test");
}

TEST(SeedAccessorTest, detect_error) {
  AsyncHelper helper;
  // seed is not running
  // TestSeed seed;
  // seed.run();

  Random random;
  std::unique_ptr<Scheduler> scheduler(Scheduler::new_instance(logger));

  SeedAccessorDelegateStub delegate;
  SeedAccessor accessor(
      logger, *scheduler, NodeID::make_random(random), delegate, SEED_URL, std::string(""), 30 * 1000, true);

  scheduler->add_task(nullptr, [&]() {
    accessor.enable_polling(true);
  });

  // wait for authenticate
  EXPECT_TRUE(helper.eventually(
      [&]() {
        return accessor.last_request_had_error();
      },
      300 * 1000, 100));
}