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
#include "kvs.hpp"

#include <cassert>

#include "command_manager.hpp"
#include "convert.hpp"
#include "definition.hpp"
#include "logger.hpp"
#include "random.hpp"
#include "scheduler.hpp"
#include "utils.hpp"
#include "value_impl.hpp"

namespace colonio {
static const unsigned int NUM_ACCEPTOR = 3;
static const unsigned int NUM_MAJORITY = 2;

static const uint32_t SET_RESPONSE_REASON_PROPOSER_CHANGED   = 1;
static const uint32_t SET_RESPONSE_REASON_PROHIBIT_OVERWRITE = 2;
static const uint32_t SET_RESPONSE_REASON_COLLISION          = 3;

static const NodeID MEMBER_DIFF[3] = {
    NodeID(0x4000000000000000, 0x0000000000000000),  // QUOTER
    NodeID(0x8000000000000000, 0x0000000000000000),  // HALF
    NodeID(0xC000000000000000, 0x0000000000000000),  // 3/4
};

KVSDelegate::~KVSDelegate() {
}

/* class KVSPaxos::AcceptorInfo */
KVS::AcceptorInfo::AcceptorInfo() : na(0), np(0), ia(0) {
}

KVS::AcceptorInfo::AcceptorInfo(PAXOS_N na_, PAXOS_N np_, PAXOS_N ia_, const Value& value_) :
    na(na_), np(np_), ia(ia_), value(value_) {
}

/* class KVS::ProposerInfo */
KVS::ProposerInfo::ProposerInfo() : np(0), ip(0), reset(true), processing_packet_id(PACKET_ID_NONE) {
}

KVS::ProposerInfo::ProposerInfo(PAXOS_N np_, PAXOS_N ip_, const Value& value_) :
    np(np_), ip(ip_), reset(true), value(value_), processing_packet_id(PACKET_ID_NONE) {
}

/* class KVS::CommandGet::Info */
KVS::CommandGet::Info::Info(KVS& p, std::string k, int c) :
    parent(p), key(k), count_retry(c), time_send(0), count_ng(0), is_finished(false) {
}

/* class KVS::CommandGet */
KVS::CommandGet::CommandGet(std::shared_ptr<Info> i) : Command(PacketMode::NONE), logger(i->parent.logger), info(i) {
}

void KVS::CommandGet::on_error(ErrorCode code, const std::string& message) {
  log_debug("error on packet of 'get'").map("message", message);
  info->count_ng += 1;

  procedure();
}

void KVS::CommandGet::on_response(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  if (!c.has_kvs_get_response()) {
    log_debug("an unexpected packet received").map("packet", packet);
    info->count_ng += 1;
    procedure();
    return;
  }

  const proto::KvsGetResponse& response = c.kvs_get_response();

  if (response.success()) {
    std::tuple<PAXOS_N, PAXOS_N> key = std::make_tuple(response.n(), response.i());
    if (info->ok_values.find(key) == info->ok_values.end()) {
      info->ok_values.insert(std::make_pair(key, ValueImpl::from_pb(response.value())));
      info->ok_counts.insert(std::make_pair(key, 1));
    } else {
      info->ok_counts.at(key)++;
    }

  } else {
    info->count_ng += 1;
  }

  procedure();
}

void KVS::CommandGet::procedure() {
  if (info->is_finished == true) {
    return;
  }

  int ok_sum = 0;
  for (auto ok_count : info->ok_counts) {
    if (ok_count.second >= NUM_MAJORITY) {
      info->is_finished = true;
      info->cb_on_success(info->ok_values.at(ok_count.first));
      return;
    }
    ok_sum += ok_count.second;
  }

  if (ok_sum + info->count_ng == NUM_ACCEPTOR) {
    log_debug("map get retry").map_int("count", info->count_retry).map_int("sum", ok_sum);
    info->is_finished = true;

    if (ok_sum == 0) {
      if (info->count_retry < info->parent.CONF_RETRY_MAX) {
        int64_t interval = info->time_send +
                           info->parent.random.generate_u32(
                               info->parent.CONF_RETRY_INTERVAL_MIN, info->parent.CONF_RETRY_INTERVAL_MAX) -
                           Utils::get_current_msec();
        info->parent.send_packet_get(
            info->key, info->count_retry + 1, interval, info->cb_on_success, info->cb_on_failure);
      } else {
        info->cb_on_failure(colonio_error(ErrorCode::KVS_NOT_FOUND, "could not find the key `" + info->key + "`"));
      }

    } else {
      PAXOS_N n    = 0;
      PAXOS_N i    = 0;
      Value* value = nullptr;
      for (auto& it : info->ok_values) {
        if (value == nullptr || n < std::get<0>(it.first) ||
            (n == std::get<0>(it.first) && i < std::get<1>(it.first))) {
          n     = std::get<0>(it.first);
          i     = std::get<1>(it.first);
          value = &(it.second);
        }
      }

      info->parent.send_packet_hint(info->key, *value, n, i);
      int64_t interval =
          info->time_send +
          info->parent.random.generate_u32(info->parent.CONF_RETRY_INTERVAL_MIN, info->parent.CONF_RETRY_INTERVAL_MAX) -
          Utils::get_current_msec();
      info->parent.send_packet_get(
          std::move(info->key), info->count_retry + 1, interval, info->cb_on_success, info->cb_on_failure);
    }
  }
}

/* class KVS::CommandSet::Info */
KVS::CommandSet::Info::Info(
    KVS& p, const std::string& k, const Value& v, const std::function<void()> cb_on_success_,
    const std::function<void(const Error&)> cb_on_failure_, const uint32_t& o) :
    parent(p), key(k), value(v), cb_on_success(cb_on_success_), cb_on_failure(cb_on_failure_), opt(o) {
}

/* class KVS::CommandSet */
KVS::CommandSet::CommandSet(std::unique_ptr<KVS::CommandSet::Info> i) :
    Command(PacketMode::NONE), logger(i->parent.logger), info(std::move(i)) {
}

void KVS::CommandSet::on_error(ErrorCode code, const std::string& message) {
  info->cb_on_failure(colonio_error(code, message));
}

void KVS::CommandSet::on_response(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  if (!c.has_kvs_set_response()) {
    log_debug("an unexpected packet received").map("packet", packet);
    info->cb_on_failure(colonio_error(ErrorCode::SYSTEM_UNEXPECTED_PACKET, "an unexpected packet received"));
    return;
  }

  const proto::KvsSetResponse& response = c.kvs_set_response();

  if (response.success()) {
    info->cb_on_success();
    return;
  }

  // failed responses.
  switch (response.reason()) {
    case SET_RESPONSE_REASON_PROPOSER_CHANGED: {
      info->parent.send_packet_set(std::move(info));
    } break;

    case SET_RESPONSE_REASON_PROHIBIT_OVERWRITE: {
      info->cb_on_failure(colonio_error(ErrorCode::KVS_PROHIBIT_OVERWRITE, "failed to set value"));
    } break;

    case SET_RESPONSE_REASON_COLLISION: {
      info->cb_on_failure(colonio_error(ErrorCode::KVS_COLLISION, "failed to set value"));
    } break;

    default: {
      info->cb_on_failure(colonio_error(ErrorCode::UNDEFINED, "bug?"));
    } break;
  }
}

/* class KVS::CommandPrepare::Reply */
KVS::CommandPrepare::Reply::Reply(const NodeID& src_nid_, PAXOS_N n_, PAXOS_N i_, bool is_success_) :
    src_nid(src_nid_), n(n_), i(i_), is_success(is_success_) {
}

/* class KVS::CommandPrepare::Info */
KVS::CommandPrepare::Info::Info(KVS& p, std::unique_ptr<const Packet> r, const std::string& k, uint32_t o) :
    packet_reply(std::move(r)), key(k), n_max(0), i_max(0), opt(o), parent(p), has_value(false), is_finished(false) {
}

KVS::CommandPrepare::Info::~Info() {
  if (!is_finished) {
    auto proposer_it = parent.proposer_infos.find(key);
    if (proposer_it != parent.proposer_infos.end()) {
      ProposerInfo& proposer        = proposer_it->second;
      proposer.processing_packet_id = PACKET_ID_NONE;
    }
  }
}

/* class KVS::CommandPrepare */
KVS::CommandPrepare::CommandPrepare(std::shared_ptr<KVS::CommandPrepare::Info> i) :
    Command(PacketMode::NONE), logger(i->parent.logger), info(i) {
}

void KVS::CommandPrepare::on_error(ErrorCode code, const std::string& message) {
  log_debug("error on packet of 'prepare'").map("message", message);
  info->replies.push_back(Reply(NodeID::NONE, 0, 0, false));

  procedure();
}

void KVS::CommandPrepare::on_response(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  if (!c.has_kvs_prepare_response()) {
    log_debug("an unexpected packet received").map("packet", packet);
    info->replies.push_back(Reply(NodeID::NONE, 0, 0, false));
    procedure();
    return;
  }

  const proto::KvsPrepareResponse& response = c.kvs_prepare_response();

  if (response.success()) {
    info->replies.push_back(Reply(packet.src_nid, response.n(), response.i(), true));

  } else {
    info->replies.push_back(Reply(packet.src_nid, response.n(), 0, false));
    info->has_value = true;
  }

  procedure();
}

void KVS::CommandPrepare::procedure() {
  if (info->is_finished) {
    return;
  }

  if ((info->opt & Colonio::KVS_PROHIBIT_OVERWRITE) != 0 && info->has_value) {
    std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
    proto::KvsSetResponse& response               = *content->mutable_kvs_set_response();
    response.set_success(false);
    response.set_reason(SET_RESPONSE_REASON_PROHIBIT_OVERWRITE);

    info->parent.command_manager.send_response(*info->packet_reply, std::move(content));
    return;
  }

  auto proposer_it = info->parent.proposer_infos.find(info->key);
  if (proposer_it == info->parent.proposer_infos.end()) {
    switch (info->packet_reply->content->as_proto().Content_case()) {
      case proto::PacketContent::kKvsSet: {
        std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
        proto::KvsSetResponse& response               = *content->mutable_kvs_set_response();
        response.set_success(false);
        response.set_reason(SET_RESPONSE_REASON_PROPOSER_CHANGED);

        info->parent.command_manager.send_response(*info->packet_reply, std::move(content));
      } break;

      case proto::PacketContent::kKvsHint: {
        // Do nothing.
      } break;

      default:
        assert(false);
    }
    return;
  }
  ProposerInfo& proposer = proposer_it->second;

  for (const auto& src : info->replies) {
    if (src.is_success) {
      for (auto& target : info->replies) {
        if (src.src_nid == target.src_nid && src.n == target.n) {
          target.is_success = true;
        }
      }
    }
  }
  unsigned int count_ok = 0;
  unsigned int count_ng = 0;
  for (const auto& it : info->replies) {
    if (it.is_success) {
      count_ok++;
      if (it.i > info->i_max) {
        info->i_max = it.i;
      }

    } else {
      count_ng++;
      if (it.n > info->n_max) {
        info->n_max = it.n;
      }
    }
  }

  if (count_ok >= NUM_MAJORITY) {
    info->is_finished = true;
    proposer.ip       = info->i_max + 1;

    info->parent.send_packet_accept(proposer, std::move(info->packet_reply), std::move(info->key), info->opt);

  } else if (count_ng >= NUM_MAJORITY) {
    info->is_finished = true;
    proposer.np       = info->n_max + 1;
    proposer.reset    = true;

    info->parent.send_packet_prepare(proposer, std::move(info->packet_reply), std::move(info->key), info->opt);
  }
}

/* class KVS::CommandAccept::Reply */
KVS::CommandAccept::Reply::Reply(const NodeID& src_nid_, PAXOS_N n_, PAXOS_N i_, bool is_success_) :
    src_nid(src_nid_), n(n_), i(i_), is_success(is_success_) {
}

/* class KVS::CommandAccept::Info */
KVS::CommandAccept::Info::Info(KVS& p, std::unique_ptr<const Packet> r, const std::string& k, uint32_t o) :
    packet_reply(std::move(r)), key(k), n_max(0), i_max(0), opt(o), parent(p), has_value(false), is_finished(false) {
}

KVS::CommandAccept::Info::~Info() {
  if (!is_finished) {
    auto proposer_it = parent.proposer_infos.find(key);
    if (proposer_it != parent.proposer_infos.end()) {
      ProposerInfo& proposer        = proposer_it->second;
      proposer.processing_packet_id = PACKET_ID_NONE;
    }
  }
}

/* class KVS::CommandAccept */
KVS::CommandAccept::CommandAccept(std::shared_ptr<KVS::CommandAccept::Info> i) :
    Command(PacketMode::NONE), logger(i->parent.logger), info(i) {
}

void KVS::CommandAccept::on_error(ErrorCode code, const std::string& message) {
  log_debug("error on packet of 'accept'").map("message", message);
  info->replies.push_back(Reply(NodeID::NONE, 0, 0, false));

  procedure();
}

void KVS::CommandAccept::on_response(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  if (!c.has_kvs_accept_response()) {
    log_debug("an unexpected packet received").map("packet", packet);
    return;
  }

  const proto::KvsAcceptResponse& response = c.kvs_accept_response();

  if (response.success()) {
    info->replies.push_back(Reply(packet.src_nid, response.n(), response.i(), true));
  } else {
    info->replies.push_back(Reply(packet.src_nid, response.n(), response.i(), false));
    info->has_value = true;
  }

  procedure();
}

void KVS::CommandAccept::procedure() {
  if (info->is_finished) {
    return;
  }

  if ((info->opt & Colonio::KVS_PROHIBIT_OVERWRITE) != 0 && info->has_value) {
    std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
    proto::KvsSetResponse& response               = *content->mutable_kvs_set_response();
    response.set_success(false);
    response.set_reason(SET_RESPONSE_REASON_PROHIBIT_OVERWRITE);

    info->parent.command_manager.send_response(*info->packet_reply, std::move(content));
    return;
  }

  auto proposer_it = info->parent.proposer_infos.find(info->key);
  if (proposer_it == info->parent.proposer_infos.end()) {
    switch (info->packet_reply->content->as_proto().Content_case()) {
      case proto::PacketContent::kKvsSet: {
        std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
        proto::KvsSetResponse& response               = *content->mutable_kvs_set_response();
        response.set_success(false);
        response.set_reason(SET_RESPONSE_REASON_PROPOSER_CHANGED);

        info->parent.command_manager.send_response(*info->packet_reply, std::move(content));
      } break;

      case proto::PacketContent::kKvsHint: {
        // Do nothing.
      } break;

      default:
        assert(false);
    }
    return;
  }
  ProposerInfo& proposer = proposer_it->second;

  for (const auto& src : info->replies) {
    if (src.is_success) {
      for (auto& target : info->replies) {
        if (src.src_nid == target.src_nid && src.n == target.n && src.i == target.i) {
          target.is_success = true;
        }
      }
    }
  }
  unsigned int count_ok = 0;
  unsigned int count_ng = 0;
  for (const auto& it : info->replies) {
    if (it.is_success) {
      count_ok++;
      if (it.i > info->i_max) {
        info->i_max = it.i;
      }

    } else {
      count_ng++;
      if (it.n > info->n_max) {
        info->n_max = it.n;
      }
    }
  }

  if (count_ok >= NUM_MAJORITY) {
    assert(proposer.processing_packet_id == info->packet_reply->id || proposer.processing_packet_id == PACKET_ID_NONE);
    info->is_finished             = true;
    proposer.ip                   = info->i_max + 1;
    proposer.reset                = false;
    proposer.processing_packet_id = PACKET_ID_NONE;

    switch (info->packet_reply->content->as_proto().Content_case()) {
      case proto::PacketContent::kKvsSet: {
        std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
        proto::KvsSetResponse& response               = *content->mutable_kvs_set_response();
        response.set_success(true);
        response.set_reason(0);

        info->parent.command_manager.send_response(*info->packet_reply, std::move(content));
      } break;

      case proto::PacketContent::kKvsHint: {
        // Do nothing.
      } break;

      default:
        assert(false);
    }

  } else if (count_ng >= NUM_MAJORITY) {
    info->is_finished = true;
    proposer.np       = info->n_max + 1;
    proposer.reset    = true;

    info->parent.send_packet_prepare(proposer, std::move(info->packet_reply), std::move(info->key), info->opt);
  }
}

KVS::KVS(Logger& l, Random& r, Scheduler& s, CommandManager& c, KVSDelegate& d, const picojson::object& config) :
    CONF_RETRY_MAX(Utils::get_json<double>(config, "retryMax", KVS_RETRY_MAX)),
    CONF_RETRY_INTERVAL_MIN(Utils::get_json<double>(config, "retryIntervalMin", KVS_RETRY_INTERVAL_MIN)),
    CONF_RETRY_INTERVAL_MAX(Utils::get_json<double>(config, "retryIntervalMax", KVS_RETRY_INTERVAL_MAX)),
    logger(l),
    random(r),
    scheduler(s),
    command_manager(c),
    delegate(d) {
  command_manager.set_handler(
      proto::PacketContent::kKvsGet, std::bind(&KVS::recv_packet_get, this, std::placeholders::_1));
  command_manager.set_handler(
      proto::PacketContent::kKvsSet, std::bind(&KVS::recv_packet_set, this, std::placeholders::_1));
  command_manager.set_handler(
      proto::PacketContent::kKvsPrepare, std::bind(&KVS::recv_packet_prepare, this, std::placeholders::_1));
  command_manager.set_handler(
      proto::PacketContent::kKvsAccept, std::bind(&KVS::recv_packet_accept, this, std::placeholders::_1));
  command_manager.set_handler(
      proto::PacketContent::kKvsHint, std::bind(&KVS::recv_packet_hint, this, std::placeholders::_1));
  command_manager.set_handler(
      proto::PacketContent::kKvsBalanceAcceptor,
      std::bind(&KVS::recv_packet_balance_acceptor, this, std::placeholders::_1));
  command_manager.set_handler(
      proto::PacketContent::kKvsBalanceProposer,
      std::bind(&KVS::recv_packet_balance_proposer, this, std::placeholders::_1));
}

KVS::~KVS() {
}

std::shared_ptr<std::map<std::string, Value>> KVS::get_local_data() {
  std::shared_ptr<std::map<std::string, Value>> data = std::make_shared<std::map<std::string, Value>>();

  for (auto& record : proposer_infos) {
    data->insert(std::make_pair(record.first, record.second.value));
  }

  return data;
}

void KVS::get(
    const std::string& key, std::function<void(const Value&)>&& on_success,
    std::function<void(const Error&)>&& on_failure) {
  send_packet_get(key, 0, 0, on_success, on_failure);
}

void KVS::set(
    const std::string& key, const Value& value, uint32_t opt, std::function<void()>&& on_success,
    std::function<void(const Error&)>&& on_failure) {
  std::unique_ptr<CommandSet::Info> info =
      std::make_unique<CommandSet::Info>(*this, key, value, on_success, on_failure, opt);
  send_packet_set(std::move(info));
}

void KVS::balance_records(const NodeID& prev_nid, const NodeID& next_nid) {
  auto acceptor_it = acceptor_infos.begin();
  while (acceptor_it != acceptor_infos.end()) {
    const auto k_m = acceptor_it->first;
    if (!check_key_acceptor(k_m.first, k_m.second)) {
      send_packet_balance_acceptor(k_m.first, k_m.second, acceptor_it->second);
      acceptor_it = acceptor_infos.erase(acceptor_it);

    } else {
      acceptor_it++;
    }
  }

  auto proposer_it = proposer_infos.begin();
  while (proposer_it != proposer_infos.end()) {
    const std::string& key = proposer_it->first;
    if (!check_key_proposer(key)) {
      send_packet_balance_proposer(key, proposer_it->second);
      proposer_it = proposer_infos.erase(proposer_it);

    } else {
      proposer_it++;
    }
  }

#ifndef NDEBUG
  debug_on_change_set();
#endif
}

bool KVS::check_key_acceptor(const std::string& key, uint32_t member_idx) {
  NodeID hash = NodeID::make_hash_from_str(key, salt);
  assert(member_idx < (sizeof(MEMBER_DIFF) / sizeof(MEMBER_DIFF[0])));
  hash += MEMBER_DIFF[member_idx];
  return delegate.kvs_on_check_covered_range(hash);
}

bool KVS::check_key_proposer(const std::string& key) {
  NodeID hash = NodeID::make_hash_from_str(key, salt);
  return delegate.kvs_on_check_covered_range(hash);
}

#ifndef NDEBUG
void KVS::debug_on_change_set() {
  picojson::array a;
  for (auto& pi : proposer_infos) {
    picojson::object o;
    o.insert(std::make_pair("key", picojson::value(pi.first)));
    o.insert(std::make_pair("value", picojson::value(ValueImpl::to_str(pi.second.value))));
    o.insert(std::make_pair("hash", picojson::value(NodeID::make_hash_from_str(pi.first, salt).to_str())));
    a.push_back(picojson::value(o));
  }

  log_debug("map paxos proposer").map("map", picojson::value(a));
}
#endif

void KVS::recv_packet_accept(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  assert(c.has_kvs_accept());
  const proto::KvsAccept& content = c.kvs_accept();
  // const uint32_t opt              = content.opt();
  const std::string& key    = content.key();
  const uint32_t member_idx = content.member_idx();
  Value value               = ValueImpl::from_pb(content.value());
  const PAXOS_N n           = content.n();
  const PAXOS_N i           = content.i();

  auto acceptor_it = acceptor_infos.find(std::make_pair(key, member_idx));
  if (acceptor_it == acceptor_infos.end()) {
    if (check_key_acceptor(key, member_idx)) {
      bool r;
      std::tie(acceptor_it, r) = acceptor_infos.insert(std::make_pair(std::make_pair(key, member_idx), AcceptorInfo()));
#ifndef NDEBUG
      debug_on_change_set();
#endif

    } else {
      // ignore
      log_debug("receive 'accept' packet at wrong node").map("key", key);
      return;
    }
  }

  AcceptorInfo& acceptor = acceptor_it->second;
  if (n >= acceptor.np) {
    if (packet.src_nid != acceptor.last_nid) {
      acceptor.ia = 0;
    }
    if (i > acceptor.ia) {
      acceptor.na = acceptor.np = n;
      acceptor.ia               = i;
      acceptor.value            = value;
      acceptor.last_nid         = packet.src_nid;

      std::unique_ptr<proto::PacketContent> response = std::make_unique<proto::PacketContent>();
      proto::KvsAcceptResponse& param                = *response->mutable_kvs_accept_response();
      param.set_success(true);
      param.set_n(acceptor.np);
      param.set_i(acceptor.ia);
      command_manager.send_response(packet, std::move(response));
      return;
    }
  }

  std::unique_ptr<proto::PacketContent> response = std::make_unique<proto::PacketContent>();
  proto::KvsAcceptResponse& param                = *response->mutable_kvs_accept_response();
  param.set_success(false);
  param.set_n(acceptor.np);
  param.set_i(acceptor.ia);
  command_manager.send_response(packet, std::move(response));
}

void KVS::recv_packet_hint(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  assert(c.has_kvs_hint());
  const proto::KvsHint& content = c.kvs_hint();
  const std::string& key        = content.key();
  Value value                   = ValueImpl::from_pb(content.value());
  const PAXOS_N n               = content.n();
  // const PAXOS_N i               = content.i();

  if (check_key_proposer(key)) {
    auto proposer_it = proposer_infos.find(key);
    if (proposer_it == proposer_infos.end()) {
      bool r;
      std::tie(proposer_it, r) = proposer_infos.insert(std::make_pair(key, ProposerInfo()));
      assert(r);
    }

    ProposerInfo& proposer = proposer_it->second;
    if (n > proposer.np) {
      proposer.np += 1;
      proposer.value = value;
    }
    // Same logic with set command.
    if (proposer.reset) {
      log_debug("prepare").map_u32("id", packet.id);
      send_packet_prepare(proposer, packet.make_copy(), key, 0x00);

    } else {
      log_debug("accept").map_u32("id", packet.id);
      send_packet_accept(proposer, packet.make_copy(), key, 0x00);
    }

  } else {
    // ignore
    log_debug("receive 'hint' packet at wrong node").map("key", key);
    return;
  }
}

void KVS::recv_packet_balance_acceptor(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  assert(c.has_kvs_balance_acceptor());
  const proto::KvsBalanceAcceptor& content = c.kvs_balance_acceptor();
  const std::string& key                   = content.key();
  const uint32_t member_idx                = content.member_idx();
  Value value                              = ValueImpl::from_pb(content.value());
  const PAXOS_N na                         = content.na();
  const PAXOS_N np                         = content.np();
  const PAXOS_N ia                         = content.ia();

  if (check_key_acceptor(key, member_idx)) {
    auto acceptor_it = acceptor_infos.find(std::make_pair(key, member_idx));
    if (acceptor_it == acceptor_infos.end()) {
      acceptor_infos.insert(std::make_pair(std::make_pair(key, member_idx), AcceptorInfo(na, np, ia, value)));

    } else {
      AcceptorInfo& acceptor = acceptor_it->second;
      if (np > acceptor.np || (np == acceptor.np && ia > acceptor.ia)) {
        acceptor.na       = na;
        acceptor.np       = np;
        acceptor.ia       = ia;
        acceptor.value    = value;
        acceptor.last_nid = NodeID::NONE;
      }
    }
  }
}

void KVS::recv_packet_balance_proposer(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  assert(c.has_kvs_balance_proposer());
  const proto::KvsBalanceProposer& content = c.kvs_balance_proposer();
  const std::string key                    = content.key();
  Value value                              = ValueImpl::from_pb(content.value());
  const PAXOS_N np                         = content.np();
  const PAXOS_N ip                         = content.ip();

  if (check_key_proposer(key)) {
    auto proposer_it = proposer_infos.find(key);
    if (proposer_it == proposer_infos.end()) {
      proposer_infos.insert(std::make_pair(key, ProposerInfo(np, ip, value)));
#ifndef NDEBUG
      debug_on_change_set();
#endif

    } else {
      ProposerInfo& proposer = proposer_it->second;
      if (proposer.processing_packet_id == PACKET_ID_NONE &&
          (np > proposer.np || (np == proposer.np && ip > proposer.ip))) {
        proposer.np    = np;
        proposer.ip    = ip;
        proposer.value = value;
      }
    }
  }
}

void KVS::recv_packet_get(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  assert(c.has_kvs_get());
  const proto::KvsGet& content = c.kvs_get();
  const std::string& key       = content.key();
  const uint32_t member_idx    = content.member_idx();

  auto acceptor_it = acceptor_infos.find(std::make_pair(key, member_idx));
  if (acceptor_it == acceptor_infos.end() || acceptor_it->second.na == 0) {
    // TODO: Search data from another acceptors.
    std::unique_ptr<proto::PacketContent> response = std::make_unique<proto::PacketContent>();
    proto::KvsGetResponse& param                   = *response->mutable_kvs_get_response();
    param.set_success(false);
    command_manager.send_response(packet, std::move(response));

  } else {
    AcceptorInfo& acceptor_info                    = acceptor_it->second;
    std::unique_ptr<proto::PacketContent> response = std::make_unique<proto::PacketContent>();
    proto::KvsGetResponse& param                   = *response->mutable_kvs_get_response();
    param.set_success(true);
    param.set_n(acceptor_info.na);
    param.set_i(acceptor_info.ia);
    ValueImpl::to_pb(param.mutable_value(), acceptor_info.value);
    command_manager.send_response(packet, std::move(response));
  }
}

void KVS::recv_packet_prepare(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  assert(c.has_kvs_prepare());
  const proto::KvsPrepare& content = c.kvs_prepare();
  const uint32_t opt               = content.opt();
  const std::string& key           = content.key();
  const uint32_t member_idx        = content.member_idx();
  const PAXOS_N n                  = content.n();

  auto acceptor_it = acceptor_infos.find(std::make_pair(key, member_idx));
  if (acceptor_it == acceptor_infos.end()) {
    if (check_key_acceptor(key, member_idx)) {
      bool r;
      std::tie(acceptor_it, r) = acceptor_infos.insert(std::make_pair(std::make_pair(key, member_idx), AcceptorInfo()));
      assert(r);

    } else {
      // dont reply success or failure packet when the node doesn't have any value.
      log_debug("receive 'prepare' packet at wrong node").map("key", key);
      return;
    }

  } else if ((opt & Colonio::KVS_PROHIBIT_OVERWRITE) != 0) {
    // This algorithm is not perfect to detect existing value.
    AcceptorInfo& acceptor                         = acceptor_it->second;
    std::unique_ptr<proto::PacketContent> response = std::make_unique<proto::PacketContent>();
    proto::KvsPrepareResponse& param               = *response->mutable_kvs_prepare_response();
    param.set_success(false);
    param.set_n(acceptor.np);
    command_manager.send_response(packet, std::move(response));
    return;
  }

  AcceptorInfo& acceptor = acceptor_it->second;
  if (n > acceptor.np) {
    acceptor.np = n;
    PAXOS_N i;

    if (packet.src_nid != acceptor.last_nid) {
      i = 0;
    } else {
      i = acceptor.ia;
    }

    std::unique_ptr<proto::PacketContent> response = std::make_unique<proto::PacketContent>();
    proto::KvsPrepareResponse& param               = *response->mutable_kvs_prepare_response();
    param.set_success(true);
    param.set_n(n);
    param.set_i(i);
    command_manager.send_response(packet, std::move(response));

  } else {
    std::unique_ptr<proto::PacketContent> response = std::make_unique<proto::PacketContent>();
    proto::KvsPrepareResponse& param               = *response->mutable_kvs_prepare_response();
    param.set_success(false);
    param.set_n(acceptor.np);
    command_manager.send_response(packet, std::move(response));
  }
}

void KVS::recv_packet_set(const Packet& packet) {
  const proto::PacketContent& c = packet.content->as_proto();
  assert(c.has_kvs_set());
  const proto::KvsSet& content = c.kvs_set();
  const std::string& key       = content.key();
  Value value                  = ValueImpl::from_pb(content.value());
  const uint32_t opt           = content.opt();

  auto proposer_it = proposer_infos.find(key);
  if (proposer_it == proposer_infos.end()) {
    if (check_key_proposer(key)) {
      bool r;
      std::tie(proposer_it, r) = proposer_infos.insert(std::make_pair(key, ProposerInfo()));
      assert(r);
#ifndef NDEBUG
      debug_on_change_set();
#endif

    } else {
      // ignore
      log_debug("receive 'set' packet at wrong node").map("key", key);
      return;
    }

  } else if ((opt & Colonio::KVS_PROHIBIT_OVERWRITE) != 0) {
    std::unique_ptr<proto::PacketContent> response = std::make_unique<proto::PacketContent>();
    proto::KvsSetResponse& param                   = *response->mutable_kvs_set_response();
    param.set_success(false);
    param.set_reason(SET_RESPONSE_REASON_PROHIBIT_OVERWRITE);
    command_manager.send_response(packet, std::move(response));
    return;
  }

  ProposerInfo& proposer = proposer_it->second;
  proposer.np += 1;
  proposer.value = value;

  if (proposer.processing_packet_id != PACKET_ID_NONE) {
    // TODO: switch by flag
    std::unique_ptr<proto::PacketContent> response = std::make_unique<proto::PacketContent>();
    proto::KvsSetResponse& param                   = *response->mutable_kvs_set_response();
    param.set_success(false);
    param.set_reason(SET_RESPONSE_REASON_COLLISION);
    command_manager.send_response(packet, std::move(response));

  } else {
    proposer.processing_packet_id = packet.id;

    // Same logic with hint command.
    if (proposer.reset) {
      send_packet_prepare(proposer, packet.make_copy(), key, opt);

    } else {
      send_packet_accept(proposer, packet.make_copy(), key, opt);
    }
  }
}

void KVS::send_packet_accept(
    ProposerInfo& proposer, std::unique_ptr<const Packet> packet_reply, const std::string& key, uint32_t opt) {
  std::shared_ptr<CommandAccept::Info> accept_info =
      std::make_shared<CommandAccept::Info>(*this, std::move(packet_reply), std::move(key), opt);

  NodeID acceptor_nid = NodeID::make_hash_from_str(accept_info->key, salt);
  for (unsigned int i = 0; i < NUM_ACCEPTOR; i++) {
    std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
    proto::KvsAccept& param                       = *content->mutable_kvs_accept();
    param.set_key(accept_info->key);
    param.set_member_idx(i);
    ValueImpl::to_pb(param.mutable_value(), proposer.value);
    param.set_n(proposer.np);
    param.set_i(proposer.ip);
    param.set_opt(opt);

    acceptor_nid += NodeID::QUARTER;
    std::shared_ptr<Command> command = std::make_shared<CommandAccept>(accept_info);
    command_manager.send_packet(acceptor_nid, std::move(content), command);
  }
}

void KVS::send_packet_balance_acceptor(const std::string& key, uint32_t member_idx, const AcceptorInfo& acceptor) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::KvsBalanceAcceptor& param              = *content->mutable_kvs_balance_acceptor();
  param.set_key(key);
  param.set_member_idx(member_idx);
  ValueImpl::to_pb(param.mutable_value(), acceptor.value);
  param.set_na(acceptor.na);
  param.set_np(acceptor.np);
  param.set_ia(acceptor.ia);

