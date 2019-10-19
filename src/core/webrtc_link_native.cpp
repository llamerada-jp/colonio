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
#include <picojson.h>

#include <cassert>
#include <string>

#include "context.hpp"
#include "convert.hpp"
#include "webrtc_link.hpp"

namespace colonio {
WebrtcLinkNative::CSDO::CSDO(WebrtcLink& parent_) : parent(parent_) {
}

void WebrtcLinkNative::CSDO::OnSuccess(webrtc::SessionDescriptionInterface* desc) {
  parent.on_csd_success(desc);
}

void WebrtcLinkNative::CSDO::OnFailure(const std::string& error) {
  parent.on_csd_failure(error);
}

WebrtcLinkNative::DCO::DCO(WebrtcLink& parent_) : parent(parent_) {
}

void WebrtcLinkNative::DCO::OnStateChange() {
  parent.on_dco_state_change(parent.data_channel->state());
}

void WebrtcLinkNative::DCO::OnMessage(const webrtc::DataBuffer& buffer) {
  parent.on_dco_message(buffer);
}

void WebrtcLinkNative::DCO::OnBufferedAmountChange(uint64_t previous_amount) {
}

WebrtcLinkNative::PCO::PCO(WebrtcLink& parent_) : parent(parent_) {
}

void WebrtcLinkNative::PCO::OnAddStream(rtc::scoped_refptr<webrtc::MediaStreamInterface> stream) {
}

void WebrtcLinkNative::PCO::OnDataChannel(rtc::scoped_refptr<webrtc::DataChannelInterface> data_channel) {
  assert(parent.data_channel.get() == nullptr);

  parent.data_channel = data_channel;
  data_channel->RegisterObserver(&parent.dco);
}

void WebrtcLinkNative::PCO::OnIceCandidate(const webrtc::IceCandidateInterface* candidate) {
  parent.on_pco_ice_candidate(candidate);
}

void WebrtcLinkNative::PCO::OnIceConnectionChange(webrtc::PeerConnectionInterface::IceConnectionState new_state) {
  parent.on_pco_connection_change(new_state);
}

void WebrtcLinkNative::PCO::OnIceGatheringChange(webrtc::PeerConnectionInterface::IceGatheringState new_state) {
}

void WebrtcLinkNative::PCO::OnRemoveStream(rtc::scoped_refptr<webrtc::MediaStreamInterface> stream) {
}

void WebrtcLinkNative::PCO::OnRenegotiationNeeded() {
}

void WebrtcLinkNative::PCO::OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState new_state) {
}

WebrtcLinkNative::SSDO::SSDO(WebrtcLink& parent_) : parent(parent_) {
}

void WebrtcLinkNative::SSDO::OnSuccess() {
}

void WebrtcLinkNative::SSDO::OnFailure(const std::string& error) {
  parent.on_ssd_failure(error);
}

WebrtcLinkNative::WebrtcLinkNative(
    WebrtcLinkDelegate& delegate_, Context& context_, WebrtcContext& webrtc_context, bool is_create_dc) :
    WebrtcLinkBase(delegate_, context_, webrtc_context),
    csdo(new rtc::RefCountedObject<CSDO>(*this)),
    dco(*this),
    pco(*this),
    ssdo(new rtc::RefCountedObject<SSDO>(*this)),
    is_remote_sdp_set(false),
    prev_status(LinkStatus::CONNECTING),
    dco_status(LinkStatus::CONNECTING),
    pco_status(LinkStatus::CONNECTING) {
  peer_connection =
      webrtc_context.peer_connection_factory->CreatePeerConnection(webrtc_context.pc_config, nullptr, nullptr, &pco);

  if (peer_connection.get() == nullptr) {
    /// @todo error
    assert(false);
  }

  if (is_create_dc) {
    data_channel = peer_connection->CreateDataChannel("data_channel", &webrtc_context.dc_config);
    if (data_channel.get() == nullptr) {
      /// @todo error
      assert(false);
    }

    data_channel->RegisterObserver(&dco);
  }
}

/**
 * Destractor, close peer connection and release.
 */
WebrtcLinkNative::~WebrtcLinkNative() {
  assert(get_status() == LinkStatus::OFFLINE);
  context.scheduler.remove_task(this);
}

void WebrtcLinkNative::disconnect() {
  logd("Disconnect.(nid= %s)", nid.to_str().c_str());

  init_data.reset();
  if (peer_connection != nullptr) {
    peer_connection->Close();
  }
}

/**
 * Get new SDP of peer.
 * @todo Relase SDP string?
 */
void WebrtcLinkNative::get_local_sdp(std::function<void(const std::string&)> func) {
  assert(local_sdp.empty());
  assert(get_status() == LinkStatus::CONNECTING);

  if (is_remote_sdp_set) {
    peer_connection->CreateAnswer(csdo, webrtc::PeerConnectionInterface::RTCOfferAnswerOptions());
  } else {
    peer_connection->CreateOffer(csdo, webrtc::PeerConnectionInterface::RTCOfferAnswerOptions());
  }

  {
    std::lock_guard<std::mutex> guard(mutex);
    while (local_sdp.empty()) {
      cond.wait(mutex);
    }
  }

  func(local_sdp);
}

