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
#include <shared_mutex>

#include "webrtc.h"
#include "webrtc.hpp"

static std::string error_message;

static std::shared_mutex links_mtx;
static std::map<unsigned int, std::unique_ptr<WebRTCLink>> links;

static void (*webrtc_link_update_state_cb)(unsigned int, int)              = nullptr;
static void (*webrtc_link_update_ice_cb)(unsigned int, const void*, int)   = nullptr;
static void (*webrtc_link_receive_data_cb)(unsigned int, const void*, int) = nullptr;
static void (*webrtc_link_raise_error_cb)(unsigned int, const char*, int)  = nullptr;

WebRTCLink::CSDO::CSDO(WebRTCLink& parent_) : parent(parent_) {
}

void WebRTCLink::CSDO::OnSuccess(webrtc::SessionDescriptionInterface* desc) {
  parent.on_csd_success(desc);
}

void WebRTCLink::CSDO::OnFailure(webrtc::RTCError error) {
  parent.on_csd_failure(error.message());
}

WebRTCLink::DCO::DCO(WebRTCLink& parent_) : parent(parent_) {
}

void WebRTCLink::DCO::OnStateChange() {
  switch (parent.data_channel->state()) {
    case webrtc::DataChannelInterface::kOpen:  // The DataChannel is ready to send data.
      parent.dco_enable = true;
      break;

    case webrtc::DataChannelInterface::kConnecting:
    case webrtc::DataChannelInterface::kClosing:
    case webrtc::DataChannelInterface::kClosed:
      parent.dco_enable = false;
      break;

    default:
      assert(false);
  }

  if (parent.dco_enable && parent.pco_enable) {
    webrtc_link_update_state_cb(parent.id, 1);
  } else {
    webrtc_link_update_state_cb(parent.id, 0);
  }
}

void WebRTCLink::DCO::OnMessage(const webrtc::DataBuffer& buffer) {
  parent.on_dco_message(buffer);
}

void WebRTCLink::DCO::OnBufferedAmountChange(uint64_t previous_amount) {
}

WebRTCLink::PCO::PCO(WebRTCLink& parent_) : parent(parent_) {
}

void WebRTCLink::PCO::OnAddStream(rtc::scoped_refptr<webrtc::MediaStreamInterface> stream) {
}

void WebRTCLink::PCO::OnDataChannel(rtc::scoped_refptr<webrtc::DataChannelInterface> data_channel) {
  assert(parent.data_channel.get() == nullptr);

  parent.data_channel = data_channel;
  data_channel->RegisterObserver(&parent.dco);
}

void WebRTCLink::PCO::OnIceCandidate(const webrtc::IceCandidateInterface* candidate) {
  parent.on_pco_ice_candidate(candidate);
}

void WebRTCLink::PCO::OnIceConnectionChange(webrtc::PeerConnectionInterface::IceConnectionState raw_state) {
  switch (raw_state) {
    case webrtc::PeerConnectionInterface::kIceConnectionConnected:
    case webrtc::PeerConnectionInterface::kIceConnectionCompleted:
      parent.pco_enable = true;
      break;

    case webrtc::PeerConnectionInterface::kIceConnectionNew:
    case webrtc::PeerConnectionInterface::kIceConnectionChecking:
    case webrtc::PeerConnectionInterface::kIceConnectionDisconnected:
    case webrtc::PeerConnectionInterface::kIceConnectionFailed:
    case webrtc::PeerConnectionInterface::kIceConnectionClosed:
      parent.pco_enable = false;
      break;

    default:
      assert(false);
  }

  if (parent.dco_enable && parent.pco_enable) {
    webrtc_link_update_state_cb(parent.id, 1);
  } else {
    webrtc_link_update_state_cb(parent.id, 0);
  }
}

void WebRTCLink::PCO::OnIceGatheringChange(webrtc::PeerConnectionInterface::IceGatheringState new_state) {
}

void WebRTCLink::PCO::OnRemoveStream(rtc::scoped_refptr<webrtc::MediaStreamInterface> stream) {
}

void WebRTCLink::PCO::OnRenegotiationNeeded() {
}

void WebRTCLink::PCO::OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState new_state) {
}

WebRTCLink::SSDO::SSDO(WebRTCLink& parent_) : parent(parent_) {
}

void WebRTCLink::SSDO::OnSuccess() {
}

void WebRTCLink::SSDO::OnFailure(webrtc::RTCError error) {
  parent.on_ssd_failure(error.message());
}

