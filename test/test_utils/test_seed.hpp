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
#pragma once

#include <gtest/gtest.h>
#include <picojson.h>
#include <signal.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include <fstream>
#include <iostream>
#include <memory>
#include <mutex>
#include <thread>

class TestSeed {
 public:
  TestSeed() :
      flg_running(false),
      flg_break(false),
      pid(0),
      path("/test"),
      ping_interval(20 * 1000),
      port(8080),
      revision(0.1),
      timeout(30 * 1000),
      update_period(500),
      force_update_times(20) {
  }

  virtual ~TestSeed() {
    stop();
  }

  void run() {
#ifndef COLONIO_SEED_BIN_PATH
#  error Need env value COLONIO_SEED_BIN_PATH.
#endif
    std::string config = generate_config();
    pid                = fork();
    if (pid == -1) {
      perror("fork");
      FAIL();

    } else if (pid == 0) {
      // Exec seed program on the child process.
      if (execl(COLONIO_SEED_BIN_PATH, COLONIO_SEED_BIN_PATH, "-verbose", "-config", config.c_str(), nullptr) == -1) {
        perror("execl");
      }
      exit(EXIT_FAILURE);

    } else {
      // Start monitoring thread on the parent(current) prcess.
      flg_running = true;
      // Reduce retry on sign-in by waiting to start the seed.
      sleep(1);

      monitoring_thread = std::make_unique<std::thread>([&]() {
        int status;
        assert(waitpid(pid, &status, 0) == pid);
        assert(WEXITSTATUS(status) == 0);
        {
          std::lock_guard<std::mutex> guard(mtx);
          flg_running = false;
          EXPECT_TRUE(flg_break);
        }
      });
    }
  }

  void set_path(const std::string& p) {
    path = p;
  }

  void set_port(int p) {
    port = p;
  }

  void set_coord_system_sphere(double r) {
    coord_system.insert(std::make_pair("type", picojson::value("sphere")));
    coord_system.insert(std::make_pair("radius", picojson::value(r)));
  }

  void add_module_map_paxos(
      const std::string& name, unsigned int channel, unsigned int retry_interval_min = 200,
      unsigned int retry_interval_max = 300) {
    picojson::object m;
    m.insert(std::make_pair("type", picojson::value("mapPaxos")));
    m.insert(std::make_pair("channel", picojson::value(static_cast<double>(channel))));
    m.insert(std::make_pair("retryIntervalMin", picojson::value(static_cast<double>(retry_interval_min))));
    m.insert(std::make_pair("retryIntervalMax", picojson::value(static_cast<double>(retry_interval_max))));
    modules.insert(std::make_pair(name, picojson::value(m)));
  }

  void stop() {
    {
      std::lock_guard<std::mutex> guard(mtx);
      if (flg_running) {
        flg_break = true;
        kill(pid, SIGTERM);
      }
    }

    if (monitoring_thread) {
      monitoring_thread->join();
    }
  }

 private:
  pid_t pid;
  bool flg_running;
  bool flg_break;
  std::unique_ptr<std::thread> monitoring_thread;
  std::mutex mtx;

  std::string path;
  int ping_interval;
  int port;
  double revision;
  int timeout;

  int update_period;
  int force_update_times;

  picojson::object coord_system;
  picojson::object modules;

  std::string generate_config() {
    std::string tmpdir;
    if (getenv("TMPDIR") == nullptr) {
      tmpdir = "/tmp";
    } else {
      tmpdir = std::string(getenv("TMPDIR"));
    }

    picojson::object config;
    config.insert(std::make_pair("path", picojson::value(path)));
    config.insert(std::make_pair("pingInterval", picojson::value(static_cast<double>(ping_interval))));
    config.insert(std::make_pair("port", picojson::value(static_cast<double>(port))));
    config.insert(std::make_pair("revision", picojson::value(static_cast<double>(revision))));
    config.insert(std::make_pair("timeout", picojson::value(static_cast<double>(timeout))));

    picojson::array urls;
    urls.push_back(picojson::value("stun:stun.l.google.com:19302"));
    picojson::object ice_server;
    ice_server.insert(std::make_pair("urls", picojson::value(urls)));

    picojson::array ice_servers;
    ice_servers.push_back(picojson::value(ice_server));

    picojson::object routing;
    routing.insert(std::make_pair("updatePeriod", picojson::value(static_cast<double>(update_period))));
    routing.insert(std::make_pair("forceUpdateTimes", picojson::value(static_cast<double>(force_update_times))));

    picojson::object node;
    node.insert(std::make_pair("iceServers", picojson::value(ice_servers)));
    node.insert(std::make_pair("routing", picojson::value(routing)));
    if (!coord_system.empty()) {
      node.insert(std::make_pair("coordSystem2D", picojson::value(coord_system)));
    }
    if (!modules.empty()) {
      node.insert(std::make_pair("modules", picojson::value(modules)));
    }
    config.insert(std::make_pair("node", picojson::value(node)));

    std::string fname = tmpdir + "/colonio_test_config.json";
    std::ofstream f(fname);
    f << picojson::value(config).serialize(true);
    f.close();

    return fname;
  }
};