LinkStatus::Type WebrtcLinkNative::get_status() {
  if (init_data) {
    return LinkStatus::CONNECTING;

  } else {
    LinkStatus::Type dco_status;
    LinkStatus::Type pco_status;
    {
      std::lock_guard<std::mutex> guard(mutex_status);
      dco_status = this->dco_status;
      pco_status = this->pco_status;
    }

    if (dco_status == LinkStatus::ONLINE && pco_status == LinkStatus::ONLINE) {
      return LinkStatus::ONLINE;

    } else if (dco_status == LinkStatus::OFFLINE && pco_status == LinkStatus::OFFLINE) {
      return LinkStatus::OFFLINE;

    } else {
      return LinkStatus::CLOSING;
    }
  }
}

/**
 * Send packet by WebRTC data channel.
 * @param packet Packet to send.
 */
bool WebrtcLinkNative::send(const std::string& packet) {
  if (get_status() == LinkStatus::ONLINE) {
    webrtc::DataBuffer buffer(rtc::CopyOnWriteBuffer(packet.c_str(), packet.size()), true);
    data_channel->Send(buffer);
    return true;

  } else {
    return false;
  }
}

/**
 * Set remote peer's SDP.
 * @param sdp String of sdp.
 */
void WebrtcLinkNative::set_remote_sdp(const std::string& sdp) {
  LinkStatus::Type status = get_status();
  if (status == LinkStatus::CONNECTING || status == LinkStatus::ONLINE) {
    webrtc::SdpParseError error;
    webrtc::SessionDescriptionInterface* session_description(
        webrtc::CreateSessionDescription((local_sdp.empty() ? "offer" : "answer"), sdp, &error));

    if (session_description == nullptr) {
      /// @todo error
      std::cout << error.line << std::endl;
      std::cout << error.description << std::endl;
      assert(false);
    }

    peer_connection->SetRemoteDescription(ssdo, session_description);
    is_remote_sdp_set = true;
  }
}

/**
 * Update ICE data.
 * @param ice String of ice.
 */
void WebrtcLinkNative::update_ice(const picojson::object& ice) {
  LinkStatus::Type status = get_status();
  if (status == LinkStatus::CONNECTING || status == LinkStatus::ONLINE) {
    webrtc::SdpParseError err_sdp;
    webrtc::IceCandidateInterface* ice_ptr = CreateIceCandidate(
        ice.at("sdpMid").get<std::string>(), static_cast<int>(ice.at("sdpMLineIndex").get<double>()),
        ice.at("candidate").get<std::string>(), &err_sdp);
    if (!err_sdp.line.empty() && !err_sdp.description.empty()) {
      /// @todo error
      std::cout << "Error on CreateIceCandidate" << std::endl
                << err_sdp.line << std::endl
                << err_sdp.description << std::endl;
      assert(false);
    }

    peer_connection->AddIceCandidate(ice_ptr);
  }
}

void WebrtcLinkNative::on_change_status() {
  {
    LinkStatus::Type dco_status;
    LinkStatus::Type pco_status;
    {
      std::lock_guard<std::mutex> guard(mutex_status);
      dco_status = this->dco_status;
      pco_status = this->pco_status;
    }

    if (init_data && dco_status == LinkStatus::ONLINE && pco_status == LinkStatus::ONLINE) {
      init_data.reset();

    } else if ((dco_status == LinkStatus::OFFLINE || pco_status == LinkStatus::OFFLINE) && dco_status != pco_status) {
      disconnect();

    } else if (dco_status == LinkStatus::OFFLINE && pco_status == LinkStatus::OFFLINE) {
      peer_connection = nullptr;
      data_channel    = nullptr;
    }
  }

  LinkStatus::Type status = get_status();
  logd("Change status.(nid=%s, %d -> %d)", nid.to_str().c_str(), prev_status, status);

  if (status != prev_status) {
    prev_status = status;
    delegate.webrtc_link_on_change_stateus(*this, status);
  }
}

void WebrtcLinkNative::on_error() {
  delegate.webrtc_link_on_error(*this);
}

void WebrtcLinkNative::on_ice_candidate() {
  while (true) {
    std::unique_ptr<picojson::object> ice;
    {
      std::lock_guard<std::mutex> guard(mutex_ice);
      if (ice_que.size() == 0) {
        break;
      } else {
        ice = std::move(ice_que[0]);
        ice_que.pop_front();
      }
    }

    delegate.webrtc_link_on_update_ice(*this, *ice);
  }
}