WebRTCLink::WebRTCLink(WebRTCConfig& config, unsigned int _id, bool create_data_channel) :
    have_error(false),
    id(_id),
    csdo(new rtc::RefCountedObject<CSDO>(*this)),
    dco(*this),
    pco(*this),
    ssdo(new rtc::RefCountedObject<SSDO>(*this)),
    is_remote_sdp_set(false),
    dco_enable(false),
    pco_enable(false) {
  webrtc::PeerConnectionDependencies pc_dependencies(&pco);
  auto error_or_peer_connection =
      config.peer_connection_factory->CreatePeerConnectionOrError(config.pc_config, std::move(pc_dependencies));
  if (error_or_peer_connection.ok()) {
    peer_connection = std::move(error_or_peer_connection.value());
  } else {
    error_message = "failed to create peer connection.";
    have_error    = true;
    return;
  }

  // return if skip creating data channel.
  if (!create_data_channel) {
    return;
  }
  webrtc::DataChannelInit dc_config;
  auto error_or_data_channel = peer_connection->CreateDataChannelOrError("data_channel", &dc_config);
  if (error_or_data_channel.ok()) {
    data_channel = std::move(error_or_data_channel.value());
  } else {
    error_message = "failed to create data channel.";
    have_error    = true;
    return;
  }

  data_channel->RegisterObserver(&dco);
}

void WebRTCLink::disconnect() {
  if (peer_connection != nullptr) {
    peer_connection->Close();
  }
}

std::string& WebRTCLink::get_local_sdp() {
  if (!local_sdp.empty() || dco_enable || pco_enable) {
    error_message = "invalid state on get_local_sdp.";
    have_error    = true;
    return local_sdp;
  }

  if (is_remote_sdp_set) {
    peer_connection->CreateAnswer(csdo.get(), webrtc::PeerConnectionInterface::RTCOfferAnswerOptions());
  } else {
    peer_connection->CreateOffer(csdo.get(), webrtc::PeerConnectionInterface::RTCOfferAnswerOptions());
  }

  {
    std::lock_guard<std::mutex> guard(mutex);
    while (local_sdp.empty()) {
      cond.wait(mutex);
    }
  }

  return local_sdp;
}

void WebRTCLink::set_remote_sdp(const std::string& sdp) {
  if (dco_enable || pco_enable) {
    error_message = "invalid state on set_remote_sdp.";
    have_error    = true;
    return;
  }

  webrtc::SdpParseError error;
  webrtc::SdpType sdp_type = local_sdp.empty() ? webrtc::SdpType::kOffer : webrtc::SdpType::kAnswer;
  std::unique_ptr<webrtc::SessionDescriptionInterface> session_description =
      webrtc::CreateSessionDescription(sdp_type, sdp, &error);

  if (session_description == nullptr) {
    error_message = error.description;
    have_error    = true;
    return;
  }

  peer_connection->SetRemoteDescription(ssdo.get(), session_description.release());
  is_remote_sdp_set = true;
}

void WebRTCLink::update_ice(const picojson::object& ice) {
  if (dco_enable && pco_enable) {
    return;
  }

  webrtc::SdpParseError err_sdp;
  std::unique_ptr<webrtc::IceCandidateInterface> ice_ptr(webrtc::CreateIceCandidate(
      ice.at("sdpMid").get<std::string>(), static_cast<int>(ice.at("sdpMLineIndex").get<double>()),
      ice.at("candidate").get<std::string>(), &err_sdp));
  if (!err_sdp.line.empty() && !err_sdp.description.empty()) {
    error_message = err_sdp.description;
    have_error    = true;
    return;
  }

  peer_connection->AddIceCandidate(ice_ptr.get());
}

void WebRTCLink::send(const void* data, int data_len) {
  if (!dco_enable || !pco_enable) {
    error_message = "should be connected.";
    have_error    = true;
    return;
  }

  webrtc::DataBuffer buffer(rtc::CopyOnWriteBuffer(reinterpret_cast<const char*>(data), size_t(data_len)), true);
  bool result = data_channel->Send(buffer);
  if (!result) {
    error_message = "failed to send data.";
    have_error    = true;
  }
}

/**
 * Get SDP string and store.
 */
void WebRTCLink::on_csd_success(webrtc::SessionDescriptionInterface* desc) {
  peer_connection->SetLocalDescription(ssdo.get(), desc);
  {
    std::lock_guard<std::mutex> guard(mutex);
    desc->ToString(&local_sdp);
  }
  cond.notify_all();
}

void WebRTCLink::on_csd_failure(const std::string& error) {
  webrtc_link_raise_error_cb(id, error.c_str(), error.size());
}

/**
 * When receive message, raise event for delegate or store data if delegate has not set.
 * @param buffer Received data buffer.
 */
void WebRTCLink::on_dco_message(const webrtc::DataBuffer& buffer) {
  webrtc_link_receive_data_cb(id, buffer.data.data<char>(), buffer.size());
}

/**
 * Get ICE string and raise event.
 */
