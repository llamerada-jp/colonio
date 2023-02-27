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
#include "node_accessor.hpp"

#include <unistd.h>

#include <cassert>
#include <memory>
#include <string>

#include "colonio.pb.h"
#include "command_manager.hpp"
#include "logger.hpp"
#include "scheduler.hpp"
#include "utils.hpp"
#include "webrtc_context.hpp"

namespace colonio {
NodeAccessorDelegate::~NodeAccessorDelegate() {
}

NodeAccessor::NodeAccessor(Logger& l, Scheduler& s, CommandManager& c, const NodeID& n, NodeAccessorDelegate& d) :
    logger(l),
    scheduler(s),
    command_manager(c),
    local_nid(n),
    delegate(d),
    first_link(nullptr),
    first_link_try_count(0),
    random_link(nullptr),
    count_seed_translate(0),
    is_updated_line_links(false) {
  command_manager.set_handler(
      proto::PacketContent::kSignalingOffer, std::bind(&NodeAccessor::recv_offer, this, std::placeholders::_1));
  command_manager.set_handler(
      proto::PacketContent::kSignalingIce, std::bind(&NodeAccessor::recv_ice, this, std::placeholders::_1));

  scheduler.repeat_task(
      this,
      [this]() {
        check_link_disconnect();
        check_link_timeout();
      },
      1000);
}

NodeAccessor::~NodeAccessor() {
  scheduler.remove(this);
  links.clear();
}

void NodeAccessor::connect_link(const NodeID& nid) {
  if (first_link != nullptr) {
    log_debug("canceled to connect link until connect first link").map("nid", nid);
  }

  auto it = nid_links.find(nid);
  if (it != nid_links.end()) {
    switch (it->second->link_state) {
      case LinkState::CONNECTING:
      case LinkState::ONLINE:
        return;

      case LinkState::CLOSING:
      case LinkState::OFFLINE: {
        disconnect_link(it->second);
      } break;

      default: {
        assert(false);
      } break;
    }
  }

  log_debug("connect a link").map("nid", nid);

  WebrtcLink* link          = create_link(true);
  link->init_data->is_prime = true;
  assign_link_nid(nid, link);

  send_offer(link, local_nid, nid, OFFER_TYPE_NORMAL);
}

void NodeAccessor::connect_init_link() {
  assert(get_link_state() == LinkState::OFFLINE);
  assert(first_link == nullptr);
  assert(random_link == nullptr);
  assert(nid_links.empty());

  log_debug("connect init link").map("local_nid", local_nid);

  first_link_try_count = 0;
  create_first_link();
}

void NodeAccessor::connect_random_link() {
  // If connecting random link yet, do nothing.
  if (random_link != nullptr || first_link != nullptr) {
    log_debug("connect random (ignore)");
    return;

  } else {
    log_debug("connect random (start)");
  }

  random_link                        = create_link(true);
  random_link->init_data->is_by_seed = true;
  random_link->init_data->is_prime   = true;

  send_offer(random_link, local_nid, local_nid, OFFER_TYPE_RANDOM);
}

LinkState::Type NodeAccessor::get_link_state() const {
  bool has_connecting = false;

  for (auto& link : links) {
    LinkState::Type status = link.second->link_state;
    if (status == LinkState::ONLINE) {
      return LinkState::ONLINE;

    } else if (status == LinkState::CONNECTING) {
      has_connecting = true;
    }
  }

  if (has_connecting || first_link != nullptr || random_link != nullptr) {
    return LinkState::CONNECTING;

  } else {
    return LinkState::OFFLINE;
  }
}

void NodeAccessor::disconnect_all(std::function<void()> on_after) {
  cleanup_closing();
  if (links.size() == 0) {
    on_after();
    return;
  }

  disconnect_link(first_link);
  disconnect_link(random_link);

  while (nid_links.size() != 0) {
    disconnect_link(nid_links.begin()->second);
  }

  scheduler.add_task(this, std::bind(&NodeAccessor::disconnect_all, this, on_after), 500);
}

void NodeAccessor::disconnect_link(const NodeID& nid) {
  auto nid_link = nid_links.find(nid);
  if (nid_link != nid_links.end()) {
    disconnect_link(nid_link->second);
  }
}

void NodeAccessor::disconnect_link(WebrtcLink* link) {
  if (link == nullptr) {
    return;
  }

  if (link == first_link) {
    log_debug("unlink first link");
    first_link = nullptr;
  }

  if (link == random_link) {
    log_debug("unlink random link");
    random_link = nullptr;
  }

  if (links.find(link) == links.end()) {
    return;
  }

  closing_links.insert(link);

  if (nid_links.erase(link->nid) > 0) {
    log_debug("disconnect link").map("nid", link->nid);
    send_buffers.erase(link->nid);
    recv_buffers.erase(link->nid);
  }

  link->nid = NodeID::NONE;
  link->disconnect();
}

void NodeAccessor::initialize(const picojson::object& config) {
  CONFIG_BUFFER_INTERVAL = Utils::get_json(config, "bufferInterval", NODE_ACCESSOR_BUFFER_INTERVAL);
  CONFIG_HOP_COUNT_MAX   = Utils::get_json(config, "hopCountMax", NODE_ACCESSOR_HOP_COUNT_MAX);
  CONFIG_PACKET_SIZE     = Utils::get_json(config, "packetSize", NODE_ACCESSOR_PACKET_SIZE);

  if (CONFIG_PACKET_SIZE != 0 && CONFIG_BUFFER_INTERVAL != 0) {
    scheduler.repeat_task(this, std::bind(&NodeAccessor::send_all_packet, this), CONFIG_BUFFER_INTERVAL);
  }

  webrtc_context.reset(WebrtcContext::new_instance());
  webrtc_context->initialize(Utils::get_json<picojson::array>(config, "iceServers"));
}

bool NodeAccessor::relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet) {
  assert(dst_nid != NodeID::NONE);
  assert(dst_nid != NodeID::SEED);
  assert(dst_nid != NodeID::THIS);
  if (get_link_state() != LinkState::ONLINE) {
    log_warn("drop packet").map("packet", *packet);
    return false;
  }

  if (dst_nid == NodeID::NEXT) {
    assert((packet->mode & PacketMode::ONE_WAY) != PacketMode::NONE);
    for (auto& nid_link : nid_links) {
      NodeID nid       = nid_link.first;
      WebrtcLink& link = *nid_link.second;

      if (link.link_state == LinkState::ONLINE) {
        try_send(nid, *packet);
      }
    }
    return true;

  } else {
    if (nid_links.find(dst_nid) == nid_links.end()) {
      log_warn("drop packet").map("packet", *packet);
      return false;

    } else {
      return try_send(dst_nid, *packet);
    }
  }
}

