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
#include "core/user_thread_pool.hpp"

#include <gtest/gtest.h>

#include "test_utils/all.hpp"

using namespace colonio;

class UserThreadPoolTest : public ::testing::Test {
 public:
  size_t get_threads_count(UserThreadPool& utp) {
    return utp.threads.size();
  }

  size_t get_free_threads_count(UserThreadPool& utp) {
    return utp.free_threads.size();
  }
};

TEST_F(UserThreadPoolTest, task_run_on_each_thread) {
  UserThreadPool utp(logger, 2, 1000);
  AsyncHelper helper;

  std::thread::id tid1;
  std::thread::id tid2;

  EXPECT_EQ(static_cast<size_t>(0), get_threads_count(utp));
  EXPECT_EQ(static_cast<size_t>(0), get_free_threads_count(utp));

  utp.push([&] {
    tid1 = std::this_thread::get_id();
    helper.pass_signal("task1");
    helper.wait_signal("fin");
  });

  EXPECT_EQ(static_cast<size_t>(1), get_threads_count(utp));
  EXPECT_EQ(static_cast<size_t>(0), get_free_threads_count(utp));

  utp.push([&] {
    tid2 = std::this_thread::get_id();
    helper.pass_signal("task2");
    helper.wait_signal("fin");
  });

  helper.wait_signal("task1");
  helper.wait_signal("task2");

  EXPECT_EQ(static_cast<size_t>(2), get_threads_count(utp));
  EXPECT_EQ(static_cast<size_t>(0), get_free_threads_count(utp));

  helper.pass_signal("fin");
  // each task should be run on different threads.
  EXPECT_NE(tid1, tid2);

  for (int i = 0; i < 10; i++) {
    if (get_threads_count(utp) == 2 && get_free_threads_count(utp) == 2) {
      break;
    }
    std::this_thread::sleep_for(std::chrono::microseconds(100));
  }
  EXPECT_EQ(static_cast<size_t>(2), get_threads_count(utp));
  EXPECT_EQ(static_cast<size_t>(2), get_free_threads_count(utp));
}

TEST_F(UserThreadPoolTest, thread_could_be_reuse) {
  UserThreadPool utp(logger, 1, 1000);
  AsyncHelper helper;

  std::thread::id tid1;
  std::thread::id tid2;

  utp.push([&] {
    tid1 = std::this_thread::get_id();
    helper.pass_signal("task1");
    helper.wait_signal("next");
  });

  std::thread th([&] {
    utp.push([&] {
      tid2 = std::this_thread::get_id();
      helper.pass_signal("task2");
    });
    helper.pass_signal("fin_push");
  });

  helper.wait_signal("task1");

  // task2 should be run after task1
  EXPECT_FALSE(helper.check_signal("fin_push"));
  EXPECT_EQ(static_cast<size_t>(1), get_threads_count(utp));
  EXPECT_EQ(static_cast<size_t>(0), get_free_threads_count(utp));
  helper.pass_signal("next");

  helper.wait_signal("task2");
  // task run on the same thread.
  EXPECT_EQ(tid1, tid2);
  th.join();

  for (int i = 0; i < 10; i++) {
    if (get_threads_count(utp) == 1 && get_free_threads_count(utp) == 1) {
      break;
    }
    std::this_thread::sleep_for(std::chrono::microseconds(100));
  }
  EXPECT_EQ(static_cast<size_t>(1), get_threads_count(utp));
  EXPECT_EQ(static_cast<size_t>(1), get_free_threads_count(utp));
}

TEST_F(UserThreadPoolTest, task_can_be_pass_in_task) {
  UserThreadPool utp(logger, 2, 1000);
  AsyncHelper helper;

  utp.push([&] {
    EXPECT_EQ(static_cast<size_t>(1), get_threads_count(utp));
    EXPECT_EQ(static_cast<size_t>(0), get_free_threads_count(utp));

    utp.push([&] {
      EXPECT_EQ(static_cast<size_t>(2), get_threads_count(utp));

      helper.pass_signal("nest");
    });
  });

  helper.wait_signal("nest");
}