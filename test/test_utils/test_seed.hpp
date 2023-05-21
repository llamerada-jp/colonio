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

#ifdef __clang__
#  include <picojson.h>
#else
#  pragma GCC diagnostic push
#  pragma GCC diagnostic ignored "-Wmaybe-uninitialized"
#  include <picojson.h>
#  pragma GCC diagnostic pop
#endif

#include <gtest/gtest.h>
#include <signal.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include <fstream>
#include <iostream>
#include <memory>
#include <thread>

class TestSeed {
 public:
  TestSeed() :
      pid(0),
      path("/test"),
      port(8080),
      revision(0.1),
      keep_alive_timeout(30 * 1000),
      polling_timeout(10 * 1000),
      update_period(500),
      force_update_times(20) {
  }

  virtual ~TestSeed() {
    stop();
  }

  void run() {
    std::string bin_path(getenv("COLONIO_SEED_BIN_PATH"));
    if (bin_path.empty()) {
      printf("env var 'COLONIO_SEED_BIN_PATH' should be specify");
      FAIL();
    }

    std::string config = generate_config();
    pid                = fork();
    if (pid == -1) {
      perror("fork");
      FAIL();

    } else if (pid == 0) {
      // Exec seed program on the child process.
      if (execl(bin_path.c_str(), bin_path.c_str(), "--config", config.c_str(), nullptr) == -1) {
        perror("execl");
      }
      exit(EXIT_FAILURE);
    }
    // Reduce retry on sign-in by waiting to start the seed.
    std::string url = "https://localhost:" + std::to_string(port);
    std::string cmd = "curl --insecure -s " + url + " > /dev/null";
    while (true) {
      int ret = system(cmd.c_str());

      if (!WIFEXITED(ret)) {
        printf("curl command exit without normal termination");
        exit(EXIT_FAILURE);
      }

      if (WEXITSTATUS(ret) == 0) {
        break;
      }

      std::this_thread::sleep_for(std::chrono::seconds(3));
    }
  }

  void set_path(const std::string& p) {
    path = p;
  }

  void set_port(int p) {
    port = p;
  }

  void set_coord_system_sphere(double r = 6378137.0) {
    coord_system.insert(std::make_pair("type", picojson::value("sphere")));
    coord_system.insert(std::make_pair("radius", picojson::value(r)));
  }

  void set_coord_system_plane(double x_min = -1.0, double x_max = 1.0, double y_min = -1.0, double y_max = 1.0) {
    coord_system.insert(std::make_pair("type", picojson::value("plane")));
    coord_system.insert(std::make_pair("xMin", picojson::value(x_min)));
    coord_system.insert(std::make_pair("xMax", picojson::value(x_max)));
    coord_system.insert(std::make_pair("yMin", picojson::value(y_min)));
    coord_system.insert(std::make_pair("yMax", picojson::value(y_max)));
  }

  void config_kvs(unsigned int retry_interval_min = 200, unsigned int retry_interval_max = 300) {
    picojson::object m;
    m.insert(std::make_pair("retryIntervalMin", picojson::value(static_cast<double>(retry_interval_min))));
    m.insert(std::make_pair("retryIntervalMax", picojson::value(static_cast<double>(retry_interval_max))));
    modules.insert(std::make_pair("kvs", picojson::value(m)));
  }

  void config_spread(unsigned int cache_time = 30000) {
    picojson::object m;
    m.insert(std::make_pair("cacheTime", picojson::value(static_cast<double>(cache_time))));
    modules.insert(std::make_pair("spread", picojson::value(m)));
  }

  void stop() {
    kill(pid, SIGTERM);
    int status;
    assert(waitpid(pid, &status, 0) == pid);
    assert(WEXITSTATUS(status) == 0);
  }

 private:
  pid_t pid;

  std::string path;
  int port;
  double revision;
  int keep_alive_timeout;
  int polling_timeout;

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
    config.insert(std::make_pair("port", picojson::value(static_cast<double>(port))));
    config.insert(std::make_pair("revision", picojson::value(static_cast<double>(revision))));
    config.insert(std::make_pair("keepAliveTimeout", picojson::value(static_cast<double>(keep_alive_timeout))));
    config.insert(std::make_pair("pollingTimeout", picojson::value(static_cast<double>(polling_timeout))));
    config.insert(std::make_pair("useTcp", picojson::value(true)));

    std::string cert_file(getenv("COLONIO_TEST_CERT"));
    std::string key_file(getenv("COLONIO_TEST_PRIVATE_KEY"));
    if (cert_file.empty() || key_file.empty()) {
      printf(
          "cert and private key file should be specify by 'COLONIO_TEST_CERT' and 'COLONIO_TEST_PRIVATE_KEY' env var.");
      exit(1);
    }
    config.insert(std::make_pair("certFile", picojson::value(cert_file)));
    config.insert(std::make_pair("keyFile", picojson::value(key_file)));

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