NodeAccessor::CommandOffer::CommandOffer(NodeAccessor& a, const NodeID& n, OFFER_TYPE t) :
    Command(
        (t == OFFER_TYPE_FIRST || t == OFFER_TYPE_RANDOM) ? (PacketMode::RELAY_SEED | PacketMode::NO_RETRY)
                                                          : PacketMode::EXPLICIT),
    logger(a.logger),
    accessor(a),
    nid(n),
    type(t) {
}

void NodeAccessor::CommandOffer::on_response(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  if (c.has_signaling_offer_success()) {
    on_success(c.signaling_offer_success());

  } else if (c.has_signaling_offer_failure()) {
    on_failure(c.signaling_offer_failure());

  } else {
    log_debug("unsupported response").map("packet", c.DebugString());
  }
}

void NodeAccessor::CommandOffer::on_success(const proto::SignalingOfferSuccess& content) {
  switch (content.status()) {
    case OFFER_STATUS_SUCCESS_FIRST: {
      assert(accessor.first_link != nullptr);
      assert(accessor.nid_links.size() == 0);
      accessor.disconnect_link(accessor.first_link);
      accessor.delegate.node_accessor_on_change_state();
    } break;

    case OFFER_STATUS_SUCCESS_ACCEPT: {
      NodeID second_nid = NodeID::from_pb(content.second_nid());
      std::string sdp   = content.sdp();
      WebrtcLink* link  = nullptr;

      switch (type) {
        case OFFER_TYPE_FIRST: {
          if (!accessor.first_link) {
            return;
          }
          assert(accessor.nid_links.empty());

          link = accessor.first_link;
          accessor.assign_link_nid(second_nid, accessor.first_link);
          accessor.first_link = nullptr;
        } break;

        case OFFER_TYPE_RANDOM: {
          if (!accessor.random_link) {
            return;
          }
          link = accessor.random_link;
          // Check duplicate.
          if (accessor.nid_links.find(second_nid) != accessor.nid_links.end()) {
            accessor.disconnect_link(accessor.random_link);
            return;
          }
          accessor.assign_link_nid(second_nid, accessor.random_link);
          accessor.random_link = nullptr;
        } break;

        case OFFER_TYPE_NORMAL: {
          if (accessor.nid_links.find(second_nid) == accessor.nid_links.end()) {
            // @todo Output drop log.
            return;
          }
          link = accessor.nid_links.at(second_nid);
        } break;

        default:
          assert(false);
      }
      link->set_remote_sdp(sdp);

      // There is a possibility to change link's status by set a SDP.
      if (link->link_state == LinkState::CONNECTING) {
        accessor.send_ice(link, link->init_data->ice);
        link->init_data->ice.clear();
        link->init_data->is_changing_ice = true;
      }
    } break;

    case OFFER_STATUS_SUCCESS_ALREADY: {
      // Do nothing.
    } break;

    default: {
      assert(false);  // fixme
    } break;
  }
}