  NodeID acceptor_nid = NodeID::make_hash_from_str(key, salt);
  assert(member_idx < (sizeof(MEMBER_DIFF) / sizeof(MEMBER_DIFF[0])));
  acceptor_nid += MEMBER_DIFF[member_idx];
  command_manager.send_packet_one_way(acceptor_nid, 0, std::move(content));
}

void KVS::send_packet_balance_proposer(const std::string& key, const ProposerInfo& proposer) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::KvsBalanceProposer& param              = *content->mutable_kvs_balance_proposer();
  param.set_key(key);
  ValueImpl::to_pb(param.mutable_value(), proposer.value);
  param.set_np(proposer.np);
  param.set_ip(proposer.ip);

  NodeID proposer_nid = NodeID::make_hash_from_str(key, salt);
  command_manager.send_packet_one_way(proposer_nid, 0, std::move(content));
}

void KVS::send_packet_get(
    const std::string& key, int count_retry, int64_t interval, const std::function<void(const Value&)> on_success,
    const std::function<void(const Error&)> on_failure) {
  std::shared_ptr<CommandGet::Info> info = std::make_unique<CommandGet::Info>(*this, std::move(key), count_retry);
  info->cb_on_success                    = on_success;
  info->cb_on_failure                    = on_failure;

  if (interval < 0) {
    interval = 0;
  }

  auto f = [this, info]() {
    info->time_send = Utils::get_current_msec();

    NodeID acceptor_nid = NodeID::make_hash_from_str(info->key, salt);

    for (unsigned int i = 0; i < NUM_ACCEPTOR; i++) {
      std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
      proto::KvsGet& param                          = *content->mutable_kvs_get();
      param.set_key(info->key);
      param.set_member_idx(i);

      acceptor_nid += NodeID::QUARTER;
      std::shared_ptr<Command> command = std::make_shared<CommandGet>(info);

      command_manager.send_packet(acceptor_nid, std::move(content), command);
    }
  };

  if (interval == 0) {
    f();
  } else {
    scheduler.add_task(this, f, interval);
  }
}

