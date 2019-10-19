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
#pragma once

#ifndef EMSCRIPTEN
#  error For WebAssembly.
#endif

#include <picojson.h>

#include <deque>
#include <string>

#include "context.hpp"
#include "node_id.hpp"
#include "webrtc_context.hpp"

namespace colonio {
class WebrtcLinkWasm : public WebrtcLinkBase {
 public:
#ifndef NDEBUG
  WebrtcLinkWasm* debug_ptr;
#endif

  WebrtcLinkWasm(WebrtcLinkDelegate& delegate_, Context& context_, WebrtcContext& webrtc_context, bool is_create_dc);
  virtual ~WebrtcLinkWasm();

  void on_csd_failure();
  void on_csd_success(const std::string& sdp);
  void on_dco_close();
  void on_dco_error(const std::string& message);
  void on_dco_message(const std::string& data);
  void on_dco_open();
  void on_pco_ice_candidate(const std::string& ice);
  void on_pco_state_change(const std::string& state);

  void disconnect() override;
  void get_local_sdp(std::function<void(const std::string&)> func) override;
  LinkStatus::Type get_status() override;
  bool send(const std::string& data) override;
  void set_remote_sdp(const std::string& sdp) override;
  void update_ice(const picojson::object& ice) override;

 private:
  bool is_remote_sdp_set;
  /// SDP of local peer.
  std::string local_sdp;
  std::function<void(const std::string&)> on_get_local_sdp;

  LinkStatus::Type prev_status;

  LinkStatus::Type dco_status;
  LinkStatus::Type pco_status;

  std::deque<std::unique_ptr<picojson::object>> ice_que;

  std::deque<std::unique_ptr<std::string>> data_que;

  void on_change_status();
  void on_error();
  void on_ice_candidate();
  void on_recv_data();
};

typedef WebrtcLinkWasm WebrtcLink;
}  // namespace colonio