void NodeAccessor::CommandOffer::on_failure(const proto::SignalingOfferFailure& content) {
  assert(false);  // fixme
}

void NodeAccessor::CommandOffer::on_error(ErrorCode code, const std::string& message) {
  if (accessor.first_link != nullptr) {
    accessor.disconnect_link(accessor.first_link);

  } else {
    auto it = accessor.nid_links.find(nid);
    if (it != accessor.nid_links.end()) {
      accessor.disconnect_link(it->second);

    } else {
      // Maybe timeouted.
      log_debug("timeout?");
      return;
    }
  }

  accessor.delegate.node_accessor_on_change_state();
}

void NodeAccessor::webrtc_link_on_change_dco_state(WebrtcLink& link, LinkState::Type new_dco_state) {
  scheduler.add_task(this, [this, &link, new_dco_state] {
    if (links.find(&link) == links.end()) {
      return;
    }

    if (link.dco_state != new_dco_state) {
      link.dco_state = new_dco_state;
      update_link_state(link);
    }
  });
}

void NodeAccessor::webrtc_link_on_change_pco_state(WebrtcLink& link, LinkState::Type new_pco_state) {
  scheduler.add_task(this, [this, &link, new_pco_state] {
    if (links.find(&link) == links.end()) {
      return;
    }

    if (link.pco_state != new_pco_state) {
      link.pco_state = new_pco_state;
      update_link_state(link);
    }
  });
}

void NodeAccessor::webrtc_link_on_error(WebrtcLink& link) {
  scheduler.add_task(this, [this, &link] {
    disconnect_link(&link);
  });
}

void NodeAccessor::webrtc_link_on_update_ice(WebrtcLink& link, const picojson::object& ice) {
  if (link.link_state != LinkState::CONNECTING) {
    return;
  }

  scheduler.add_task(this, [this, &link, ice = picojson::value(ice)] {
    if (links.find(&link) == links.end() || !link.init_data) {
      return;
    }

    WebrtcLink::InitData& init_data = *link.init_data;
    if (init_data.is_changing_ice) {
      assert(init_data.ice.size() == 0);
      picojson::array ice_array;
      ice_array.push_back(ice);
      send_ice(&link, ice_array);

    } else {
      init_data.ice.push_back(ice);
    }
  });
}

/**
 * When receive event has happen, Raise on_recv on another thread.
 * @param link Data received link.
 * @param data Received data.
 */