void WebrtcLinkNative::on_recv_data() {
  while (true) {
    std::unique_ptr<std::string> data;
    {
      std::lock_guard<std::mutex> guard(mutex_data);
      if (data_que.size() == 0) {
        break;
      } else {
        data = std::move(data_que[0]);
        data_que.pop_front();
      }
    }

    delegate.webrtc_link_on_recv_data(*this, *data);
  }
}

/**
 * Get SDP string and store.
 */
void WebrtcLinkNative::on_csd_success(webrtc::SessionDescriptionInterface* desc) {
  peer_connection->SetLocalDescription(ssdo, desc);

  std::lock_guard<std::mutex> guard(mutex);
  cond.notify_all();
  desc->ToString(&local_sdp);
}

void WebrtcLinkNative::on_csd_failure(const std::string& error) {
  // @todo Output error log.
  std::cerr << error << std::endl;
  context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkNative::on_error, this), 0);
}

/**
 * When receive message, raise event for delegate or store data if delegate has not set.
 * @param buffer Received data buffer.
 */
void WebrtcLinkNative::on_dco_message(const webrtc::DataBuffer& buffer) {
  std::unique_ptr<std::string> data = std::make_unique<std::string>(buffer.data.data<char>(), buffer.size());
  // Can't receive message when OPENING, but there is a possibility to receive message when CLOSEING or CLOSED.
  assert(dco_status != LinkStatus::CONNECTING);
  assert(pco_status != LinkStatus::CONNECTING);

  std::lock_guard<std::mutex> guard(mutex_data);
  data_que.push_back(std::move(data));

  context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkNative::on_recv_data, this), 0);
}

/**
 * Raise status chagne event by data channel status.
 * @param status Data channel status.
 */
void WebrtcLinkNative::on_dco_state_change(webrtc::DataChannelInterface::DataState status) {
  LinkStatus::Type should;
  switch (status) {
    case webrtc::DataChannelInterface::kConnecting:
      should = LinkStatus::CONNECTING;
      break;

    case webrtc::DataChannelInterface::kOpen:  // The DataChannel is ready to send data.
      should = LinkStatus::ONLINE;
      break;

    case webrtc::DataChannelInterface::kClosing:
      should = LinkStatus::CLOSING;
      break;

    case webrtc::DataChannelInterface::kClosed:
      should = LinkStatus::OFFLINE;
      break;

    default:
      assert(false);
  }

  bool is_changed = false;
  {
    std::lock_guard<std::mutex> guard(mutex_status);
    if (should != dco_status) {
      dco_status = should;
      is_changed = true;
    }
  }

  if (is_changed) {
    context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkNative::on_change_status, this), 0);
  }
}

/**
 * Raise status change event by peer connection status.
 * @param status Peer connection status.
 */
void WebrtcLinkNative::on_pco_connection_change(webrtc::PeerConnectionInterface::IceConnectionState status) {
  LinkStatus::Type should;
  switch (status) {
    case webrtc::PeerConnectionInterface::kIceConnectionNew:
    case webrtc::PeerConnectionInterface::kIceConnectionChecking:
      should = LinkStatus::CONNECTING;
      break;

    case webrtc::PeerConnectionInterface::kIceConnectionConnected:
    case webrtc::PeerConnectionInterface::kIceConnectionCompleted:
      should = LinkStatus::ONLINE;
      break;

    case webrtc::PeerConnectionInterface::kIceConnectionDisconnected:
      should = LinkStatus::CLOSING;
      break;

    case webrtc::PeerConnectionInterface::kIceConnectionFailed:
    case webrtc::PeerConnectionInterface::kIceConnectionClosed:
      should = LinkStatus::OFFLINE;
      break;

    default:
      assert(false);
  }

  bool is_changed = false;
  {
    std::lock_guard<std::mutex> guard(mutex_status);
    if (should != pco_status) {
      pco_status = should;
      is_changed = true;
    }
  }

  if (is_changed) {
    context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkNative::on_change_status, this), 0);
  }
}

/**
 * Get ICE string and raise event.
 */
void WebrtcLinkNative::on_pco_ice_candidate(const webrtc::IceCandidateInterface* candidate) {
  std::unique_ptr<picojson::object> ice = std::make_unique<picojson::object>();
  std::string candidate_str;
  candidate->ToString(&candidate_str);
  ice->insert(std::make_pair("candidate", picojson::value(candidate_str)));
  ice->insert(std::make_pair("sdpMid", picojson::value(candidate->sdp_mid())));
  ice->insert(std::make_pair("sdpMLineIndex", picojson::value(static_cast<double>(candidate->sdp_mline_index()))));

  {
    std::lock_guard<std::mutex> guard(mutex_ice);
    ice_que.push_back(std::move(ice));
  }

  context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkNative::on_ice_candidate, this), 0);
}

void WebrtcLinkNative::on_ssd_failure(const std::string& error) {
  /// @todo Output error log.
  std::cerr << error << std::endl;
  context.scheduler.add_timeout_task(this, std::bind(&WebrtcLinkNative::on_error, this), 0);
}
}  // namespace colonio