void WebRTCLink::on_pco_ice_candidate(const webrtc::IceCandidateInterface* candidate) {
  picojson::object ice;
  std::string candidate_str;
  candidate->ToString(&candidate_str);
  ice.insert(std::make_pair("candidate", picojson::value(candidate_str)));
  ice.insert(std::make_pair("sdpMid", picojson::value(candidate->sdp_mid())));
  ice.insert(std::make_pair("sdpMLineIndex", picojson::value(static_cast<double>(candidate->sdp_mline_index()))));

  std::string ice_str = picojson::value(ice).serialize();
  webrtc_link_update_ice_cb(id, ice_str.c_str(), ice_str.size());
}

void WebRTCLink::on_ssd_failure(const std::string& error) {
  webrtc_link_raise_error_cb(id, error.c_str(), error.size());
}

void webrtc_link_get_error_message(const char** message, int* message_len) {
  *message     = error_message.c_str();
  *message_len = error_message.size();
}

void webrtc_link_init(
    void (*update_state_cb)(unsigned int, int), void (*update_ice_cb)(unsigned int, const void*, int),
    void (*receive_data_cb)(unsigned int, const void*, int), void (*error_cb)(unsigned int, const char*, int)) {
  webrtc_link_update_state_cb = update_state_cb;
  webrtc_link_update_ice_cb   = update_ice_cb;
  webrtc_link_receive_data_cb = receive_data_cb;
  webrtc_link_raise_error_cb  = error_cb;
}

unsigned int webrtc_link_new(unsigned int config_id, int create_data_channel) {
  WebRTCConfig* config = get_webrtc_config(config_id);
  if (config == nullptr) {
    error_message = "invalid config id on webrtc_link_new.";
    return 0;
  }

  std::lock_guard<std::shared_mutex> guard(links_mtx);

  unsigned int id = 1;
  while (links.find(id) != links.end()) {
    ++id;
  }
  std::unique_ptr<WebRTCLink> link(new WebRTCLink(*config, id, create_data_channel != 0));
  if (link->have_error) {
    return 0;
  }
  links[id] = std::move(link);
  return id;
}

int webrtc_link_disconnect(unsigned int id) {
  std::unique_ptr<WebRTCLink> link;
  {
    std::lock_guard<std::shared_mutex> guard(links_mtx);
    auto it = links.find(id);
    if (it == links.end()) {
      error_message = "invalid id on webrtc_link_disconnect.";
      return -1;
    }
    link = std::move(it->second);
    links.erase(it);
  }

  link->disconnect();
  if (link->have_error) {
    return -1;
  }
  return 0;
}

int webrtc_link_get_local_sdp(unsigned int id, const char** sdp, int* sdp_len) {
  WebRTCLink* link;
  {
    std::shared_lock<std::shared_mutex> guard(links_mtx);
    auto it = links.find(id);
    if (it == links.end()) {
      error_message = "invalid id on webrtc_link_get_local_sdp.";
      return -1;
    }
    link = it->second.get();
  }

  std::string& sdp_str = link->get_local_sdp();
  if (link->have_error) {
    return -1;
  }
  *sdp     = sdp_str.c_str();
  *sdp_len = sdp_str.size();
  return 0;
}

int webrtc_link_set_remote_sdp(unsigned int id, const char* sdp, int sdp_len) {
  WebRTCLink* link;
  {
    std::shared_lock<std::shared_mutex> guard(links_mtx);
    auto it = links.find(id);
    if (it == links.end()) {
      error_message = "invalid id on webrtc_link_set_remote_sdp.";
      return -1;
    }
    link = it->second.get();
  }

  link->set_remote_sdp(std::string(sdp, sdp_len));
  if (link->have_error) {
    return -1;
  }
  return 0;
}

int webrtc_link_update_ice(unsigned int id, const char* ice, int ice_len) {
  WebRTCLink* link;
  {
    std::shared_lock<std::shared_mutex> guard(links_mtx);
    auto it = links.find(id);
    if (it == links.end()) {
      error_message = "invalid id on webrtc_link_update_ice.";
      return -1;
    }
    link = it->second.get();
  }

  picojson::value i;
  std::string err = picojson::parse(i, ice);
  if (!err.empty()) {
    error_message = err;
    return -1;
  }
  link->update_ice(i.get<picojson::object>());
  if (link->have_error) {
    return -1;
  }
  return 0;
}

int webrtc_link_send(unsigned int id, const void* data, int data_len) {
  WebRTCLink* link;
  {
    std::shared_lock<std::shared_mutex> guard(links_mtx);
    auto it = links.find(id);
    if (it == links.end()) {
      error_message = "invalid id on webrtc_link_send.";
      return -1;
    }
    link = it->second.get();
  }

  link->send(data, data_len);
  if (link->have_error) {
    return -1;
  }
  return 0;
}