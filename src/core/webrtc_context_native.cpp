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
#include <cassert>
#include <iostream>

#include <api/audio_codecs/builtin_audio_decoder_factory.h>
#include <api/audio_codecs/builtin_audio_encoder_factory.h>
#include <api/video_codecs/builtin_video_decoder_factory.h>
#include <api/video_codecs/builtin_video_encoder_factory.h>
#include <rtc_base/ssladapter.h>

#include "webrtc_context.hpp"
#include "utils.hpp"

namespace colonio {

WebrtcContextNative::WebrtcContextNative() {
}

WebrtcContextNative::~WebrtcContextNative() {
  peer_connection_factory = nullptr;

  // Subthread will quit after release resources.
  if (network_thread) {
    network_thread->Stop();
  }
  if (worker_thread) {
    worker_thread->Stop();
  }
  if (signaling_thread) {
    signaling_thread->Stop();
  }

  rtc::CleanupSSL();
}

void WebrtcContextNative::initialize(const picojson::array& ice_servers) {
  for (auto& it : ice_servers) {
    const picojson::object& ice_server = it.get<picojson::object>();
    webrtc::PeerConnectionInterface::IceServer entry;
    if (ice_server.at("urls").is<std::string>()) {
      entry.urls.push_back(ice_server.at("urls").get<std::string>());

    } else if (ice_server.at("urls").is<picojson::array>()) {
      for (auto& url : ice_server.at("urls").get<picojson::array>()) {
        entry.urls.push_back(url.get<std::string>());
      }

    } else {
      // @todo error
      assert(false);
    }

    Utils::check_json_optional(ice_server, "username", &entry.username);
    Utils::check_json_optional(ice_server, "credential", &entry.password);

    pc_config.servers.push_back(entry);
  }

  rtc::InitializeSSL();

  network_thread = rtc::Thread::CreateWithSocketServer();
  network_thread->Start();
  worker_thread = rtc::Thread::Create();
  worker_thread->Start();
  signaling_thread = rtc::Thread::Create();
  signaling_thread->Start();

  peer_connection_factory = webrtc::CreatePeerConnectionFactory(
      network_thread.get(),
      worker_thread.get(),
      signaling_thread.get(),
      nullptr /* default_adm */,
      webrtc::CreateBuiltinAudioEncoderFactory(),
      webrtc::CreateBuiltinAudioDecoderFactory(),
      webrtc::CreateBuiltinVideoEncoderFactory(),
      webrtc::CreateBuiltinVideoDecoderFactory(),
      nullptr /* audio_mixer */,
      nullptr /* audio_processing */);

  if (peer_connection_factory.get() == nullptr) {
    // @todo error
    assert(false);
  }
}
}  // namespace colonio