void NodeAccessor::webrtc_link_on_recv_data(WebrtcLink& link, const std::string& data) {
  NodeID nid = link.nid;
  scheduler.add_task(this, [this, nid, data] {
    if (nid_links.find(nid) == nid_links.end()) {
      return;
    }

    proto::NodePackets ca;
    if (!ca.ParseFromString(data)) {
      /// @todo error
      assert(false);
    }

    for (int i = 0; i < ca.packet_size(); i++) {
      const proto::NodePacket& pb_packet = ca.packet(i);
      if (pb_packet.has_head()) {
        const proto::NodePacketHead& pb_head = pb_packet.head();

        std::unique_ptr<Packet> packet = std::make_unique<Packet>(
            NodeID::from_pb(pb_head.dst_nid()), NodeID::from_pb(pb_head.src_nid()), pb_packet.id(),
            pb_head.hop_count() + 1, nullptr, static_cast<PacketMode::Type>(pb_head.mode()));
        std::unique_ptr<const std::string> content = std::make_unique<const std::string>(pb_packet.content());

        if (pb_packet.index() == 0) {
          packet->content = std::make_shared<Packet::Content>(std::move(content));
          delegate.node_accessor_on_recv_packet(nid, std::move(packet));

        } else {
          RecvBuffer& recv_buffer = recv_buffers[nid];
          recv_buffer.packet.swap(packet);
          recv_buffer.last_index = pb_packet.index();
          recv_buffer.content_list.clear();
          recv_buffer.content_list.push_back(std::move(content));
        }

      } else {
        auto it = recv_buffers.find(nid);
        if (it != recv_buffers.end()) {
          RecvBuffer& recv_buffer = it->second;
          if (recv_buffer.packet->id == pb_packet.id() && recv_buffer.last_index == pb_packet.index() + 1) {
            recv_buffer.last_index = pb_packet.index();
            recv_buffer.content_list.push_back(std::make_unique<std::string>(pb_packet.content()));
          }

          if (recv_buffer.last_index == 0) {
            int content_size = 0;
            for (auto& one : recv_buffer.content_list) {
              content_size += one->size();
            }
            std::unique_ptr<std::string> content(new std::string());
            content->reserve(content_size);
            for (auto& one : recv_buffer.content_list) {
              content->append(one->data(), one->size());
            }
            recv_buffer.packet->content = std::make_shared<Packet::Content>(std::move(content));
            delegate.node_accessor_on_recv_packet(nid, std::move(recv_buffer.packet));
            recv_buffers.erase(it);
          }
        }
        // Ignore packet if ID is different from previous packet.
      }
    }
  });
}

void NodeAccessor::assign_link_nid(const NodeID& nid, WebrtcLink* link) {
  assert(nid_links.find(nid) == nid_links.end());

  link->nid = nid;
  nid_links.insert(std::make_pair(nid, link));

  update_online_links(nid);
}

void NodeAccessor::check_link_disconnect() {
  cleanup_closing();

  std::set<WebrtcLink*> target_links;
  for (auto& it : links) {
    WebrtcLink* link = it.first;
    if (link->link_state != LinkState::ONLINE && link->link_state != LinkState::CONNECTING &&
        link->nid != NodeID::NONE && link != first_link && link != random_link) {
      target_links.insert(it.first);
    }
  }
  for (auto it : target_links) {
    disconnect_link(it);
  }
}

