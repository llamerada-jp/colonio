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
#pragma once

#ifdef __clang__
#  include <picojson.h>
#else
#  pragma GCC diagnostic push
#  pragma GCC diagnostic ignored "-Wmaybe-uninitialized"
#  include <picojson.h>
#  pragma GCC diagnostic pop
#endif

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
class NodeAccessor;

class NodeAccessorDelegate {
 public:
  virtual ~NodeAccessorDelegate();
  virtual void node_accessor_on_change_online_links(NodeAccessor& na, const std::set<NodeID>& nids) = 0;
  virtual void node_accessor_on_change_state(NodeAccessor& na)                                      = 0;
  virtual void node_accessor_on_recv_packet(
      NodeAccessor& na, const NodeID& nid, std::unique_ptr<const Packet> packet) = 0;
};

class NodeAccessor : public ModuleBase, public WebrtcLinkDelegate {
 public:
  NodeAccessor(ModuleParam& param, NodeAccessorDelegate& na_delegate);
  virtual ~NodeAccessor();
  NodeAccessor(const NodeAccessor&) = delete;
  NodeAccessor& operator=(const NodeAccessor&) = delete;

  void connect_link(const NodeID& nid);
  void connect_init_link();
  void connect_random_link();
  LinkState::Type get_link_state() const;
  void disconnect_all(std::function<void()> on_after);
  void disconnect_link(const NodeID& nid);
  void initialize(const picojson::object& config);
  bool relay_packet(const NodeID& dst, std::unique_ptr<const Packet> packet);

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
    void on_error(ErrorCode code, const std::string& message) override;

   private:
    NodeAccessor& accessor;
    NodeID nid;
    OFFER_TYPE type;
  };

  unsigned int CONFIG_BUFFER_INTERVAL;
  unsigned int CONFIG_HOP_COUNT_MAX;
  unsigned int CONFIG_PACKET_SIZE;

  NodeAccessorDelegate& delegate;
  std::unique_ptr<WebrtcContext> webrtc_context;

  /** Link pool. */
  std::map<WebrtcLink*, std::unique_ptr<WebrtcLink>> links;
  WebrtcLink* first_link;
  unsigned int first_link_try_count;
  WebrtcLink* random_link;
  /** Map of node-id and link. */
  std::map<NodeID, WebrtcLink*> nid_links;
  /** Closing links. */
  std::set<WebrtcLink*> closing_links;

  /** Count of links which need to transrate packet by seed. */
  unsigned int count_seed_transrate;

  /** Use on update_online_links() */
  bool is_updated_line_links;
  std::set<NodeID> last_online_links;

  /** Waiting buffer of packets to send. */
  std::map<NodeID, std::list<Packet>> send_buffers;
  /** Waiting buffer of packets to recv. */
  struct RecvBuffer {
    std::unique_ptr<Packet> packet;
    unsigned int last_index;
    std::list<std::shared_ptr<const std::string>> content_list;
  };
  std::map<NodeID, RecvBuffer> recv_buffers;

  void module_process_command(std::unique_ptr<const Packet> packet) override;

  void webrtc_link_on_change_dco_state(WebrtcLink& link, LinkState::Type new_dco_state) override;
  void webrtc_link_on_change_pco_state(WebrtcLink& link, LinkState::Type new_pco_state) override;
  void webrtc_link_on_error(WebrtcLink& link) override;
  void webrtc_link_on_update_ice(WebrtcLink& link, const picojson::object& ice) override;
  void webrtc_link_on_recv_data(WebrtcLink& link, const std::string& data) override;

  void assign_link_nid(const NodeID& nid, WebrtcLink* link);
  void check_link_disconnect();
  void check_link_timeout();
  void cleanup_closing();
  void create_first_link();
  WebrtcLink* create_link(bool is_create_dc);
  void disconnect_link(WebrtcLink* link);
  void recv_offer(std::unique_ptr<const Packet> packet);
  void recv_ice(std::unique_ptr<const Packet> packet);
  void send_all_packet();
  void send_ice(WebrtcLink* link, const picojson::array& ice);
  void send_offer(WebrtcLink* link, const NodeID& prime_nid, const NodeID& second_nid, OFFER_TYPE type);
  bool send_packet_list(const NodeID& dst_nid, bool is_all);
  void update_link_state(WebrtcLink& link);
  void update_online_links(const NodeID& nid);
  bool try_send(const NodeID& dst_nid, const Packet& packet);
};
}  // namespace colonio
