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
#include "webrtc_link_wasm.hpp"

#include <emscripten.h>

#include <cassert>
#include <sstream>

#include "logger.hpp"

extern "C" {
typedef unsigned long COLONIO_PTR_T;

extern void webrtc_link_initialize(COLONIO_PTR_T this_ptr, bool is_create_dc);
extern void webrtc_link_finalize(COLONIO_PTR_T this_ptr);
extern void webrtc_link_disconnect(COLONIO_PTR_T this_ptr);
extern void webrtc_link_get_local_sdp(COLONIO_PTR_T this_ptr, bool had_remote_sdp_set);
extern void webrtc_link_send(COLONIO_PTR_T this_ptr, COLONIO_PTR_T data_ptr, int data_siz);
extern void webrtc_link_set_remote_sdp(COLONIO_PTR_T this_ptr, COLONIO_PTR_T sdp_ptr, int sdp_siz, bool is_offer);
extern void webrtc_link_update_ice(COLONIO_PTR_T this_ptr, COLONIO_PTR_T ice_ptr, int ice_siz);

EMSCRIPTEN_KEEPALIVE void webrtc_link_on_csd_failure(COLONIO_PTR_T this_ptr);
EMSCRIPTEN_KEEPALIVE void webrtc_link_on_csd_success(COLONIO_PTR_T this_ptr, COLONIO_PTR_T sdp_ptr, int sdp_siz);
EMSCRIPTEN_KEEPALIVE void webrtc_link_on_dco_close(COLONIO_PTR_T this_ptr);
EMSCRIPTEN_KEEPALIVE void webrtc_link_on_dco_closing(COLONIO_PTR_T this_ptr);
EMSCRIPTEN_KEEPALIVE void webrtc_link_on_dco_error(COLONIO_PTR_T this_ptr, COLONIO_PTR_T message_ptr, int message_siz);
EMSCRIPTEN_KEEPALIVE void webrtc_link_on_dco_message(COLONIO_PTR_T this_ptr, COLONIO_PTR_T data_ptr, int data_siz);
EMSCRIPTEN_KEEPALIVE void webrtc_link_on_dco_open(COLONIO_PTR_T this_ptr);
EMSCRIPTEN_KEEPALIVE void webrtc_link_on_pco_ice_candidate(COLONIO_PTR_T this_ptr, COLONIO_PTR_T ice_ptr, int ice_siz);
EMSCRIPTEN_KEEPALIVE void webrtc_link_on_pco_state_change(
    COLONIO_PTR_T this_ptr, COLONIO_PTR_T state_ptr, int state_siz);
}

void webrtc_link_on_csd_failure(COLONIO_PTR_T this_ptr) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  THIS.on_csd_failure();
}

void webrtc_link_on_csd_success(COLONIO_PTR_T this_ptr, COLONIO_PTR_T sdp_ptr, int sdp_siz) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  std::string sdp(reinterpret_cast<char*>(sdp_ptr), sdp_siz);
  THIS.on_csd_success(sdp);
}

void webrtc_link_on_dco_close(COLONIO_PTR_T this_ptr) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  THIS.delegate.webrtc_link_on_change_dco_state(THIS, colonio::LinkState::OFFLINE);
}

void webrtc_link_on_dco_closing(COLONIO_PTR_T this_ptr) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  THIS.delegate.webrtc_link_on_change_dco_state(THIS, colonio::LinkState::CLOSING);
}

void webrtc_link_on_dco_error(COLONIO_PTR_T this_ptr, COLONIO_PTR_T message_ptr, int message_siz) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  std::string message(reinterpret_cast<char*>(message_ptr), message_siz);
  THIS.on_dco_error(message);
}

void webrtc_link_on_dco_message(COLONIO_PTR_T this_ptr, COLONIO_PTR_T data_ptr, int data_siz) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  std::string data(reinterpret_cast<char*>(data_ptr), data_siz);
  THIS.on_dco_message(data);
}

void webrtc_link_on_dco_open(COLONIO_PTR_T this_ptr) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  THIS.delegate.webrtc_link_on_change_dco_state(THIS, colonio::LinkState::ONLINE);
}

void webrtc_link_on_pco_ice_candidate(COLONIO_PTR_T this_ptr, COLONIO_PTR_T ice_ptr, int ice_siz) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  std::string ice(reinterpret_cast<char*>(ice_ptr), ice_siz);
  THIS.on_pco_ice_candidate(ice);
}

