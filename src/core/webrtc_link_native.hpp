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
#pragma once

#ifdef EMSCRIPTEN
#  error For native.
#endif

#include <api/peer_connection_interface.h>
#include <picojson.h>

#include <atomic>
#include <condition_variable>
#include <deque>
#include <mutex>
#include <string>
#include <thread>

#include "webrtc_context.hpp"

namespace colonio {
class Context;

class WebrtcLinkNative : public WebrtcLinkBase {
 public:
  WebrtcLinkNative(WebrtcLinkDelegate& delegate_, Context& context_, WebrtcContext& webrtc_context, bool is_create_dc);
  virtual ~WebrtcLinkNative();

  void disconnect() override;
  void get_local_sdp(std::function<void(const std::string&)> func) override;
  LinkStatus::Type get_status() override;
  bool send(const std::string& data) override;
  void set_remote_sdp(const std::string& sdp) override;
  void update_ice(const picojson::object& ice) override;

 private:
  class CSDO : public webrtc::CreateSessionDescriptionObserver {
   public:
    explicit CSDO(WebrtcLinkNative& parent_);

    void OnSuccess(webrtc::SessionDescriptionInterface* desc) override;
    void OnFailure(const std::string& error) override;

   private:
    WebrtcLinkNative& parent;
  };

  class DCO : public webrtc::DataChannelObserver {
   public:
    explicit DCO(WebrtcLinkNative& parent_);

    void OnStateChange() override;
    void OnMessage(const webrtc::DataBuffer& buffer) override;
    void OnBufferedAmountChange(uint64_t previous_amount) override;

   private:
    WebrtcLinkNative& parent;
  };

  class PCO : public webrtc::PeerConnectionObserver {
   public:
    explicit PCO(WebrtcLinkNative& parent_);

    void OnAddStream(rtc::scoped_refptr<webrtc::MediaStreamInterface> stream) override;
    void OnDataChannel(rtc::scoped_refptr<webrtc::DataChannelInterface> data_channel) override;
    void OnIceCandidate(const webrtc::IceCandidateInterface* candidate) override;
    void OnIceConnectionChange(webrtc::PeerConnectionInterface::IceConnectionState new_state) override;
    void OnIceGatheringChange(webrtc::PeerConnectionInterface::IceGatheringState new_state) override;
    void OnRemoveStream(rtc::scoped_refptr<webrtc::MediaStreamInterface> stream) override;
    void OnRenegotiationNeeded() override;
    void OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState new_state) override;

   private:
    WebrtcLinkNative& parent;
  };

  class SSDO : public webrtc::SetSessionDescriptionObserver {
   public:
    explicit SSDO(WebrtcLinkNative& parent_);

    void OnSuccess() override;
    void OnFailure(const std::string& error) override;

   private:
    WebrtcLinkNative& parent;
  };

  rtc::scoped_refptr<CSDO> csdo;
  DCO dco;
  PCO pco;
  rtc::scoped_refptr<SSDO> ssdo;

  rtc::scoped_refptr<webrtc::PeerConnectionInterface> peer_connection;
  rtc::scoped_refptr<webrtc::DataChannelInterface> data_channel;
  webrtc::DataChannelInit dc_config;

  std::mutex mutex;
  std::condition_variable_any cond;
  bool is_remote_sdp_set;
  /// SDP of local peer.
  std::string local_sdp;

  LinkStatus::Type prev_status;

  std::mutex mutex_status;
  LinkStatus::Type dco_status;
  LinkStatus::Type pco_status;

  std::mutex mutex_ice;
  std::deque<std::unique_ptr<picojson::object>> ice_que;

  std::mutex mutex_data;
  std::deque<std::unique_ptr<std::string>> data_que;

  void on_change_status();
  void on_error();
  void on_ice_candidate();
  void on_recv_data();

  void on_csd_success(webrtc::SessionDescriptionInterface* desc);
  void on_csd_failure(const std::string& error);
  void on_dco_message(const webrtc::DataBuffer& buffer);
  void on_dco_state_change(webrtc::DataChannelInterface::DataState status);
  void on_pco_connection_change(webrtc::PeerConnectionInterface::IceConnectionState status);
  void on_pco_ice_candidate(const webrtc::IceCandidateInterface* candidate);
  void on_ssd_failure(const std::string& error);
};

typedef WebrtcLinkNative WebrtcLink;
}  // namespace colonio