void NodeAccessor::check_link_timeout() {
  bool is_changed_first      = false;
  const int64_t CURRENT_MSEC = Utils::get_current_msec();

  if (first_link != nullptr) {
    if (first_link->init_data && first_link->init_data->start_time + CONNECT_LINK_TIMEOUT < CURRENT_MSEC) {
      disconnect_link(first_link);
      is_changed_first = true;

      if (get_link_state() != LinkState::ONLINE) {
        if (first_link_try_count < FIRST_LINK_RETRY_MAX) {
          first_link_try_count++;
          create_first_link();

        } else {
          delegate.node_accessor_on_change_state();
        }
      }
    }
  }

  std::set<WebrtcLink*> target_links;
  if (random_link != nullptr) {
    if (random_link->init_data && random_link->init_data->start_time + CONNECT_LINK_TIMEOUT < CURRENT_MSEC) {
      target_links.insert(random_link);
    }
  }

  for (auto& nid_link : nid_links) {
    WebrtcLink* link = nid_link.second;

    if (link->init_data && link->init_data->start_time + CONNECT_LINK_TIMEOUT < CURRENT_MSEC) {
      target_links.insert(link);
    }
  }

  for (auto link : target_links) {
    disconnect_link(link);
  }

  if (!is_changed_first && !target_links.empty() && random_link == nullptr && nid_links.empty()) {
    command_manager.chunk_out(proto::PacketContent::kSignalingOffer);

    first_link_try_count = 0;
    if (first_link == nullptr) {
      create_first_link();
    }
    delegate.node_accessor_on_change_state();
  }
}

void NodeAccessor::cleanup_closing() {
  auto closing_link = closing_links.begin();
  while (closing_link != closing_links.end()) {
    WebrtcLink* link = *closing_link;
    assert(first_link != link);
    assert(random_link != link);
    assert(link->nid == NodeID::NONE);

    if (link->link_state == LinkState::OFFLINE) {
      links.erase(link);
      closing_link = closing_links.erase(closing_link);

    } else {
      // There is a link that is not closing rarely, so disconnect it.
      if (link->link_state != LinkState::CLOSING) {
        link->disconnect();
      }
      closing_link++;
    }
  }
}

void NodeAccessor::create_first_link() {
  assert(first_link == nullptr);
  assert(random_link == nullptr);
  assert(nid_links.empty());

  // Create the initial link.
  first_link                        = create_link(true);
  first_link->init_data->is_by_seed = true;
  first_link->init_data->is_prime   = true;

  send_offer(first_link, local_nid, local_nid, OFFER_TYPE_FIRST);
}

WebrtcLink* NodeAccessor::create_link(bool is_create_dc) {
  WebrtcLinkParam wl_param(*this, logger, *webrtc_context);
  WebrtcLink* link            = WebrtcLink::new_instance(wl_param, is_create_dc);
  link->init_data->start_time = Utils::get_current_msec();
  links.insert(std::make_pair(link, link));

  return link;
}

