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
#include "core/scheduler_native.hpp"

#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "colonio/colonio.hpp"
#include "test_utils/all.hpp"

using namespace colonio;
using ::testing::MatchesRegex;

TEST(SchedulerNativeTest, add_and_remove) {
  AsyncHelper helper;
  SchedulerNative scheduler(logger, 0);

  void* pointer1 = &logger;
  void* pointer2 = &scheduler;

  scheduler.add_controller_task(
      pointer1, [] {}, 1000);
  scheduler.add_controller_task(
      pointer2, [] {}, 1000);
  scheduler.add_controller_loop(
      pointer2, [] {}, 500);
  scheduler.add_user_task(pointer1, [] {
    sleep(1);
  });

  scheduler.add_controller_task(&helper, [&] {
    EXPECT_TRUE(scheduler.has_task(&helper));
    scheduler.remove_task(&helper, false);
    EXPECT_TRUE(scheduler.has_task(&helper));

    EXPECT_TRUE(scheduler.has_task(pointer1));
    EXPECT_TRUE(scheduler.has_task(pointer2));

    scheduler.remove_task(pointer1);
    EXPECT_FALSE(scheduler.has_task(pointer1));
    EXPECT_TRUE(scheduler.has_task(pointer2));

    scheduler.remove_task(pointer2);
    EXPECT_FALSE(scheduler.has_task(pointer1));
    EXPECT_FALSE(scheduler.has_task(pointer2));

    scheduler.add_controller_task(pointer1, [] {});
    scheduler.add_user_task(pointer1, [&] {
      scheduler.add_controller_task(pointer1, [] {});
    });
    scheduler.add_controller_loop(
        pointer1, [] {}, 500);

    helper.pass_signal("end");
  });

  helper.wait_signal("end");
}

TEST(SchedulerNativeTest, thread1) {
  AsyncHelper helper;
  SchedulerNative scheduler(logger, 0);
  void* dummy = &helper;

  std::thread::id tid_controller;
  std::thread::id tid_loop;
  std::thread::id tid_user;

  scheduler.add_controller_loop(
      dummy,
      [&] {
        tid_loop = std::this_thread::get_id();
        helper.pass_signal("loop");
      },
      100);
  helper.wait_signal("loop");
  scheduler.remove_task(dummy);

  scheduler.add_controller_task(dummy, [&] {
    tid_controller = std::this_thread::get_id();
    helper.pass_signal("controller");
  });
  helper.wait_signal("controller");

  scheduler.add_user_task(dummy, [&] {
    tid_user = std::this_thread::get_id();
    helper.pass_signal("user");
  });
  helper.wait_signal("user");

  EXPECT_EQ(tid_controller, tid_loop);
  EXPECT_NE(tid_user, tid_controller);
  EXPECT_NE(tid_controller, std::this_thread::get_id());
  EXPECT_NE(tid_user, std::this_thread::get_id());
}

TEST(SchedulerNativeTest, thread2) {
  AsyncHelper helper;
  void* dummy = &helper;

  std::thread::id tid_controller1;
  std::thread::id tid_controller2;
  std::thread::id tid_user1;
  std::thread::id tid_user2;

  std::unique_ptr<std::thread> th_controller;
  std::unique_ptr<std::thread> th_user;
  {
    SchedulerNative scheduler(logger, Colonio::EXPLICIT_EVENT_THREAD | Colonio::EXPLICIT_CONTROLLER_THREAD);

    th_controller.reset(new std::thread([&] {
      tid_controller1 = std::this_thread::get_id();
      scheduler.start_controller_routine();
    }));

    th_user.reset(new std::thread([&] {
      tid_user1 = std::this_thread::get_id();
      scheduler.start_user_routine();
    }));

    scheduler.add_controller_task(dummy, [&] {
      tid_controller2 = std::this_thread::get_id();
      helper.pass_signal("controller");
    });

    scheduler.add_user_task(dummy, [&] {
      tid_user2 = std::this_thread::get_id();
      helper.pass_signal("user");
    });

    helper.wait_signal("controller");
    helper.wait_signal("user");
  }

  th_controller->join();
  th_user->join();

  EXPECT_EQ(tid_controller1, tid_controller2);
  EXPECT_EQ(tid_user1, tid_user2);
  EXPECT_NE(tid_controller1, tid_user1);
}

TEST(SchedulerNativeTest, task) {
  AsyncHelper helper;
  SchedulerNative scheduler(logger, 0);
  void* dummy = &helper;

  int count = 0;

  scheduler.add_controller_task(
      dummy,
      [&] {
        count++;
        helper.mark("a");
        helper.pass_signal("a");
      },
      200);
  scheduler.add_controller_task(dummy, [&] {
    helper.mark("b");
  });
  scheduler.add_controller_task(
      dummy,
      [&] {
        count++;
        helper.mark("c");
      },
      50);
  scheduler.add_user_task(dummy, [&] {
    helper.mark("d");
  });

  helper.wait_signal("a");

  EXPECT_THAT(helper.get_route(), MatchesRegex("^bdca|dbca$"));
  EXPECT_EQ(count, 2);

  count = 0;
  scheduler.add_controller_loop(
      dummy,
      [&] {
        count++;
      },
      100);

  sleep(1);
  EXPECT_LE(6, count);
  EXPECT_LE(count, 12);
}