void webrtc_link_on_pco_state_change(COLONIO_PTR_T this_ptr, COLONIO_PTR_T state_ptr, int state_siz) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  std::string state(reinterpret_cast<char*>(state_ptr), state_siz);
  // https://w3c.github.io/webrtc-pc/#rtcicetransportstate
  colonio::LinkState::Type new_state = 0;
  if (state == "new" || state == "checking") {
    assert(THIS.pco_state == colonio::LinkState::CONNECTING);
    new_state = colonio::LinkState::CONNECTING;

  } else if (state == "connected" || state == "completed") {
    assert(THIS.pco_state == colonio::LinkState::CONNECTING || THIS.pco_state == colonio::LinkState::ONLINE);
    new_state = colonio::LinkState::ONLINE;

  } else if (state == "disconnected") {
    assert(THIS.pco_state != colonio::LinkState::OFFLINE);
    new_state = colonio::LinkState::CLOSING;

  } else if (state == "closed" || state == "failed") {
    new_state = colonio::LinkState::OFFLINE;

  } else {
    assert(false);
  }

  THIS.delegate.webrtc_link_on_change_pco_state(THIS, new_state);
}

namespace colonio {
WebrtcLinkWasm::WebrtcLinkWasm(WebrtcLinkParam& param, bool is_create_dc) :
    WebrtcLink(param), is_remote_sdp_set(false) {
#ifndef NDEBUG
  debug_ptr = this;
#endif

  webrtc_link_initialize(reinterpret_cast<COLONIO_PTR_T>(this), is_create_dc);
}

WebrtcLinkWasm::~WebrtcLinkWasm() {
  disconnect();
  webrtc_link_finalize(reinterpret_cast<COLONIO_PTR_T>(this));

#ifndef NDEBUG
  debug_ptr = nullptr;
#endif
}

void WebrtcLinkWasm::on_csd_failure() {
  delegate.webrtc_link_on_error(*this);
}

void WebrtcLinkWasm::on_csd_success(const std::string& sdp) {
  local_sdp = sdp;
  on_get_local_sdp(sdp);
}

void WebrtcLinkWasm::on_dco_error(const std::string& message) {
  logw("dco error").map("message", message);
  delegate.webrtc_link_on_error(*this);
}

void WebrtcLinkWasm::on_dco_message(const std::string& data) {
  delegate.webrtc_link_on_recv_data(*this, data);
}

void WebrtcLinkWasm::on_pco_ice_candidate(const std::string& ice_str) {
  if (ice_str == "") {
    return;
  }

  // Parse ice.
  std::istringstream is(ice_str);
  picojson::value v;
  std::string err = picojson::parse(v, is);
  if (!err.empty()) {
    /// @todo error
    assert(false);
  }

  delegate.webrtc_link_on_update_ice(*this, v.get<picojson::object>());
}

void WebrtcLinkWasm::disconnect() {
  init_data.reset();
  if (dco_state == LinkState::CLOSING || dco_state == LinkState::OFFLINE) {
    return;
  }
  webrtc_link_disconnect(reinterpret_cast<COLONIO_PTR_T>(this));
}

void WebrtcLinkWasm::get_local_sdp(std::function<void(const std::string&)>&& func) {
  on_get_local_sdp = func;

  webrtc_link_get_local_sdp(reinterpret_cast<COLONIO_PTR_T>(this), is_remote_sdp_set);
}

LinkState::Type WebrtcLinkWasm::get_new_link_state() {
  return dco_state;
}

bool WebrtcLinkWasm::send(const std::string& data) {
  if (link_state != LinkState::ONLINE) {
    return false;
  }

  webrtc_link_send(reinterpret_cast<COLONIO_PTR_T>(this), reinterpret_cast<COLONIO_PTR_T>(data.c_str()), data.size());
  return true;
}

void WebrtcLinkWasm::set_remote_sdp(const std::string& sdp) {
  if (link_state == LinkState::CONNECTING || link_state == LinkState::ONLINE) {
    webrtc_link_set_remote_sdp(
        reinterpret_cast<COLONIO_PTR_T>(this), reinterpret_cast<COLONIO_PTR_T>(sdp.c_str()), sdp.size(),
        local_sdp.empty());
    is_remote_sdp_set = true;
  }
}

void WebrtcLinkWasm::update_ice(const picojson::object& ice) {
  if (link_state == LinkState::CONNECTING || link_state == LinkState::ONLINE) {
    std::string ice_str = picojson::value(ice).serialize();
    webrtc_link_update_ice(
        reinterpret_cast<COLONIO_PTR_T>(this), reinterpret_cast<COLONIO_PTR_T>(ice_str.c_str()), ice_str.size());
  }
}
}  // namespace colonio