void NodeAccessor::recv_offer(const Packet& packet) {
  proto::SignalingOffer content = packet.content->as_proto().signaling_offer();
  NodeID prime_nid              = NodeID::from_pb(content.prime_nid());
  const std::string& sdp        = content.sdp();
  OFFER_TYPE type               = static_cast<OFFER_TYPE>(content.type());
  bool is_by_seed;
  if (type == OFFER_TYPE_FIRST || type == OFFER_TYPE_RANDOM) {
    is_by_seed = true;
  } else {
    is_by_seed = false;
  }

  if (prime_nid != local_nid) {
    auto it = nid_links.find(prime_nid);
    if (it != nid_links.end()) {
      WebrtcLink* link = it->second;
      if (link->link_state == LinkState::ONLINE) {
        // Already having a online connection with prime node yet.
        std::unique_ptr<proto::PacketContent> response_content = std::make_unique<proto::PacketContent>();
        proto::SignalingOfferSuccess& param                    = *response_content->mutable_signaling_offer_success();

        param.set_status(OFFER_STATUS_SUCCESS_ALREADY);
        local_nid.to_pb(param.mutable_second_nid());

        command_manager.send_response(packet, std::move(response_content));

      } else if (prime_nid < local_nid) {
        // Already having not a online connection that will be reconnect.
        disconnect_link(link);

        link = create_link(false);
        link->set_remote_sdp(sdp);
        link->init_data->is_by_seed = is_by_seed;
        assign_link_nid(prime_nid, link);

        const Packet p = packet;
        link->get_local_sdp([this, p](const std::string& sdp) -> void {
          std::unique_ptr<proto::PacketContent> response_content = std::make_unique<proto::PacketContent>();
          proto::SignalingOfferSuccess& param                    = *response_content->mutable_signaling_offer_success();

          param.set_status(OFFER_STATUS_SUCCESS_ACCEPT);
          local_nid.to_pb(param.mutable_second_nid());
          param.set_sdp(sdp);

          command_manager.send_response(p, std::move(response_content));
        });

      } else {
        // Already having a connection and it will be disconnect.
        disconnect_link(link);
      }

    } else {
      WebrtcLink* link = create_link(false);
      link->set_remote_sdp(sdp);
      link->init_data->is_by_seed = is_by_seed;
      assign_link_nid(prime_nid, link);

      const Packet p = packet;
      link->get_local_sdp([this, p](const std::string& sdp) -> void {
        std::unique_ptr<proto::PacketContent> response_content = std::make_unique<proto::PacketContent>();
        proto::SignalingOfferSuccess& param                    = *response_content->mutable_signaling_offer_success();

        param.set_status(OFFER_STATUS_SUCCESS_ACCEPT);
        local_nid.to_pb(param.mutable_second_nid());
        param.set_sdp(sdp);

        command_manager.send_response(p, std::move(response_content));
      });
    }

  } else if (type == OFFER_TYPE_RANDOM) {
    command_manager.cancel_packet(packet);
    disconnect_link(random_link);

  } else if (type == OFFER_TYPE_FIRST) {
    if (command_manager.cancel_packet(packet)) {
      disconnect_link(first_link);

    } else {
      std::unique_ptr<proto::PacketContent> response_content = std::make_unique<proto::PacketContent>();
      proto::SignalingOfferFailure& param                    = *response_content->mutable_signaling_offer_failure();

      param.set_status(OFFER_STATUS_FAILURE_CONFLICT);
      prime_nid.to_pb(param.mutable_prime_nid());

      command_manager.send_response(packet, std::move(response_content));
    }

  } else {
    std::unique_ptr<proto::PacketContent> response_content = std::make_unique<proto::PacketContent>();
    proto::SignalingOfferFailure& param                    = *response_content->mutable_signaling_offer_failure();

    param.set_status(OFFER_STATUS_FAILURE_CONFLICT);
    prime_nid.to_pb(param.mutable_prime_nid());

    command_manager.send_response(packet, std::move(response_content));
  }
}

void NodeAccessor::recv_ice(const Packet& packet) {
  proto::SignalingICE content = packet.content->as_proto().signaling_ice();

  NodeID content_local_nid = NodeID::from_pb(content.local_nid());
  picojson::value v;
  const std::string err = picojson::parse(v, content.ice());
  if (err.empty() == false) {
    std::cerr << err << std::endl;
    assert(false);
  }
  picojson::array ice_array = v.get<picojson::array>();

  assert(NodeID::from_pb(content.remote_nid()) == local_nid);

  auto it = nid_links.find(content_local_nid);
  if (it != nid_links.end()) {
    WebrtcLink* link = it->second;

    for (auto& it : ice_array) {
      picojson::object& ice = it.get<picojson::object>();
      link->update_ice(ice);
    }

    if (link->init_data && !link->init_data->is_changing_ice) {
      if (link->init_data->ice.size() != 0) {
        send_ice(link, link->init_data->ice);
        link->init_data->ice.clear();
      }
      link->init_data->is_changing_ice = true;
    }
  }
}

void NodeAccessor::send_all_packet() {
  std::set<NodeID> nids;
  for (auto& it : send_buffers) {
    nids.insert(it.first);
  }
  for (auto& it : nids) {
    send_packet_list(it, true);
  }
}

void NodeAccessor::send_offer(WebrtcLink* link, const NodeID& prime_nid, const NodeID& second_nid, OFFER_TYPE type) {
  link->get_local_sdp([this, prime_nid, second_nid, type](const std::string& sdp) -> void {
    std::shared_ptr<Command> command = std::make_unique<CommandOffer>(*this, second_nid, type);

    std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
    proto::SignalingOffer& param                  = *content->mutable_signaling_offer();

    prime_nid.to_pb(param.mutable_prime_nid());
    second_nid.to_pb(param.mutable_second_nid());
    param.set_sdp(sdp);
    param.set_type(type);

    command_manager.send_packet(second_nid, std::move(content), command);
  });
}

