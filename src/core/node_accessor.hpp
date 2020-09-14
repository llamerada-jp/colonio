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

#include <picojson.h>

#include <functional>
#include <list>
#include <memory>
#include <string>

#include "command.hpp"
#include "module_base.hpp"
#include "node_id.hpp"
#include "packet.hpp"
#include "webrtc_context.hpp"
#include "webrtc_link.hpp"

namespace colonio {
class Context;
class NodeAccessor;

class NodeAccessorDelegate {
 public:
  virtual ~NodeAccessorDelegate();
  virtual void node_accessor_on_change_online_links(NodeAccessor& na, const std::set<NodeID> nids) = 0;
  virtual void node_accessor_on_change_status(NodeAccessor& na)                                    = 0;
  virtual void node_accessor_on_recv_packet(
      NodeAccessor& na, const NodeID& nid, std::unique_ptr<const Packet> packet) = 0;
};

class NodeAccessor : public ModuleBase, public WebrtcLinkDelegate {
 public:
  NodeAccessor(Context& context, ModuleDelegate& module_delegate, NodeAccessorDelegate& na_delegate);
  virtual ~NodeAccessor();

  void connect_link(const NodeID& nid);
  void connect_init_link();
  void connect_random_link();
  LinkStatus::Type get_status();
  void disconnect_all(std::function<void()> on_after);
  void disconnect_link(const NodeID& nid);
  void initialize(const picojson::object& config);
  bool relay_packet(const NodeID& dst, std::unique_ptr<const Packet> packet);
  void update_link_status();

 private:
  enum OFFER_TYPE {
    OFFER_TYPE_FIRST = 0,
    OFFER_TYPE_RANDOM,
    OFFER_TYPE_NORMAL,
  };

  enum OFFER_STATUS_SUCCESS {
    OFFER_STATUS_SUCCESS_FIRST = 0,
    OFFER_STATUS_SUCCESS_ACCEPT,
    OFFER_STATUS_SUCCESS_ALREADY,
  };

  enum OFFERS_STATUS_FAILURE {
    OFFER_STATUS_FAILURE_CONFLICT = 0,
  };

  class CommandOffer : public Command {
   public:
    CommandOffer(NodeAccessor& accessor_, const NodeID& nid_, OFFER_TYPE type_);
    void on_success(std::unique_ptr<const Packet> packet) override;
    void on_failure(std::unique_ptr<const Packet> packet) override;
    void on_error(const std::string& message) override;

   private:
    NodeAccessor& accessor;
    NodeID nid;
    OFFER_TYPE type;
  };

  unsigned int CONFIG_BUFFER_INTERVAL;
  unsigned int CONFIG_HOP_COUNT_MAX;
  unsigned int CONFIG_PACKET_SIZE;

  NodeAccessorDelegate& delegate;
  WebrtcContext webrtc_context;

  /** Link pool. */
  std::unique_ptr<WebrtcLink> first_link;
  unsigned int first_link_try_count;
  std::unique_ptr<WebrtcLink> random_link;
  /** Map of node-id and link. */
  std::map<NodeID, std::unique_ptr<WebrtcLink>> links;
  /** Closing links. */
  std::set<std::unique_ptr<WebrtcLink>> closing_links;

  /** Count of links which need to transrate packet by seed. */
  unsigned int count_seed_transrate;

  /** Use on update_link_status() */
  std::set<NodeID> last_link_status;

  /** Waiting buffer of packets to send. */
  std::map<NodeID, std::list<Packet>> send_buffers;
  /** Waiting buffer of packets to recv. */
  struct RecvBuffer {
    std::unique_ptr<Packet> packet;
    int last_index;
    std::list<std::shared_ptr<const std::string>> content_list;
  };
  std::map<NodeID, RecvBuffer> recv_buffers;

  explicit NodeAccessor(const NodeAccessor&);
  NodeAccessor& operator=(const NodeAccessor&);

  void module_process_command(std::unique_ptr<const Packet> packet) override;

  void webrtc_link_on_change_status(WebrtcLink& link, LinkStatus::Type status) override;
  void webrtc_link_on_error(WebrtcLink& link) override;
  void webrtc_link_on_update_ice(WebrtcLink& link, const picojson::object& ice) override;
  void webrtc_link_on_recv_data(WebrtcLink& link, const std::string& data) override;

  void check_link_disconnect();
  void check_link_timeout();
  void cleanup_closing();
  void create_first_link();
  WebrtcLink* create_link(bool is_create_dc);
  void disconnect_first_link();
  void disconnect_random_link();
  void recv_offer(std::unique_ptr<const Packet> packet);
  void recv_ice(std::unique_ptr<const Packet> packet);
  void send_all_packet();
  void send_ice(WebrtcLink* link, const picojson::array& ice);
  void send_offer(WebrtcLink* link, const NodeID& prime_nid, const NodeID& second_nid, OFFER_TYPE type);
  bool send_packet_list(const NodeID& dst_nid, bool is_all);
  bool try_send(const NodeID& dst_nid, const Packet& packet);
};
}  // namespace colonio