void KVS::send_packet_hint(const std::string& key, const Value& value, PAXOS_N n, PAXOS_N i) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::KvsHint& param                         = *content->mutable_kvs_hint();
  param.set_key(key);
  ValueImpl::to_pb(param.mutable_value(), value);
  param.set_n(n);
  param.set_i(i);

  NodeID proposer_nid = NodeID::make_hash_from_str(key, salt);
  command_manager.send_packet_one_way(proposer_nid, 0, std::move(content));
}

void KVS::send_packet_prepare(
    ProposerInfo& proposer, std::unique_ptr<const Packet> packet_reply, const std::string& key, uint32_t opt) {
  std::shared_ptr<CommandPrepare::Info> prepare_info =
      std::make_unique<CommandPrepare::Info>(*this, std::move(packet_reply), std::move(key), opt);

  NodeID acceptor_nid = NodeID::make_hash_from_str(prepare_info->key, salt);
  for (unsigned int i = 0; i < NUM_ACCEPTOR; i++) {
    std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
    proto::KvsPrepare& param                      = *content->mutable_kvs_prepare();
    param.set_key(prepare_info->key);
    param.set_member_idx(i);
    param.set_n(proposer.np);
    param.set_opt(opt);

    acceptor_nid += NodeID::QUARTER;
    std::shared_ptr<Command> command = std::make_shared<CommandPrepare>(prepare_info);
    command_manager.send_packet(acceptor_nid, std::move(content), command);
  }
}

void KVS::send_packet_set(std::unique_ptr<CommandSet::Info> info) {
  std::unique_ptr<proto::PacketContent> content = std::make_unique<proto::PacketContent>();
  proto::KvsSet& param                          = *content->mutable_kvs_set();
  param.set_key(info->key);
  ValueImpl::to_pb(param.mutable_value(), info->value);
  param.set_opt(info->opt);

  NodeID proposer_nid              = NodeID::make_hash_from_str(info->key, salt);
  std::shared_ptr<Command> command = std::make_shared<CommandSet>(std::move(info));
  command_manager.send_packet(proposer_nid, std::move(content), command);
}
}  // namespace colonio