void NodeAccessor::send_ice(WebrtcLink* link, const picojson::array& ice) {
  assert(link->nid != NodeID::NONE);
  assert(link->init_data);

  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::SignalingICE& param                    = *content->mutable_signaling_ice();

  local_nid.to_pb(param.mutable_local_nid());
  link->nid.to_pb(param.mutable_remote_nid());
  param.set_ice(picojson::value(ice).serialize());

  PacketMode::Type mode;
  if (link->init_data->is_by_seed) {
    mode = PacketMode::EXPLICIT | PacketMode::RELAY_SEED;
    if (!link->init_data->is_prime) {
      mode |= PacketMode::RESPONSE;
    }
  } else {
    mode = PacketMode::EXPLICIT;
  }

  command_manager.send_packet_one_way(link->nid, mode, std::move(content));
}

bool NodeAccessor::send_packet_list(const NodeID& dst_nid, bool is_all) {
  auto link_it = nid_links.find(dst_nid);
  auto sb_it   = send_buffers.find(dst_nid);
  if (link_it == nid_links.end() || sb_it == send_buffers.end()) {
    send_buffers.erase(sb_it);
    return false;
  }
  WebrtcLink& link      = *link_it->second;
  std::list<Packet>& sb = sb_it->second;
  auto it               = sb.begin();

  do {
    {  // Bind small packet.
      proto::NodePackets ca;
      unsigned int size_sum = 0;
      while (it != sb.end()) {
        if (it->hop_count > CONFIG_HOP_COUNT_MAX) {
          it = sb.erase(it);
          continue;
        }
        const int content_size = it->content ? it->content->as_string().size() : 0;
        if (size_sum + ESTIMATED_HEAD_SIZE + content_size > CONFIG_PACKET_SIZE) {
          break;
        } else {
          size_sum += ESTIMATED_HEAD_SIZE + content_size;
        }
        proto::NodePacket* packet   = ca.add_packet();
        proto::NodePacketHead* head = packet->mutable_head();
        if (it->dst_nid == NodeID::NEXT) {
          dst_nid.to_pb(head->mutable_dst_nid());
        } else {
          it->dst_nid.to_pb(head->mutable_dst_nid());
        }
        it->src_nid.to_pb(head->mutable_src_nid());
        head->set_hop_count(it->hop_count);
        head->set_mode(it->mode);
        packet->set_id(it->id);
        packet->set_index(0);
        if (it->content) {
          packet->set_content(it->content->as_string());
        }
        it++;
      }

      if (ca.packet_size() != 0) {
        // Send build packets and remove it.
        if (link.send(ca.SerializeAsString())) {
          it = sb.erase(sb.begin(), it);
        } else {
          return false;
        }
      }
    }

    while (it != sb.end() && it->content &&
           ESTIMATED_HEAD_SIZE + it->content->as_string().size() >= CONFIG_PACKET_SIZE) {
      if (it->hop_count > CONFIG_HOP_COUNT_MAX) {
        it = sb.erase(it);
        continue;
      }
      unsigned const int content_size = it->content ? it->content->as_string().size() : 0;
      // Split large packet to send it.
      unsigned const int num = (ESTIMATED_HEAD_SIZE + content_size) / CONFIG_PACKET_SIZE +
                               ((ESTIMATED_HEAD_SIZE + content_size) % CONFIG_PACKET_SIZE == 0 ? 0 : 1);
      unsigned int size_send = 0;
      bool result            = true;
      for (int idx = num - 1; result && idx >= 0; idx--) {
        proto::NodePackets ca;
        proto::NodePacket* packet = ca.add_packet();
        // Append header data for only the first packet.
        if (idx == static_cast<int>(num) - 1) {
          proto::NodePacketHead* head = packet->mutable_head();
          if (it->dst_nid == NodeID::NEXT) {
            dst_nid.to_pb(head->mutable_dst_nid());
          } else {
            it->dst_nid.to_pb(head->mutable_dst_nid());
          }
          it->src_nid.to_pb(head->mutable_src_nid());
          head->set_hop_count(it->hop_count);
          head->set_mode(it->mode);
        }
        packet->set_id(it->id);
        packet->set_index(idx);

        // Calc content_size in the packet.
        unsigned int packet_content_size;
        if (idx == static_cast<int>(num) - 1) {
          packet_content_size = CONFIG_PACKET_SIZE - ESTIMATED_HEAD_SIZE;
          assert(size_send + packet_content_size < content_size);
        } else if (idx == 0) {
          packet_content_size = content_size - size_send;
          assert(packet_content_size != 0);
          assert(packet_content_size <= CONFIG_PACKET_SIZE);
        } else {
          packet_content_size = CONFIG_PACKET_SIZE;
          assert(size_send + CONFIG_PACKET_SIZE < content_size);
        }
        packet->set_content(std::string(it->content->as_string(), size_send, packet_content_size));
        size_send += packet_content_size;
        result = result && link.send(ca.SerializeAsString());
      }

      // Remove the packet if the sending packet is a success.
      if (result) {
        it = sb.erase(it);
      } else {
        return false;
      }
    }
  } while (is_all && it != sb.end());

  return true;
}

