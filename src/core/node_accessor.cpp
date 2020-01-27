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
#include <unistd.h>

#include <cassert>
#include <memory>
#include <string>

#include "node_accessor_protocol.pb.h"

#include "context.hpp"
#include "convert.hpp"
#include "node_accessor.hpp"
#include "utils.hpp"

namespace colonio {
NodeAccessorDelegate::~NodeAccessorDelegate() {
}

NodeAccessor::NodeAccessor(Context& context, ModuleDelegate& module_delegate,
                           NodeAccessorDelegate& na_delegate) :
    Module(context, module_delegate, ModuleChannel::WEBRTC_CONNECT, 0),
    delegate(na_delegate),
    count_seed_transrate(0) {
  context.scheduler.add_interval_task(this, [this]() {
      check_link_disconnect();
      check_link_timeout();
    }, 1000);
}

NodeAccessor::~NodeAccessor() {
  context.scheduler.remove_task(this);
  // It must close connections with subthread alive.
  links.clear();
}

void NodeAccessor::connect_link(const NodeID& nid) {
  auto it = links.find(nid);
  if (it != links.end()) {
    switch (it->second->get_status()) {
      case LinkStatus::CONNECTING:
      case LinkStatus::ONLINE:
        return;

      case LinkStatus::CLOSING:
      case LinkStatus::OFFLINE: {
        closing_links.insert(std::move(it->second));
        links.erase(it);
      } break;

      default: {
        assert(false);
      } break;
    }
  }

  logd("Connect a link.(nid=%s)", nid.to_str().c_str());

  WebrtcLink* link = create_link(true);
  link->nid = nid;
  link->init_data->is_prime = true;
  assert(links.find(nid) == links.end());
  links.insert(std::make_pair(nid, std::unique_ptr<WebrtcLink>(link)));

  update_link_status();

  send_offer(link, context.my_nid, nid, OFFER_TYPE_NORMAL);
}

void NodeAccessor::connect_init_link() {
  assert(get_status() == LinkStatus::OFFLINE);
  assert(!first_link);
  // assert(links.empty()); containing closeing links.

  logd("Connect init link.(tmp_my_nid=%s)", context.my_nid.to_str().c_str());

  first_link_try_count = 0;
  create_first_link();
}

void NodeAccessor::connect_random_link() {
  // If connecting random link yet, do nothing.
  if (random_link || first_link) {
    logd("Connect random (ignore).");
    return;

  } else {
    logd("Connect random (start).");
  }

  random_link.reset(create_link(true));
  random_link->init_data->is_by_seed = true;
  random_link->init_data->is_prime = true;

  send_offer(random_link.get(), context.my_nid, context.my_nid, OFFER_TYPE_RANDOM);
}

LinkStatus::Type NodeAccessor::get_status() {
  bool is_connecting = false;

  for (auto& link : links) {
    LinkStatus::Type status = link.second->get_status();
    if (status == LinkStatus::ONLINE) {
      return LinkStatus::ONLINE;

    } else if (status == LinkStatus::CONNECTING) {
      is_connecting = true;
    }
  }

  if (is_connecting || first_link || random_link) {
    return LinkStatus::CONNECTING;

  } else {
    return LinkStatus::OFFLINE;
  }
}

void NodeAccessor::disconnect_all() {
  while (first_link || random_link || links.size() != 0 || closing_links.size() != 0) {
    disconnect_first_link();
    disconnect_random_link();
    
    std::set<NodeID> nids;
    for (const auto& it : links) {
      nids.insert(it.first);
    }
    for (const auto& nid : nids) {
      disconnect_link(nid);
    }

    cleanup_closing();
  }
}

void NodeAccessor::disconnect_link(const NodeID& nid) {
  auto it = links.find(nid);

  if (it == links.end()) {
    return;
  }

  logd("Disconnect link.(nid=%s)", nid.to_str().c_str());

  WebrtcLink* link = it->second.get();
  link->disconnect();
}

void NodeAccessor::initialize(const picojson::object& config) {

  CONFIG_PACKET_SIZE = Utils::get_json(config, "packetSize", NODE_ACCESSOR_PACKET_SIZE);
  CONFIG_BUFFER_INTERVAL = Utils::get_json(config, "bufferInterval", NODE_ACCESSOR_BUFFER_INTERVAL);

  if (CONFIG_PACKET_SIZE != 0 && CONFIG_BUFFER_INTERVAL != 0) {
    context.scheduler.add_interval_task(this, [this]() {
        send_all_packet();
      }, CONFIG_BUFFER_INTERVAL);
  }

  webrtc_context.initialize(Utils::get_json<picojson::array>(config, "iceServers"));
}

bool NodeAccessor::relay_packet(const NodeID& dst_nid, std::unique_ptr<const Packet> packet) {
  assert(dst_nid != NodeID::NONE);
  assert(dst_nid != NodeID::SEED);
  assert(dst_nid != NodeID::THIS);
  if (get_status() != LinkStatus::ONLINE) {
    logd("drop packet:%s", Utils::dump_packet(*packet).c_str());
    return false;
  }

  if (dst_nid == NodeID::NEXT) {
    assert((packet->mode & PacketMode::ONE_WAY) != PacketMode::NONE);
    for (auto& it_link : links) {
      NodeID nid = it_link.first;
      WebrtcLink& link = *it_link.second.get();

      if (link.get_status() == LinkStatus::ONLINE) {
        try_send(nid, *packet);
      }
    }
    return true;

  } else {
    if (links.find(dst_nid) == links.end()) {
      logd("drop packet:%s", Utils::dump_packet(*packet).c_str());
      return false;

    } else {
      return try_send(dst_nid, *packet);
    }
  }
}

NodeAccessor::CommandOffer::CommandOffer(NodeAccessor& accessor_, const NodeID& nid_, OFFER_TYPE type_) :
    Command(CommandID::WebrtcConnect::OFFER,
            (type_ == OFFER_TYPE_FIRST || type_ == OFFER_TYPE_RANDOM) ?
            (PacketMode::RELAY_SEED | PacketMode::NO_RETRY) : PacketMode::EXPLICIT),
    accessor(accessor_),
    nid(nid_),
    type(type_) {
}

void NodeAccessor::CommandOffer::on_success(std::unique_ptr<const Packet> packet) {
  NodeAccessorProtocol::OfferSuccess content;
  packet->parse_content(&content);

  switch (content.status()) {
    case OFFER_STATUS_SUCCESS_FIRST: {
      assert(accessor.first_link);
      assert(accessor.links.size() == 0);
      accessor.disconnect_first_link();
      accessor.delegate.node_accessor_on_change_status(accessor, accessor.get_status());
    } break;

    case OFFER_STATUS_SUCCESS_ACCEPT: {
      NodeID second_nid = NodeID::from_pb(content.second_nid());
      std::string sdp = content.sdp();
      WebrtcLink* link;

      switch (type) {
        case OFFER_TYPE_FIRST: {
          assert(accessor.first_link);
          link = accessor.first_link.get();
          accessor.first_link->nid = second_nid;
          assert(accessor.links.find(second_nid) == accessor.links.end());
          accessor.links.insert(std::make_pair(second_nid, std::move(accessor.first_link)));
        } break;

        case OFFER_TYPE_RANDOM: {
          assert(accessor.random_link);
          link = accessor.random_link.get();
          accessor.random_link->nid = second_nid;
          if (accessor.links.find(second_nid) == accessor.links.end()) {
            accessor.links.insert(std::make_pair(second_nid, std::move(accessor.random_link)));
          } else {
            // Duplicate.
            accessor.disconnect_random_link();
          }
        } break;

        case OFFER_TYPE_NORMAL: {
          if (accessor.links.find(second_nid) == accessor.links.end()) {
            // @todo Output drop log.
            return;
          }
          link = accessor.links.at(second_nid).get();
        } break;

        default:
          assert(false);
      }
      link->set_remote_sdp(sdp);

      // There is a possibility to change link's status by set a SDP.
      if (link->get_status() == LinkStatus::CONNECTING) {
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

void NodeAccessor::CommandOffer::on_failure(std::unique_ptr<const Packet> packet) {
  assert(false);  // fixme
}

void NodeAccessor::CommandOffer::on_error(const std::string& message) {
  std::unique_ptr<WebrtcLink> link;
  bool is_first_link = false;

  if (accessor.first_link) {
    link = std::move(accessor.first_link);
    is_first_link = true;

  } else {
    auto it = accessor.links.find(nid);
    if (it != accessor.links.end()) {
      link = std::move(it->second);
      accessor.links.erase(it);

    } else {
      // Maybe timeouted.
      logD(accessor.context, "Timeout?");
      return;
    }
  }

  link->disconnect();
  accessor.closing_links.insert(std::move(link));
  accessor.update_link_status();
  accessor.delegate.node_accessor_on_change_status(accessor, accessor.get_status());
}

void NodeAccessor::module_process_command(std::unique_ptr<const Packet> packet) {
  if (packet->channel == ModuleChannel::WEBRTC_CONNECT) {
    switch (packet->command_id) {
      case CommandID::WebrtcConnect::OFFER:
        recv_offer(std::move(packet));
        break;

      case CommandID::WebrtcConnect::ICE:
        recv_ice(std::move(packet));
        break;

      default:
        assert(false);
        return;
        break;
    }
  } else {
    assert(false);
  }
}

void NodeAccessor::webrtc_link_on_change_stateus(WebrtcLink& link, LinkStatus::Type link_status) {
  logd("Change status. (status=%d)", link_status);

  update_link_status();
  delegate.node_accessor_on_change_status(*this, get_status());
}

void NodeAccessor::webrtc_link_on_error(WebrtcLink& link) {
  if (first_link && first_link.get() == &link) {
    disconnect_first_link();

  } else {
    auto it = links.find(link.nid);
    if (it != links.end()) {
      link.disconnect();
      closing_links.insert(std::move(it->second));
      links.erase(it);
    }
  }
  update_link_status();
}

void NodeAccessor::webrtc_link_on_update_ice(WebrtcLink& link, const picojson::object& ice) {
  if (link.get_status() == LinkStatus::CONNECTING) {
    if (link.init_data && link.init_data->is_changing_ice) {
      assert(link.init_data->ice.size() == 0);
      picojson::array ice_array;
      ice_array.push_back(picojson::value(ice));
      send_ice(&link, ice_array);

    } else {
      assert(link.init_data);
      link.init_data->ice.push_back(picojson::value(ice));
    }
  }
}

/**
 * When receive event has happen, Raise on_recv on another thraed.
 * @param link Data received link.
 * @param data Received data.
 */
void NodeAccessor::webrtc_link_on_recv_data(WebrtcLink& link, const std::string& data) {
  NodeAccessorProtocol::Carrier ca;
  if (!ca.ParseFromString(data)) {
    /// @todo error
    assert(false);
  }

  for (int i = 0; i < ca.packet_size(); i++) {
    const NodeAccessorProtocol::Packet& pb_packet = ca.packet(i);
    if (pb_packet.has_head()) {
      const NodeAccessorProtocol::Head& pb_head = pb_packet.head();
      std::unique_ptr<Packet> packet = std::make_unique<Packet>(
        Packet {
          NodeID::from_pb(pb_head.dst_nid()),
          NodeID::from_pb(pb_head.src_nid()),
          pb_packet.id(),
          nullptr,
          static_cast<PacketMode::Type>(pb_head.mode()),
          static_cast<ModuleChannel::Type>(pb_head.channel()),
          static_cast<ModuleNo>(pb_head.module_no()),
          static_cast<CommandID::Type>(pb_head.command_id())
        });
      std::shared_ptr<const std::string> content(new std::string(pb_packet.content()));

      if (pb_packet.index() == 0) {
        packet->content = content;
        delegate.node_accessor_on_recv_packet(*this, link.nid, std::move(packet));

      } else {
        RecvBuffer& recv_buffer = recv_buffers[link.nid];
        recv_buffer.packet.swap(packet);
        recv_buffer.last_index = pb_packet.index();
        recv_buffer.content_list.clear();
        recv_buffer.content_list.push_back(content);
      }

    } else {
      auto it = recv_buffers.find(link.nid);
      if (it != recv_buffers.end()) {
        RecvBuffer& recv_buffer = it->second;
        if (recv_buffer.packet->id == pb_packet.id() && recv_buffer.last_index == pb_packet.index() + 1) {
          recv_buffer.last_index = pb_packet.index();
          recv_buffer.content_list.push_back(std::shared_ptr<std::string>(new std::string(pb_packet.content())));
        }

        if (recv_buffer.last_index == 0) {
          int content_size = 0;
          for (auto& one : recv_buffer.content_list) {
            content_size += one->size();
          }
          std::shared_ptr<std::string> content(new std::string());
          content->reserve(content_size);
          for (auto& one : recv_buffer.content_list) {
            content->append(one->data(), one->size());
          }
          recv_buffer.packet->content = content;
          delegate.node_accessor_on_recv_packet(*this, link.nid, std::move(recv_buffer.packet));
          recv_buffers.erase(it);
        }
      }
      // Ignore packet if ID is different from previous packet.
    }
  }
}

void NodeAccessor::check_link_disconnect() {
  cleanup_closing();

  auto it = links.begin();
  bool is_changed = false;
  while (it != links.end()) {
    WebrtcLink& link = *reinterpret_cast<WebrtcLink*>(it->second.get());
    if (link.get_status() == LinkStatus::OFFLINE) {
      closing_links.insert(std::move(it->second));
      it = links.erase(it);
      is_changed = true;

    } else {
      it++;
    }
  }

  if (is_changed) {
    update_link_status();
  }
}

void NodeAccessor::check_link_timeout() {
  bool is_changed_first = false;
  bool is_changed_normal = false;
  int64_t current_msec = Utils::get_current_msec();

  if (first_link) {
    if (first_link->init_data &&
        first_link->init_data->start_time + CONNECT_LINK_TIMEOUT < current_msec) {
      disconnect_first_link();
      is_changed_first = true;

      LinkStatus::Type status = get_status();
      if (status != LinkStatus::ONLINE) {
        if (first_link_try_count < FIRST_LINK_RETRY_MAX) {
          first_link_try_count ++;
          create_first_link();

        } else {
          delegate.node_accessor_on_change_status(*this, status);
        }
      }
    }
  }

  if (random_link) {
    if (random_link->init_data &&
        random_link->init_data->start_time + CONNECT_LINK_TIMEOUT < current_msec) {
      disconnect_random_link();
      is_changed_normal = true;
    }
  }

  auto it = links.begin();
  while (it != links.end()) {
    WebrtcLink& link = *(it->second.get());

    if (link.init_data &&
        link.init_data->start_time + CONNECT_LINK_TIMEOUT < current_msec) {
      link.disconnect();
      closing_links.insert(std::move(it->second));
      it = links.erase(it);
      is_changed_normal = true;
      continue;
    }

    it++;
  }

  if (is_changed_normal) {
    update_link_status();
  }

  // Dont check previous link status to detect first link's ice timeout.
  if (!is_changed_first && is_changed_normal && links.size() == 0) {
    reset();
    first_link_try_count = 0;
    if (!first_link) {
      create_first_link();
    }
    delegate.node_accessor_on_change_status(*this, get_status());
  }
}

void NodeAccessor::cleanup_closing() {
  auto it = closing_links.begin();
  while (it != closing_links.end()) {
    WebrtcLink& link = *it->get();
    LinkStatus::Type status = link.get_status();
    if (status == LinkStatus::OFFLINE &&
        !context.scheduler.is_having_task(&link)) {
      it = closing_links.erase(it);

    } else {
      assert(status == LinkStatus::CLOSING ||
             context.scheduler.is_having_task(&link));
      it++;
    }
  }
}

void NodeAccessor::create_first_link() {
  assert(!first_link);

  // Create the initial link.
  first_link.reset(create_link(true));
  first_link->init_data->is_by_seed = true;
  first_link->init_data->is_prime = true;

  send_offer(first_link.get(), context.my_nid, context.my_nid, OFFER_TYPE_FIRST);
}

WebrtcLink* NodeAccessor::create_link(bool is_create_dc) {
  WebrtcLink* link = new WebrtcLink(*this, context, webrtc_context, is_create_dc);
  link->init_data->start_time = Utils::get_current_msec();

  return link;
}

void NodeAccessor::disconnect_first_link() {
  if (!first_link) {
    return;
  }

  first_link->disconnect();
  closing_links.insert(std::move(first_link));
}

void NodeAccessor::disconnect_random_link() {
  if (!random_link) {
    return;
  }

  random_link->disconnect();
  closing_links.insert(std::move(random_link));
}

void NodeAccessor::recv_offer(std::unique_ptr<const Packet> packet) {
  NodeAccessorProtocol::Offer content;
  packet->parse_content(&content);
  NodeID prime_nid = NodeID::from_pb(content.prime_nid());
  const std::string& sdp = content.sdp();
  OFFER_TYPE type = static_cast<OFFER_TYPE>(content.type());
  bool is_by_seed;
  if (type == OFFER_TYPE_FIRST ||
      type == OFFER_TYPE_RANDOM) {
    is_by_seed = true;
  } else {
    is_by_seed = false;
  }

  if (prime_nid != context.my_nid) {
    auto it = links.find(prime_nid);
    if (it != links.end()) {
      WebrtcLink* link = it->second.get();
      if (link->get_status() == LinkStatus::ONLINE) {
        // Already having a online connection with prime node yet.
        NodeAccessorProtocol::OfferSuccess param;
        param.set_status(OFFER_STATUS_SUCCESS_ALREADY);
        context.my_nid.to_pb(param.mutable_second_nid());

        send_success(*packet, serialize_pb(param));

      } else if (prime_nid < context.my_nid) {
        // Already having not a online connection that will be reconnect.
        link->disconnect();
        closing_links.insert(std::move(it->second));
        links.erase(it);

        link = create_link(false);
        link->nid = prime_nid;
        link->set_remote_sdp(sdp);
        link->init_data->is_by_seed = is_by_seed;
        assert(links.find(prime_nid) == links.end());
        links.insert(std::make_pair(prime_nid, std::unique_ptr<WebrtcLink>(link)));

        update_link_status();

        const Packet& p = *packet; // @todo use std::move
        link->get_local_sdp([this, p](const std::string& sdp) -> void {
            NodeAccessorProtocol::OfferSuccess param;
            param.set_status(OFFER_STATUS_SUCCESS_ACCEPT);
            context.my_nid.to_pb(param.mutable_second_nid());
            param.set_sdp(sdp);

            send_success(p, serialize_pb(param));
          });

      } else {
        // Already having a connecton and it will be disconnect.
        link->disconnect();
        closing_links.insert(std::move(it->second));
        links.erase(it);
        update_link_status();
      }

    } else {
      WebrtcLink* link = create_link(false);
      link->nid = prime_nid;
      link->set_remote_sdp(sdp);
      link->init_data->is_by_seed = is_by_seed;
      assert(links.find(prime_nid) == links.end());
      links.insert(std::make_pair(prime_nid, std::unique_ptr<WebrtcLink>(link)));

      const Packet& p = *packet; // @todo use std::move
      link->get_local_sdp([this, p](const std::string& sdp) -> void {
          NodeAccessorProtocol::OfferSuccess param;
          param.set_status(OFFER_STATUS_SUCCESS_ACCEPT);
          context.my_nid.to_pb(param.mutable_second_nid());
          param.set_sdp(sdp);

          send_success(p, serialize_pb(param));
        });
    }

  } else if (type == OFFER_TYPE_RANDOM) {
    cancel_packet(packet->id);

    if (random_link) {
      disconnect_random_link();
    }

  } else if (type == OFFER_TYPE_FIRST) {
    if (cancel_packet(packet->id)) {
      if (first_link) {
        disconnect_first_link();
      }

    } else {
      NodeAccessorProtocol::OfferFailure param;
      param.set_status(OFFER_STATUS_FAILURE_CONFRICT);
      prime_nid.to_pb(param.mutable_prime_nid());

      send_failure(*packet, serialize_pb(param));
    }
    
  } else {
    NodeAccessorProtocol::OfferFailure param;
    param.set_status(OFFER_STATUS_FAILURE_CONFRICT);
    prime_nid.to_pb(param.mutable_prime_nid());

    send_failure(*packet, serialize_pb(param));
  }
}

void NodeAccessor::recv_ice(std::unique_ptr<const Packet> packet) {
  NodeAccessorProtocol::ICE content;
  packet->parse_content(&content);
  NodeID local_nid = NodeID::from_pb(content.local_nid());
  NodeID remote_nid = NodeID::from_pb(content.remote_nid());
  picojson::value v;
  const std::string err = picojson::parse(v, content.ice());
  if (err.empty() == false) {
    std::cerr << err << std::endl;
    assert(false);
  }
  picojson::array ice_array = v.get<picojson::array>();

  assert(remote_nid == context.my_nid);

  auto it = links.find(local_nid);
  if (it != links.end()) {
    WebrtcLink* link = it->second.get();

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
  for (auto& it : send_buffers) {
    send_packet_list(it.first, true);
  }
}

void NodeAccessor::send_offer(WebrtcLink* link, const NodeID& prime_nid,
                              const NodeID& second_nid, OFFER_TYPE type) {
  link->get_local_sdp([this, prime_nid, second_nid, type](const std::string& sdp) -> void {
      std::unique_ptr<Command> command = std::make_unique<CommandOffer>(*this, second_nid, type);

      NodeAccessorProtocol::Offer content;
      prime_nid.to_pb(content.mutable_prime_nid());
      second_nid.to_pb(content.mutable_second_nid());
      content.set_sdp(sdp);
      content.set_type(type);
      send_packet(std::move(command), second_nid, serialize_pb(content));
    });
}

void NodeAccessor::send_ice(WebrtcLink* link, const picojson::array& ice) {
  assert(link->nid != NodeID::NONE);
  assert(link->init_data);
  NodeAccessorProtocol::ICE content;
  context.my_nid.to_pb(content.mutable_local_nid());
  link->nid.to_pb(content.mutable_remote_nid());
  content.set_ice(picojson::value(ice).serialize());

  PacketMode::Type mode;
  if (link->init_data->is_by_seed) {
    mode = PacketMode::EXPLICIT | PacketMode::RELAY_SEED;
    if (!link->init_data->is_prime) {
      mode |= PacketMode::REPLY;
    }
  } else {
    mode = PacketMode::EXPLICIT;
  }

  send_packet(link->nid, mode, CommandID::WebrtcConnect::ICE, serialize_pb(content));
}

bool NodeAccessor::send_packet_list(const NodeID& dst_nid, bool is_all) {
  auto link_it = links.find(dst_nid);
  auto sb_it = send_buffers.find(dst_nid);
  if (link_it == links.end() || sb_it == send_buffers.end()) {
    send_buffers.erase(sb_it);
    return false;
  }
  WebrtcLink& link = *link_it->second;
  std::list<Packet>& sb = sb_it->second;
  auto it = sb.begin();

  do {
    { // Bind small packet.
      NodeAccessorProtocol::Carrier ca;
      unsigned int size_sum = 0;
      while (it != sb.end()) {
        const int content_size = it->content ? it->content->size() : 0;
        if (size_sum + ESTIMATED_HEAD_SIZE + content_size > CONFIG_PACKET_SIZE) {
          break;
        } else {
          size_sum += ESTIMATED_HEAD_SIZE + content_size;
        }
        NodeAccessorProtocol::Packet* packet = ca.add_packet();
        NodeAccessorProtocol::Head* head = packet->mutable_head();
        if (it->dst_nid == NodeID::NEXT) {
          dst_nid.to_pb(head->mutable_dst_nid());
        } else {
          it->dst_nid.to_pb(head->mutable_dst_nid());
        }
        it->src_nid.to_pb(head->mutable_src_nid());
        head->set_mode(it->mode);
        head->set_channel(it->channel);
        head->set_command_id(it->command_id);
        packet->set_id(it->id);
        packet->set_index(0);
        if (it->content) {
          packet->set_content(*it->content);
        }
        it++;
      }

      if (ca.packet_size() != 0) {
        // Send binded packets and remove it.
        if (link.send(*serialize_pb(ca))) {
          it = sb.erase(sb.begin(), it);
        } else {
          return false;
        }
      }
    }

    while (it != sb.end() && it->content && ESTIMATED_HEAD_SIZE + it->content->size() >= CONFIG_PACKET_SIZE) {
      const int content_size = it->content ? it->content->size() : 0;
      // Split large packet to send it.
      const int num = (ESTIMATED_HEAD_SIZE + content_size) / CONFIG_PACKET_SIZE +
        ((ESTIMATED_HEAD_SIZE + content_size) % CONFIG_PACKET_SIZE == 0 ? 0 : 1);
      int size_send = 0;
      bool result = true;
      for (int idx = num - 1; result && idx >= 0; idx --) {
        NodeAccessorProtocol::Carrier ca;
        NodeAccessorProtocol::Packet* packet = ca.add_packet();
        // Append header data for only the first packet.
        if (idx == num - 1) {
          NodeAccessorProtocol::Head* head = packet->mutable_head();
          if (it->dst_nid == NodeID::NEXT) {
            dst_nid.to_pb(head->mutable_dst_nid());
          } else {
            it->dst_nid.to_pb(head->mutable_dst_nid());
          }
          it->src_nid.to_pb(head->mutable_src_nid());
          head->set_mode(it->mode);
          head->set_channel(it->channel);
          head->set_command_id(it->command_id);
        }
        packet->set_id(it->id);
        packet->set_index(idx);

        // Calc content_size in the packet.
        int packet_content_size;
        if (idx == num - 1) {
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
        packet->set_content(std::string(*it->content, size_send, packet_content_size));
        size_send += packet_content_size;
        result = result && link.send(*serialize_pb(ca));
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
    int estimated_size = 0;
    for (auto& it : it_buffer->second) {
      if (it.content) {
        estimated_size += ESTIMATED_HEAD_SIZE + it.content->size();
      } else {
        estimated_size += ESTIMATED_HEAD_SIZE;
      }
      if (estimated_size > CONFIG_PACKET_SIZE) {
        // Send packet if a list of packets size are larger than threthold.
        return send_packet_list(dst_nid, false);
      }
    }
  }
  
  return true;
}

/**
 * When update edge status, collect online node-ids and pass to routing module.
 * @param handle libuv's handler.
 */
void NodeAccessor::update_link_status() {
  std::set<NodeID> nids;

  // Tell event for Routing module
  for (auto& it_link : links) {
    NodeID nid = it_link.first;
    WebrtcLink* link = it_link.second.get();
    if (link->get_status() == LinkStatus::ONLINE) {
      nids.insert(nid);
    }
  }

  if (nids != last_link_status) {
    delegate.node_accessor_on_change_online_links(*this, nids);
    last_link_status = nids;
  }
}
}  // namespace colonio
