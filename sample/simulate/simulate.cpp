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

#ifndef NDEBUG
#  include <glog/logging.h>
#endif

#include <hiredis/hiredis.h>
#include <picojson.h>
#include <unistd.h>
#include <uv.h>

#include <cassert>
#include <cstdio>
#include <cstdlib>
#include <ctime>
#include <random>
#include <sstream>
#include <map>
#include <mutex>

#include "colonio/colonio.hpp"

static const std::string SERVER_URL = "http://localdev:8080/ws/simulate";
static const std::string REDIS_HOST = "localdev";
static const int REDIS_PORT         = 6379;

std::random_device rd;
std::mt19937 mt(rd());
std::uniform_real_distribution<double> rand_lon(-180.0, 180.0);
std::uniform_real_distribution<double> rand_lat(-90.0, 90.0);
double lon;
double lat;
double target_lon;
double target_lat;

static redisContext* rc;
bool is_online = false;
colonio::Map* map;
std::string now_str;
bool is_running  = true;
int64_t val_set1 = 0;
int64_t val_set2 = 0;

std::mutex mtx_debug;
std::map<std::string, std::string> debug_infos;

#define redis_command(cmd, ...)                                                        \
  {                                                                                    \
    redisReply* r = reinterpret_cast<redisReply*>(redisCommand(rc, cmd, __VA_ARGS__)); \
    if (r != nullptr) {                                                                \
      freeReplyObject(r);                                                              \
    }                                                                                  \
  }

void on_debug_event(colonio::DebugEvent::Type type, const std::string& json_str);

void on_timer(uv_timer_t* handle);
void on_map_set();
void on_map_set_failure(const colonio::Exception& reason);

class MyColonio : public colonio::Colonio {
 public:
  void on_output_log(colonio::LogLevel::Type level, const std::string& message) override {
    if (level == colonio::LogLevel::INFO) {
      std::cout << message << std::endl;
    } else {
      std::cerr << message << std::endl;
    }
  }

  void on_debug_event(colonio::DebugEvent::Type event, const std::string& json_str) override {
    ::on_debug_event(event, json_str);
  }
};
std::unique_ptr<MyColonio> my_colonio;

static const char* DB_KEY[] = {"map_set",    "links",      "nexts",   "position",
                               "required1d", "required2d", "known1d", "known2d"};

void on_debug_event(colonio::DebugEvent::Type type, const std::string& json_str) {
  assert(type <= 7);
  std::lock_guard<std::mutex> lock(mtx_debug);
  debug_infos[DB_KEY[type]] = json_str;
}

void on_timer(uv_timer_t* handle) {
  std::string local_nid = my_colonio->get_local_nid();
  {
    std::lock_guard<std::mutex> lock(mtx_debug);
    for (auto& it : debug_infos) {
      redis_command("HSET %s %s %s", it.first.c_str(), local_nid.c_str(), it.second.c_str());
    }
    debug_infos.clear();
  }

  time_t rawtime = time(nullptr);
  tm* timeinfo   = localtime(&rawtime);
  char buffer[80];

  strftime(buffer, sizeof(buffer), "%Y/%m/%d %I:%M:%S", timeinfo);
  now_str = std::string(buffer);

  if (is_running) {
    return;
  }

  is_running = true;
  val_set1++;
  std::cout << now_str << " map set:" << val_set1 << std::endl;
  assert(false);

  try {
    map->set(colonio::Value(my_colonio->get_local_nid()), colonio::Value(val_set1));
    on_map_set();
  } catch (const colonio::Exception& ex) {
    on_map_set_failure(ex);
  }

  //*
  if (mt() % 100 == 0) {
    target_lon = rand_lon(mt);
    target_lat = rand_lat(mt);
  }
  if (lon < target_lon) lon += 1;
  if (lon > target_lon) lon -= 1;
  if (lat < target_lat) lat += 1;
  if (lat > target_lat) lat -= 1;
  my_colonio->set_position(M_PI * lon / 180.0, M_PI * lat / 180.0);
  //*/
}

void on_map_set() {
  std::cout << now_str << " map set success:" << val_set1 << std::endl;
  val_set2 = val_set1;
  std::cout << now_str << " map get" << std::endl;
  assert(false);
  try {
    colonio::Value value = map->get(colonio::Value(my_colonio->get_local_nid()));
    int64_t val          = value.get<int64_t>();

    if (val == val_set2) {
      std::cout << now_str << " map get success:" << val << std::endl;
    } else {
      std::cout << now_str << " map get wrong:" << val << " != " << val_set2 << std::endl;
    }

    is_running = false;
  } catch (const colonio::Exception& ex) {
    std::cout << now_str << " map get failure:" << ex.message << " : " << static_cast<int>(ex.code) << std::endl;
    is_running = false;
  }
}

void on_map_set_failure(const colonio::Exception& ex) {
  std::cout << now_str << " map set failure:" << static_cast<int>(ex.code) << std::endl;
  std::cout << now_str << " map set:" << val_set1 << std::endl;
  is_running = false;
  // assert(false);
}

int main(int argc, char* argv[]) {
#ifndef NDEBUG
  google::InitGoogleLogging(argv[0]);
  google::InstallFailureSignalHandler();
#endif

  // Libuv
  uv_loop_t* loop = uv_default_loop();

  // redis
  rc = redisConnect(REDIS_HOST.c_str(), REDIS_PORT);

  // colonio
  try {
    my_colonio = std::make_unique<MyColonio>();
    my_colonio->connect(SERVER_URL, "");
    redis_command("HSET inits %s %s", std::to_string(getpid()).c_str(), my_colonio->get_local_nid().c_str());
    is_online = true;
    map       = &(my_colonio->access_map("map"));

    is_running = false;

    lon        = rand_lon(mt);
    lat        = rand_lat(mt);
    target_lon = rand_lon(mt);
    target_lat = rand_lat(mt);
    my_colonio->set_position(M_PI * lon / 180.0, M_PI * lat / 180.0);

  } catch (const colonio::Exception& ex) {
    my_colonio->disconnect();
    is_online = false;
  }

  // libuv for timer
  is_running = true;
  uv_timer_t timer;
  uv_timer_init(loop, &timer);
  uv_timer_start(&timer, on_timer, 0, 1000);

  // loop
  uv_run(loop, UV_RUN_DEFAULT);

  // quit
  my_colonio.reset();
  redisFree(rc);

  return EXIT_SUCCESS;
}