bool NodeAccessor::try_send(const NodeID& dst_nid, const Packet& packet) {
  auto it_buffer = send_buffers.find(dst_nid);
  if (it_buffer == send_buffers.end()) {
    it_buffer = send_buffers.insert(std::make_pair(dst_nid, std::list<Packet>())).first;
  }
  it_buffer->second.push_back(packet);

  if (CONFIG_BUFFER_INTERVAL == 0) {
    return send_packet_list(dst_nid, false);

  } else {
    // Estimating packet size.
    unsigned int estimated_size = 0;
    for (auto& it : it_buffer->second) {
      if (it.content) {
        estimated_size += ESTIMATED_HEAD_SIZE + it.content->as_string().size();
      } else {
        estimated_size += ESTIMATED_HEAD_SIZE;
      }
      if (estimated_size > CONFIG_PACKET_SIZE) {
        // Send packet if a list of packets size are larger than threshold.
        return send_packet_list(dst_nid, false);
      }
    }
  }

  return true;
}

void NodeAccessor::update_link_state(WebrtcLink& link) {
  LinkState::Type new_state = link.get_new_link_state();
  if (new_state == link.link_state) {
    return;
  }

  if (new_state != LinkState::CONNECTING) {
    link.init_data.reset();
  }
  bool is_change_online_links = (new_state == LinkState::ONLINE || link.link_state == LinkState::ONLINE);

  link.link_state = new_state;

  if (new_state != LinkState::ONLINE && new_state != LinkState::CONNECTING && &link != first_link &&
      &link != random_link) {
    disconnect_link(&link);
  }

  // update online links
  if (is_change_online_links) {
    update_online_links(link.nid);
  }

  delegate.node_accessor_on_change_state();
}

/**
 * When update edge status, collect online node-ids and pass to routing module.
 */
void NodeAccessor::update_online_links(const NodeID& nid) {
  // update last_online_links if needed
  bool is_updated = false;
  auto nid_link   = nid_links.find(nid);
  if (nid_link != nid_links.end()) {
    WebrtcLink* link = nid_link->second;
    if (link->link_state == LinkState::ONLINE && last_online_links.find(nid) == last_online_links.end()) {
      last_online_links.insert(nid);
      is_updated = true;
    }

  } else if (last_online_links.erase(nid) > 0) {
    is_updated = true;
  }

  // skip if hadn't update
  if (!is_updated) {
    return;
  }

  is_updated_line_links = true;
  scheduler.add_task(this, [this] {
    if (!is_updated_line_links) {
      return;
    }

    std::set<NodeID> nids = last_online_links;
    is_updated_line_links = false;
    delegate.node_accessor_on_change_online_links(nids);
  });
}
}  // namespace colonio
