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
#include <emscripten.h>
#include <emscripten/bind.h>

#include <cassert>
#include <string>

#include "webrtc_context.hpp"

extern "C" {
typedef unsigned long COLONIO_PTR_T;

extern void webrtc_context_initialize();
extern void webrtc_context_add_ice_server(COLONIO_PTR_T str_ptr, int str_siz);
}

namespace colonio {
WebrtcContextWasm::WebrtcContextWasm() {
  // Do nothing.
}

WebrtcContextWasm::~WebrtcContextWasm() {
  // Do nothing.
}

void WebrtcContextWasm::initialize(const picojson::array& ice_servers) {
  webrtc_context_initialize();

  for (auto& ice_server : ice_servers) {
    std::string ice_server_str = ice_server.serialize();
    webrtc_context_add_ice_server(reinterpret_cast<COLONIO_PTR_T>(ice_server_str.c_str()), ice_server_str.size());
  }
}
}  // namespace colonio
