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

#ifdef EMSCRIPTEN
#  error For native.
#endif

#include <api/create_peerconnection_factory.h>
#include <rtc_base/thread.h>

#include "webrtc_context.hpp"

namespace colonio {
class WebrtcContextNative : public WebrtcContext {
 public:
  rtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> peer_connection_factory;
  webrtc::PeerConnectionInterface::RTCConfiguration pc_config;

  WebrtcContextNative();
  virtual ~WebrtcContextNative();

  void initialize(const picojson::array& ice_servers);

 private:
  std::unique_ptr<rtc::Thread> network_thread;
  std::unique_ptr<rtc::Thread> worker_thread;
  std::unique_ptr<rtc::Thread> signaling_thread;
};
}  // namespace colonio
