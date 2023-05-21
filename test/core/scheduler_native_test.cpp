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

#include <gtest/gtest.h>

#include "test_utils/all.hpp"

using namespace colonio;

TEST(SchedulerNativeTest, add_and_remove) {
  AsyncHelper helper;
  SchedulerNative scheduler(logger);

  void* owner1 = &logger;
  void* owner2 = &scheduler;
  void* owner3 = &helper;

  scheduler.add_task(
      owner1, [] {}, 1000);
  scheduler.add_task(
      owner2, [] {}, 1000);
  scheduler.repeat_task(
      owner2, [] {}, 500);

  scheduler.add_task(owner3, [&] {
    // cannot remove the task because it is running now
    EXPECT_TRUE(scheduler.exists(owner3));
    scheduler.remove(owner3, false);
    EXPECT_TRUE(scheduler.exists(owner3));

    EXPECT_TRUE(scheduler.exists(owner1));
    EXPECT_TRUE(scheduler.exists(owner2));

    // can remove the task because it is not running
    scheduler.remove(owner1);
    EXPECT_FALSE(scheduler.exists(owner1));
    EXPECT_TRUE(scheduler.exists(owner2));

    scheduler.remove(owner2);
    EXPECT_FALSE(scheduler.exists(owner1));
    EXPECT_FALSE(scheduler.exists(owner2));

    scheduler.add_task(owner1, [&] {
      helper.mark("2");
      helper.pass_signal("end");
    });
    scheduler.repeat_task(
        owner1, [] {}, 500);

    helper.mark("1");
  });

  helper.wait_signal("end");
  EXPECT_STREQ("12", helper.get_route().c_str());
}

TEST(SchedulerNativeTest, check_thread_id) {
  AsyncHelper helper;
  SchedulerNative scheduler(logger);
  void* dummy = &helper;

  std::thread::id tid_task;
  std::thread::id tid_loop;

  scheduler.repeat_task(
      dummy,
      [&] {
        tid_loop = std::this_thread::get_id();
        helper.pass_signal("loop");
      },
      100);
  helper.wait_signal("loop");
  scheduler.remove(dummy);

  scheduler.add_task(dummy, [&] {
    tid_task = std::this_thread::get_id();
    helper.pass_signal("task");
  });
  helper.wait_signal("task");

  EXPECT_EQ(tid_task, tid_loop);
  EXPECT_NE(tid_task, std::this_thread::get_id());
}

TEST(SchedulerNativeTest, task_order) {
  AsyncHelper helper;
  SchedulerNative scheduler(logger);
  void* dummy = &helper;

  int count = 0;

  scheduler.add_task(
      dummy,
      [&] {
        count++;
        helper.mark("a");
        helper.pass_signal("a");
      },
      1000);
  scheduler.add_task(dummy, [&] {
    helper.mark("b");
  });
  scheduler.add_task(
      dummy,
      [&] {
        count++;
        helper.mark("c");
      },
      500);

  helper.wait_signal("a");

  EXPECT_STREQ("bca", helper.get_route().c_str());
  EXPECT_EQ(count, 2);

  count = 0;
  scheduler.repeat_task(
      dummy,
      [&] {
        count++;
      },
      100);

  sleep(1);
  EXPECT_LE(6, count);
  EXPECT_LE(count, 12);
}
