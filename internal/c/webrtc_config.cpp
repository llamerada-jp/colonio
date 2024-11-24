/*
 * Copyright 2017- Yuji Ito <llamerada.jp@gmail.com>
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

#include <map>
#include <memory>
#include <mutex>

#include "webrtc.h"
#include "webrtc.hpp"

static std::mutex configs_mtx;
static std::map<unsigned int, std::unique_ptr<WebRTCConfig>> configs;

void WebRTCConfig::setup(const std::string& ice_str) {
  picojson::value ice_v;
  std::string err = picojson::parse(ice_v, ice_str);
  if (!err.empty()) {
    return;
  }

  if (ice_v.is<picojson::array>()) {
    picojson::array ice_servers = ice_v.get<picojson::array>();
    for (auto& it : ice_servers) {
      const picojson::object& ice_server = it.get<picojson::object>();
      webrtc::PeerConnectionInterface::IceServer entry;
      assert(ice_server.at("urls").is<picojson::array>());
      for (auto& url : ice_server.at("urls").get<picojson::array>()) {
        entry.urls.push_back(url.get<std::string>());
      }
      if (ice_server.find("username") != ice_server.end()) {
        entry.username = ice_server.at("username").get<std::string>();
      }
      if (ice_server.find("credential") != ice_server.end()) {
        entry.password = ice_server.at("credential").get<std::string>();
      }

      pc_config.servers.push_back(entry);
    }
  }

  network_thread = rtc::Thread::CreateWithSocketServer();
  network_thread->Start();
  worker_thread = rtc::Thread::Create();
  worker_thread->Start();
  signaling_thread = rtc::Thread::Create();
  signaling_thread->Start();

  webrtc::PeerConnectionFactoryDependencies dependencies;
  dependencies.network_thread   = network_thread.get();
  dependencies.worker_thread    = worker_thread.get();
  dependencies.signaling_thread = signaling_thread.get();
  peer_connection_factory       = webrtc::CreateModularPeerConnectionFactory(std::move(dependencies));

  if (peer_connection_factory.get() == nullptr) {
    // @todo error
    assert(false);
  }
}

void WebRTCConfig::destruct() {
  peer_connection_factory = nullptr;

  // Sub-thread will quit after release resources.
  network_thread->Stop();
  worker_thread->Stop();
  signaling_thread->Stop();
}

WebRTCConfig* get_webrtc_config(unsigned int config_id) {
  std::lock_guard<std::mutex> lock(configs_mtx);
  auto it = configs.find(config_id);
  if (it != configs.end()) {
    return it->second.get();
  }
  return nullptr;
}

void webrtc_config_init() {
  webrtc::field_trial::InitFieldTrialsFromString("");
  rtc::InitializeSSL();
}

unsigned int webrtc_config_new(const char* ice, int ice_len) {
  std::string ice_str(ice, ice_len);
  std::unique_ptr<WebRTCConfig> config(new WebRTCConfig());
  config->setup(ice_str);
  std::lock_guard<std::mutex> lock(configs_mtx);
  unsigned int id = 0;
  while (configs.find(id) != configs.end()) {
    ++id;
  }
  configs[id] = std::move(config);
  return id;
}

void webrtc_config_destruct(unsigned int id) {
  std::lock_guard<std::mutex> lock(configs_mtx);
  auto it = configs.find(id);
  if (it != configs.end()) {
    it->second->destruct();
    configs.erase(it);
  }
}
