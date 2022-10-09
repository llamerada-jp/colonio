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

#ifndef EMSCRIPTEN
#  error For WebAssembly.
#endif

#ifdef __clang__
#  include <picojson.h>
#else
#  pragma GCC diagnostic push
#  pragma GCC diagnostic ignored "-Wmaybe-uninitialized"
#  include <picojson.h>
#  pragma GCC diagnostic pop
#endif

#include <deque>
#include <functional>
#include <memory>
#include <string>

#include "webrtc_link.hpp"

namespace colonio {
class WebrtcLinkWasm : public WebrtcLink {
 public:
#ifndef NDEBUG
  WebrtcLinkWasm* debug_ptr;
#endif

  WebrtcLinkWasm(WebrtcLinkParam& param, bool is_create_dc);
  virtual ~WebrtcLinkWasm();

  void on_csd_failure();
  void on_csd_success(const std::string& sdp);
  void on_dco_error(const std::string& message);
  void on_dco_message(const std::string& data);
  void on_pco_ice_candidate(const std::string& ice);
  void on_pco_state_change(const std::string& state);

  void disconnect() override;
  void get_local_sdp(std::function<void(const std::string&)>&& func) override;
  LinkState::Type get_new_link_state() override;
  bool send(const std::string& data) override;
  void set_remote_sdp(const std::string& sdp) override;
  void update_ice(const picojson::object& ice) override;

 private:
  bool is_remote_sdp_set;
  /// SDP of local peer.
  std::string local_sdp;
  std::function<void(const std::string&)> on_get_local_sdp;
};
}  // namespace colonio
