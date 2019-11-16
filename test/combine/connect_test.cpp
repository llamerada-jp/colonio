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

#include <glog/logging.h>
#include <gtest/gtest.h>

#include "colonio/colonio_libuv.hpp"
#include "test_utils/async_helper.hpp"
#include "test_utils/test_seed.hpp"

using namespace colonio;
using namespace colonio_helper;

TEST(ConnectTest, connectSingle) {
  AsyncHelper helper;
  TestSeed seed;
  seed.run();

  ColonioLibuv node(helper.get_libuv_instance());

  node.connect(
      "http://localhost:8080/test", "",
      [&](Colonio& c) {
        helper.mark("a");
        c.disconnect();
      },
      [&](Colonio& c) {
        ADD_FAILURE();
        c.disconnect();
      });

  // node.run();
  helper.run();
  helper.check_route("a");
}

int main(int argc, char** argv) {
  google::InitGoogleLogging(argv[0]);
  google::InstallFailureSignalHandler();

  CHECK_EQ(RUN_ALL_TESTS(), 0);
}