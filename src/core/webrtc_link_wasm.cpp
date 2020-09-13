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
#include <emscripten.h>

#include <cassert>
#include <sstream>

#include "logger.hpp"
#include "scheduler.hpp"
#include "webrtc_link.hpp"

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

  THIS.on_dco_close();
}

void webrtc_link_on_dco_closing(COLONIO_PTR_T this_ptr) {
  colonio::WebrtcLinkWasm& THIS = *reinterpret_cast<colonio::WebrtcLinkWasm*>(this_ptr);
  assert(THIS.debug_ptr == &THIS);

  THIS.on_dco_closing();
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

  THIS.on_dco_open();
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
  THIS.on_pco_state_change(state);
}

namespace colonio {
WebrtcLinkWasm::WebrtcLinkWasm(
    WebrtcLinkDelegate& delegate_, Context& context_, WebrtcContext& webrtc_context_, bool is_create_dc) :
    WebrtcLinkBase(delegate_, context_, webrtc_context_),
    is_remote_sdp_set(false),
    prev_status(LinkStatus::CONNECTING),
    dco_status(LinkStatus::CONNECTING),
    pco_status(LinkStatus::CONNECTING) {
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
  context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkWasm::on_error, this), 0);
}

void WebrtcLinkWasm::on_csd_success(const std::string& sdp) {
  local_sdp = sdp;
  on_get_local_sdp(sdp);
}

void WebrtcLinkWasm::on_dco_close() {
  if (dco_status != LinkStatus::OFFLINE) {
    dco_status = LinkStatus::OFFLINE;
    context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkWasm::on_change_status, this), 0);
  }
}

void WebrtcLinkWasm::on_dco_closing() {
  if (dco_status != LinkStatus::CLOSING) {
    dco_status = LinkStatus::CLOSING;
    context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkWasm::on_change_status, this), 0);
  }
}

void WebrtcLinkWasm::on_dco_error(const std::string& message) {
  logw("dco error").map("message", message);
  context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkWasm::on_error, this), 0);
}

void WebrtcLinkWasm::on_dco_message(const std::string& data) {
  data_que.push_back(std::make_unique<std::string>(data));
  context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkWasm::on_recv_data, this), 0);
}

void WebrtcLinkWasm::on_dco_open() {
  if (dco_status != LinkStatus::ONLINE) {
    dco_status = LinkStatus::ONLINE;
    context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkWasm::on_change_status, this), 0);
  }
}

void WebrtcLinkWasm::on_pco_ice_candidate(const std::string& ice_str) {
  if (ice_str != "") {
    // Parse ice.
    std::istringstream is(ice_str);
    picojson::value v;
    std::string err = picojson::parse(v, is);
    if (!err.empty()) {
      /// @todo error
      assert(false);
    }

    std::unique_ptr<picojson::object> ice = std::make_unique<picojson::object>(v.get<picojson::object>());

    ice_que.push_back(std::move(ice));

  } else {
    context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkWasm::on_ice_candidate, this), 0);
  }
}

void WebrtcLinkWasm::on_pco_state_change(const std::string& state) {
  // https://w3c.github.io/webrtc-pc/#rtcicetransportstate
  LinkStatus::Type should;
  if (state == "new" || state == "checking") {
    assert(pco_status == LinkStatus::CONNECTING);
    should = LinkStatus::CONNECTING;

  } else if (state == "connected" || state == "completed") {
    assert(pco_status == LinkStatus::CONNECTING || pco_status == LinkStatus::ONLINE);
    should = LinkStatus::ONLINE;

  } else if (state == "disconnected") {
    assert(pco_status != LinkStatus::OFFLINE);
    should = LinkStatus::CLOSING;

  } else if (state == "closed" || state == "failed") {
    should = LinkStatus::OFFLINE;

  } else {
    assert(false);
  }

  if (should != pco_status) {
    pco_status = should;
    context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkWasm::on_change_status, this), 0);
  }
}

void WebrtcLinkWasm::disconnect() {
  init_data.reset();
  webrtc_link_disconnect(reinterpret_cast<COLONIO_PTR_T>(this));
}

void WebrtcLinkWasm::get_local_sdp(std::function<void(const std::string&)> func) {
  on_get_local_sdp = func;

  webrtc_link_get_local_sdp(reinterpret_cast<COLONIO_PTR_T>(this), is_remote_sdp_set);
}

LinkStatus::Type WebrtcLinkWasm::get_status() {
  if (init_data) {
    return LinkStatus::CONNECTING;
  }

  return dco_status;
}

bool WebrtcLinkWasm::send(const std::string& data) {
  if (get_status() == LinkStatus::ONLINE) {
    webrtc_link_send(reinterpret_cast<COLONIO_PTR_T>(this), reinterpret_cast<COLONIO_PTR_T>(data.c_str()), data.size());
    return true;

  } else {
    return false;
  }
}

void WebrtcLinkWasm::set_remote_sdp(const std::string& sdp) {
  LinkStatus::Type status = get_status();
  if (status == LinkStatus::CONNECTING || status == LinkStatus::ONLINE) {
    webrtc_link_set_remote_sdp(
        reinterpret_cast<COLONIO_PTR_T>(this), reinterpret_cast<COLONIO_PTR_T>(sdp.c_str()), sdp.size(),
        local_sdp.empty());
    is_remote_sdp_set = true;
  }
}

void WebrtcLinkWasm::update_ice(const picojson::object& ice) {
  LinkStatus::Type status = get_status();
  if (status == LinkStatus::CONNECTING || status == LinkStatus::ONLINE) {
    std::string ice_str = picojson::value(ice).serialize();
    webrtc_link_update_ice(
        reinterpret_cast<COLONIO_PTR_T>(this), reinterpret_cast<COLONIO_PTR_T>(ice_str.c_str()), ice_str.size());
  }
}

void WebrtcLinkWasm::on_change_status() {
  if (init_data && dco_status == LinkStatus::ONLINE && pco_status == LinkStatus::ONLINE) {
    init_data.reset();

  } else if ((dco_status == LinkStatus::OFFLINE || pco_status == LinkStatus::OFFLINE) && dco_status != pco_status) {
    disconnect();
  }

  LinkStatus::Type status = get_status();

  if (status != prev_status) {
    prev_status = status;
    delegate.webrtc_link_on_change_status(*this, status);
  }
}

void WebrtcLinkWasm::on_error() {
  delegate.webrtc_link_on_error(*this);
}

void WebrtcLinkWasm::on_ice_candidate() {
  while (true) {
    std::unique_ptr<picojson::object> ice;
    if (ice_que.size() == 0) {
      break;
    } else {
      ice = std::move(ice_que[0]);
      ice_que.pop_front();
    }

    delegate.webrtc_link_on_update_ice(*this, *ice);
  }
}

void WebrtcLinkWasm::on_recv_data() {
  while (true) {
    std::unique_ptr<std::string> data;
    if (data_que.size() == 0) {
      break;
    } else {
      data = std::move(data_que[0]);
      data_que.pop_front();
    }

    delegate.webrtc_link_on_recv_data(*this, *data);
  }
}
}  // namespace colonio
