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
#pragma once

#include <api/create_peerconnection_factory.h>
#include <picojson.h>
#include <rtc_base/ssl_adapter.h>
#include <rtc_base/thread.h>
#include <system_wrappers/include/field_trial.h>

#include <condition_variable>
#include <mutex>
#include <string>

class WebRTCConfig;
class WebRTCLink;

WebRTCConfig* get_webrtc_config(unsigned int config_id);

class WebRTCConfig {
 public:
  rtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> peer_connection_factory;
  webrtc::PeerConnectionInterface::RTCConfiguration pc_config;

  void setup(const std::string& ice_str);
  void destruct();

 private:
  std::unique_ptr<rtc::Thread> network_thread;
  std::unique_ptr<rtc::Thread> worker_thread;
  std::unique_ptr<rtc::Thread> signaling_thread;
};

class WebRTCLink {
 public:
  bool have_error;

  WebRTCLink(WebRTCConfig& config, unsigned int _id, bool create_data_channel);

  void disconnect();
  std::string& get_local_sdp();
  void set_remote_sdp(const std::string& sdp);
  void update_ice(const picojson::object& ice);
  void send(const void* data, int data_len);

 private:
  class CSDO : public webrtc::CreateSessionDescriptionObserver {
   public:
    explicit CSDO(WebRTCLink& parent_);

    void OnSuccess(webrtc::SessionDescriptionInterface* desc) override;
    void OnFailure(webrtc::RTCError error) override;

   private:
    WebRTCLink& parent;
  };

  class DCO : public webrtc::DataChannelObserver {
   public:
    explicit DCO(WebRTCLink& parent_);

    void OnStateChange() override;
    void OnMessage(const webrtc::DataBuffer& buffer) override;
    void OnBufferedAmountChange(uint64_t previous_amount) override;

   private:
    WebRTCLink& parent;
  };

  class PCO : public webrtc::PeerConnectionObserver {
   public:
    explicit PCO(WebRTCLink& parent_);

    void OnAddStream(rtc::scoped_refptr<webrtc::MediaStreamInterface> stream) override;
    void OnDataChannel(rtc::scoped_refptr<webrtc::DataChannelInterface> data_channel) override;
    void OnIceCandidate(const webrtc::IceCandidateInterface* candidate) override;
    void OnIceConnectionChange(webrtc::PeerConnectionInterface::IceConnectionState new_state) override;
    void OnIceGatheringChange(webrtc::PeerConnectionInterface::IceGatheringState new_state) override;
    void OnRemoveStream(rtc::scoped_refptr<webrtc::MediaStreamInterface> stream) override;
    void OnRenegotiationNeeded() override;
    void OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState new_state) override;

   private:
    WebRTCLink& parent;
  };

  class SSDO : public webrtc::SetSessionDescriptionObserver {
   public:
    explicit SSDO(WebRTCLink& parent_);

    void OnSuccess() override;
    void OnFailure(webrtc::RTCError error) override;

   private:
    WebRTCLink& parent;
  };

  const unsigned int id;
  rtc::scoped_refptr<CSDO> csdo;
  DCO dco;
  PCO pco;
  rtc::scoped_refptr<SSDO> ssdo;

  rtc::scoped_refptr<webrtc::PeerConnectionInterface> peer_connection;
  rtc::scoped_refptr<webrtc::DataChannelInterface> data_channel;

  std::mutex mutex;
  std::condition_variable_any cond;
  bool is_remote_sdp_set;
  /// SDP of local peer.
  std::string local_sdp;
  bool dco_enable;
  bool pco_enable;

  void on_csd_success(webrtc::SessionDescriptionInterface* desc);
  void on_csd_failure(const std::string& error);
  void on_dco_message(const webrtc::DataBuffer& buffer);
  void on_pco_ice_candidate(const webrtc::IceCandidateInterface* candidate);
  void on_ssd_failure(const std::string& error);
};
