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
#include "core/seed_link_native.hpp"

#include <gtest/gtest.h>

#include "core/colonio.pb.h"
#include "core/node_id.hpp"
#include "core/random.hpp"
#include "test_utils/all.hpp"

using namespace colonio;

TEST(SeedLinkNative, post) {
  AsyncHelper helper;
  Random random;
  TestSeed seed;
  seed.run();

  SeedLinkParam param(logger, "https://localhost:8080/test", true);
  SeedLinkNative link(param);

  // post authenticate request and get session id
  std::string session1;

  proto::SeedAuthenticate auth;
  auth.set_version(PROTOCOL_VERSION);
  NodeID::make_random(random).to_pb(auth.mutable_nid());
  std::string auth_bin;
  auth.SerializeToString(&auth_bin);

  link.post("/authenticate", auth_bin, [&](int code, const std::string& bin) {
    EXPECT_EQ(code, 200);

    proto::SeedAuthenticateResponse res;
    if (!res.ParseFromString(bin)) {
      FAIL();
    }
    session1 = res.session_id();

    helper.pass_signal("auth1");
  });
  helper.wait_signal("auth1");

  // post poll request on another thread
  proto::SeedPoll poll;
  poll.set_session_id(session1);
  poll.set_online(true);
  std::string poll_bin;
  poll.SerializeToString(&poll_bin);

  std::thread t([&]() {
    link.post("/poll", poll_bin, [&](int code, const std::string& bin) {
      EXPECT_EQ(code, 200);

      proto::SeedPollResponse res;
      if (!res.ParseFromString(bin)) {
        FAIL();
      }

      helper.pass_signal("poll");
    });
  });

  // make one more session
  std::string session2;
  NodeID::make_random(random).to_pb(auth.mutable_nid());
  auth.SerializeToString(&auth_bin);
  link.post("/authenticate", auth_bin, [&](int code, const std::string& bin) {
    EXPECT_EQ(code, 200);

    proto::SeedAuthenticateResponse res;
    if (!res.ParseFromString(bin)) {
      FAIL();
    }
    session2 = res.session_id();

    helper.pass_signal("auth2");
  });
  helper.wait_signal("auth2");

  // relay a packet
  proto::SeedRelay relay;
  relay.set_session_id(session2);
  proto::SeedPacket* packet = relay.add_packets();
  packet->set_id(10);
  NodeID::make_random(random).to_pb(packet->mutable_dst_nid());
  NodeID::make_random(random).to_pb(packet->mutable_src_nid());
  std::string relay_bin;
  relay.SerializeToString(&relay_bin);
  // packet should be received by `poll`
  link.post("/relay", relay_bin, [&](int code, const std::string& bin) {
    EXPECT_EQ(code, 200);
  });

  // wait to finish
  helper.wait_signal("poll");
  t.join();
}

TEST(SeedLinkNative, invalid_path) {
  AsyncHelper helper;
  Random random;
  TestSeed seed;
  seed.run();

  SeedLinkParam param(logger, "https://localhost:8080/test", true);
  SeedLinkNative link(param);

  std::string dummy;
  link.post("/invalid", dummy, [&](int code, const std::string& bin) {
    EXPECT_NE(code, 200);

    helper.pass_signal("invalid");
  });
  helper.wait_signal("invalid");
}

TEST(SeedLinkNative, invalid_connection) {
  AsyncHelper helper;

  // server is not running
  // TestSeed seed;
  // seed.run();

  SeedLinkParam param(logger, "https://localhost:8080/test", true);
  SeedLinkNative link(param);

  last_log = "";
  std::string dummy;
  link.post("/invalid", dummy, [&](int code, const std::string& bin) {
    EXPECT_EQ(code, 0);
    helper.pass_signal("invalid");
  });
  helper.wait_signal("invalid");
}