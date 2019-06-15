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

#include "context.hpp"
#include "convert.hpp"
#include "node_accessor.hpp"
#include "utils.hpp"

namespace colonio {
NodeAccessorDelegate::~NodeAccessorDelegate() {
}

NodeAccessor::NodeAccessor(Context& context, ModuleDelegate& module_delegate, NodeAccessorDelegate& na_delegate) :
    Module(context, module_delegate, ModuleChannel::WEBRTC_CONNECT),
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

  picojson::object js;
  js.insert(std::make_pair("dst_nid", packet->dst_nid.to_json()));
  js.insert(std::make_pair("src_nid", packet->src_nid.to_json()));
  js.insert(std::make_pair("id", Convert::int2json(packet->id)));
  js.insert(std::make_pair("mode", Convert::int2json(packet->mode)));
  js.insert(std::make_pair("channel", Convert::int2json(packet->channel)));
  js.insert(std::make_pair("command_id", Convert::int2json(packet->command_id)));
  js.insert(std::make_pair("content", picojson::value(packet->content)));

  if (dst_nid == NodeID::NEXT) {
    assert((packet->mode & PacketMode::ONE_WAY) != PacketMode::NONE);
    for (auto& it_link : links) {
      NodeID nid = it_link.first;
      WebrtcLink& link = *it_link.second.get();

      if (link.get_status() == LinkStatus::ONLINE) {
        js.at("dst_nid") = nid.to_json();
        
        link.send(picojson::value(js).serialize());
      }
    }
    return true;

  } else {
    if (links.find(dst_nid) == links.end()) {
      logd("drop packet:%s", Utils::dump_packet(*packet).c_str());
      return false;

    } else {
      return links.at(dst_nid)->send(picojson::value(js).serialize());
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
  uint8_t status = Convert::json2int<uint8_t>(packet->content.at("status"));
  switch (status) {
    case OFFER_STATUS_SUCCESS_FIRST: {
      assert(accessor.first_link);
      assert(accessor.links.size() == 0);
      accessor.disconnect_first_link();
      accessor.delegate.node_accessor_on_change_status(accessor, accessor.get_status());
    } break;

    case OFFER_STATUS_SUCCESS_ACCEPT: {
      NodeID second_nid = NodeID::from_json(packet->content.at("second_nid"));
      std::string sdp = packet->content.at("sdp").get<std::string>();
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
  std::istringstream is(data);
  picojson::value v;
  std::string err = picojson::parse(v, is);
  if (!err.empty()) {
    /// @todo error
    assert(false);
  }

  picojson::object& js = v.get<picojson::object>();
  std::unique_ptr<const Packet> packet = std::make_unique<const Packet>(
      Packet {
        NodeID::from_json(js.at("dst_nid")),
        NodeID::from_json(js.at("src_nid")),
        Convert::json2int<uint32_t>(js.at("id")),
        Convert::json2int<PacketMode::Type>(js.at("mode")),
        Convert::json2int<ModuleChannel::Type>(js.at("channel")),
        Convert::json2int<CommandID::Type>(js.at("command_id")),
        js.at("content").get<picojson::object>()
      });

  delegate.node_accessor_on_recv_packet(*this, link.nid, std::move(packet));
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
  NodeID prime_nid = NodeID::from_json(packet->content.at("prime_nid"));
  std::string sdp = packet->content.at("sdp").get<std::string>();
  OFFER_TYPE type = static_cast<OFFER_TYPE>(Convert::json2int<uint8_t>(packet->content.at("type")));
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
        picojson::object content;
        content.insert(std::make_pair("status", Convert::int2json<uint8_t>(OFFER_STATUS_SUCCESS_ALREADY)));
        content.insert(std::make_pair("second_nid", context.my_nid.to_json()));
      
        send_success(*packet, content);

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
            picojson::object content;
            content.insert(std::make_pair("status", Convert::int2json<uint8_t>(OFFER_STATUS_SUCCESS_ACCEPT)));
            content.insert(std::make_pair("second_nid", context.my_nid.to_json()));
            content.insert(std::make_pair("sdp", picojson::value(sdp)));

            send_success(p, content);
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
          picojson::object content;
          content.insert(std::make_pair("status", Convert::int2json<uint8_t>(OFFER_STATUS_SUCCESS_ACCEPT)));
          content.insert(std::make_pair("second_nid", context.my_nid.to_json()));
          content.insert(std::make_pair("sdp", picojson::value(sdp)));

          send_success(p, content);
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
      picojson::object content;
      content.insert(std::make_pair("status", Convert::int2json<uint8_t>(OFFER_STATUS_FAILURE_CONFRICT)));
      content.insert(std::make_pair("prime_nid", prime_nid.to_json()));

      send_failure(*packet, content);
    }
    
  } else {
    picojson::object content;
    content.insert(std::make_pair("status", Convert::int2json<uint8_t>(OFFER_STATUS_FAILURE_CONFRICT)));
    content.insert(std::make_pair("prime_nid", prime_nid.to_json()));

    send_failure(*packet, content);
  }
}

void NodeAccessor::recv_ice(std::unique_ptr<const Packet> packet) {
  NodeID local_nid = NodeID::from_json(packet->content.at("local_nid"));
  NodeID remote_nid = NodeID::from_json(packet->content.at("remote_nid"));
  picojson::array ice_array = packet->content.at("ice").get<picojson::array>();

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

void NodeAccessor::send_offer(WebrtcLink* link, const NodeID& prime_nid,
                              const NodeID& second_nid, OFFER_TYPE type) {
  link->get_local_sdp([this, prime_nid, second_nid, type](const std::string& sdp) -> void {
      std::unique_ptr<Command> command = std::make_unique<CommandOffer>(*this, second_nid, type);

      picojson::object content;
      content.insert(std::make_pair("prime_nid", prime_nid.to_json()));
      content.insert(std::make_pair("second_nid", second_nid.to_json()));
      content.insert(std::make_pair("sdp", picojson::value(sdp)));
      content.insert(std::make_pair("type", Convert::int2json(static_cast<uint8_t>(type))));
      send_packet(std::move(command), second_nid, content);
    });
}

void NodeAccessor::send_ice(WebrtcLink* link, const picojson::array& ice) {
  assert(link->nid != NodeID::NONE);
  assert(link->init_data);
  picojson::object content;
  content.insert(std::make_pair("local_nid", context.my_nid.to_json()));
  content.insert(std::make_pair("remote_nid", link->nid.to_json()));
  content.insert(std::make_pair("ice", picojson::value(ice)));
  PacketMode::Type mode;
  if (link->init_data->is_by_seed) {
    mode = PacketMode::EXPLICIT | PacketMode::RELAY_SEED;
    if (!link->init_data->is_prime) {
      mode |= PacketMode::REPLY;
    }
  } else {
    mode = PacketMode::EXPLICIT;
  }
  send_packet(link->nid, mode, CommandID::WebrtcConnect::ICE, content);
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
