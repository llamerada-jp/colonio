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
#include "colonio_impl.hpp"

#include <cassert>

#include "context.hpp"
#include "convert.hpp"
#include "coord_system_sphere.hpp"
#include "logger.hpp"
#include "map_paxos/map_paxos_api.hpp"
#include "pubsub_2d/pubsub2d_api.hpp"
#include "routing_1d.hpp"
#include "scheduler.hpp"
#include "utils.hpp"

namespace colonio {
ColonioImpl::ColonioImpl(Context& context, APIDelegate& api_delegate_, APIBundler& api_bundler_) :
    APIBase(context, api_delegate_, APIChannel::COLONIO),
    api_delegate(api_delegate_),
    api_bundler(api_bundler_),
    module_bundler(*this, *this, *this),
    api_connect_id(0),
    enable_retry(true),
    node_accessor(nullptr),
    routing(nullptr),
    node_status(LinkStatus::OFFLINE),
    seed_status(LinkStatus::OFFLINE) {
}

ColonioImpl::~ColonioImpl() {
}

LinkStatus::Type ColonioImpl::get_status() {
  return context.link_status;
}

void ColonioImpl::api_on_recv_call(const api::Call& call) {
  switch (call.param_case()) {
    case api::Call::ParamCase::kColonioConnect:
      api_connect(call.id(), call.colonio_connect());
      break;

    case api::Call::ParamCase::kColonioDisconnect:
      api_disconnect(call.id());
      break;

    case api::Call::ParamCase::kColonioGetLocalNid:
      api_get_local_nid(call.id());
      break;

    case api::Call::ParamCase::kColonioSetPosition:
      api_set_position(call.id(), call.colonio_set_position());
      break;

    default:
      colonio_fatal("Called incorrect colonio API entry : %d", call.param_case());
      break;
  }
}

void ColonioImpl::module_do_send_packet(ModuleBase& module, std::unique_ptr<const Packet> packet) {
  relay_packet(std::move(packet), false);
}

void ColonioImpl::module_do_relay_packet(
    ModuleBase& module, const NodeID& dst_nid, std::unique_ptr<const Packet> packet) {
  assert(!dst_nid.is_special() || dst_nid == NodeID::NEXT);

  node_accessor->relay_packet(dst_nid, std::move(packet));
}

void ColonioImpl::node_accessor_on_change_online_links(NodeAccessor& na, const std::set<NodeID> nids) {
  if (routing) {
    routing->on_change_online_links(nids);
  }

#ifndef NDEBUG
  {
    picojson::array a;
    for (auto nid : nids) {
      a.push_back(picojson::value(nid.to_str()));
    }
    context.debug_event(DebugEvent::LINKS, picojson::value(a));
  }
#endif
}

void ColonioImpl::node_accessor_on_change_status(NodeAccessor& na, LinkStatus::Type status) {
  context.scheduler.add_timeout_task(
      this, [this, status]() { on_change_accessor_status(seed_accessor->get_status(), status); }, 0);
}

void ColonioImpl::node_accessor_on_recv_packet(
    NodeAccessor& na, const NodeID& nid, std::unique_ptr<const Packet> packet) {
  if (routing) {
    routing->on_recv_packet(nid, *packet);
  }
  relay_packet(std::move(packet), false);
}

void ColonioImpl::routing_do_connect_node(Routing& routing, const NodeID& nid) {
  if (node_accessor->get_status() == LinkStatus::ONLINE) {
    node_accessor->connect_link(nid);
  }
}

void ColonioImpl::routing_do_disconnect_node(Routing& routing, const NodeID& nid) {
  if (node_accessor->get_status() == LinkStatus::ONLINE) {
    node_accessor->disconnect_link(nid);
  }
}

void ColonioImpl::routing_do_connect_seed(Routing& route) {
  if (enable_retry) {
    seed_accessor->connect();
  }
}

void ColonioImpl::routing_do_disconnect_seed(Routing& route) {
  seed_accessor->disconnect();
}

void ColonioImpl::routing_on_module_1d_change_nearby(Routing& routing, const NodeID& prev_nid, const NodeID& next_nid) {
  context.scheduler.add_timeout_task(
      this, [this, prev_nid, next_nid]() { module_bundler.module_1d_on_change_nearby(prev_nid, next_nid); }, 0);
}

const NodeID& ColonioImpl::module_2d_do_get_relay_nid(Module2D& module_2d, const Coordinate& position) {
  return routing->get_relay_nid_2d(position);
}

void ColonioImpl::routing_on_module_2d_change_nearby(Routing& routing, const std::set<NodeID>& nids) {
  context.scheduler.add_timeout_task(this, [this, nids]() { module_bundler.module_2d_on_change_nearby(nids); }, 0);
}

void ColonioImpl::routing_on_module_2d_change_nearby_position(
    Routing& routing, const std::map<NodeID, Coordinate>& positions) {
  context.scheduler.add_timeout_task(
      this, [this, positions]() { module_bundler.module_2d_on_change_nearby_position(positions); }, 0);
}

void ColonioImpl::seed_accessor_on_change_status(SeedAccessor& sa, LinkStatus::Type status) {
  context.scheduler.add_timeout_task(
      this, [this, status]() { on_change_accessor_status(status, node_accessor->get_status()); }, 0);
}

void ColonioImpl::seed_accessor_on_recv_config(SeedAccessor& sa, const picojson::object& newconfig) {
  if (config.empty()) {
    config = newconfig;
    // node_accessor
    node_accessor->initialize(config);

    // coord_system
    picojson::object cs2d_config;
    if (Utils::check_json_optional(config, "coordSystem2D", &cs2d_config)) {
      const std::string& type = Utils::get_json<std::string>(cs2d_config, "type");
      if (type == "sphere") {
        context.coord_system = std::make_unique<CoordSystemSphere>(cs2d_config);
      }
    }

    // routing
    routing = std::make_unique<Routing>(
        context, *this, *this, APIChannel::COLONIO, Utils::get_json<picojson::object>(config, "routing"));
    module_bundler.registrate(routing.get(), false, false);

    // modules
    initialize_algorithms();

  } else {
    // check revision
    if (newconfig.at("revision").get<double>() != config.at("revision").get<double>()) {
      // @todo Warn new revision to the application.
    }
  }
}

void ColonioImpl::seed_accessor_on_recv_packet(SeedAccessor& sa, std::unique_ptr<const Packet> packet) {
  relay_packet(std::move(packet), true);
}

void ColonioImpl::seed_accessor_on_recv_require_random(SeedAccessor& sa) {
  if (node_accessor != nullptr) {
    node_accessor->connect_random_link();
  }
}

bool ColonioImpl::module_1d_do_check_covered_range(Module1D& module_1d, const NodeID& nid) {
  assert(routing);
  return routing->is_covered_range_1d(nid);
}

void ColonioImpl::api_connect(uint32_t id, const api::colonio::Connect& param) {
  const std::string& url   = param.url();
  const std::string& token = param.token();

  if (context.link_status != LinkStatus::OFFLINE && context.link_status != LinkStatus::CLOSING) {
    loge("Duplicate connection.");
    return;

  } else {
    logi("Connect start.(url=%s)", url.c_str());
  }

  api_connect_id = id;
  enable_retry   = true;

  seed_accessor = std::make_unique<SeedAccessor>(context, *this, url, token);
  node_accessor = std::make_unique<NodeAccessor>(context, *this, *this);
  module_bundler.registrate(node_accessor.get(), false, false);

  context.scheduler.add_timeout_task(
      this, [this]() { on_change_accessor_status(seed_accessor->get_status(), node_accessor->get_status()); }, 0);
}

void ColonioImpl::api_get_local_nid(uint32_t id) {
  std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
  reply->set_id(id);
  api::colonio::GetLocalNIDReply* param = reply->mutable_colonio_get_local_nid();
  context.local_nid.to_pb(param->mutable_local_nid());
  api_reply(std::move(reply));
}

void ColonioImpl::api_disconnect(uint32_t id) {
  enable_retry = false;
  seed_accessor->disconnect();
  node_accessor->disconnect_all([this, id]() {
    on_change_accessor_status(LinkStatus::OFFLINE, LinkStatus::OFFLINE);
    module_bundler.clear();

    node_accessor.reset();
    routing.reset();

    seed_accessor.reset();

    context.scheduler.remove_task(this);
    api_success(id);
  });
}

void ColonioImpl::api_set_position(uint32_t id, const api::colonio::SetPosition& param) {
  if (context.coord_system) {
    Coordinate pos          = Coordinate::from_pb(param.position());
    Coordinate old_position = context.get_local_position();
    context.set_local_position(pos);
    Coordinate new_position = context.get_local_position();
    if (old_position != new_position) {
      context.scheduler.add_timeout_task(
          this, [this, new_position]() { this->routing->on_change_local_position(new_position); }, 0);
      context.scheduler.add_timeout_task(
          this, [this, new_position]() { module_bundler.module_2d_on_change_local_position(new_position); }, 0);
#ifndef NDEBUG
      context.debug_event(DebugEvent::POSITION, Convert::coordinate2json(new_position));
#endif
    }

    std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
    reply->set_id(id);
    api::colonio::SetPositionReply* reply_param = reply->mutable_colonio_set_position();
    new_position.to_pb(reply_param->mutable_position());
    api_reply(std::move(reply));

  } else {
    api_failure(id, Exception::Code::CONFLICT_WITH_SETTING, "coordinate system was not enabled");
  }
}

void ColonioImpl::check_api_connect() {
  if (api_connect_id != 0 && context.link_status == LinkStatus::ONLINE && api_connect_reply) {
    assert(seed_accessor->get_auth_status() == AuthStatus::SUCCESS);
    logi("Connect success.");

    std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
    reply->set_id(api_connect_id);
    reply->set_allocated_colonio_connect(api_connect_reply.get());
    api_connect_reply.release();

    api_reply(std::move(reply));
    api_connect_id = 0;
  }
}

void ColonioImpl::initialize_algorithms() {
  const picojson::object& modules_config = Utils::get_json<picojson::object>(config, "modules", picojson::object());
  api_connect_reply                      = std::make_unique<api::colonio::ConnectReply>();

  for (auto& it : modules_config) {
    const std::string& name               = it.first;
    const picojson::object& module_config = Utils::get_json<picojson::object>(modules_config, name);
    const std::string& type               = Utils::get_json<std::string>(module_config, "type");

    api::colonio::ConnectReply_Module* module = api_connect_reply->add_modules();
    module->set_channel(Utils::get_json<double>(module_config, "channel"));
    module->set_name(name);

    if (type == "pubsub2D") {
      PubSub2DAPI::make_entry(context, api_bundler, api_delegate, module_bundler, module_config);
      module->set_type(api::colonio::ConnectReply_ModuleType_PUBSUB2D);

    } else if (type == "mapPaxos") {
      MapPaxosAPI::make_entry(context, api_bundler, api_delegate, module_bundler, module_config);
      module->set_type(api::colonio::ConnectReply_ModuleType_MAP);

    } else {
      colonio_fatal("unsupported algorithm(%s)", type.c_str());
    }
  }

  check_api_connect();
}

void ColonioImpl::on_change_accessor_status(LinkStatus::Type seed_status, LinkStatus::Type node_status) {
  logd(
      "seed(%d) node(%d) auth(%d) onlyone(%d)", seed_status, node_status, seed_accessor->get_auth_status(),
      seed_accessor->is_only_one());

  assert(seed_status != LinkStatus::CLOSING);
  assert(node_status != LinkStatus::CLOSING);

  this->seed_status = seed_status;
  this->node_status = node_status;

  LinkStatus::Type status = LinkStatus::OFFLINE;

  if (node_status == LinkStatus::ONLINE) {
    status = LinkStatus::ONLINE;

  } else if (node_status == LinkStatus::CONNECTING) {
    if (seed_accessor->is_only_one() == true) {
      status = LinkStatus::ONLINE;
    } else {
      status = LinkStatus::CONNECTING;
    }

  } else if (seed_status == LinkStatus::ONLINE) {
    if (seed_accessor->is_only_one() == true) {
      status = LinkStatus::ONLINE;
    } else {
      node_accessor->connect_init_link();
      status = LinkStatus::CONNECTING;
    }

  } else if (seed_status == LinkStatus::CONNECTING) {
    status = LinkStatus::CONNECTING;

  } else if (!seed_accessor) {
    assert(node_accessor == nullptr);

    status = LinkStatus::OFFLINE;

  } else if (seed_accessor->get_auth_status() == AuthStatus::FAILURE) {
    loge("Connect failure.");
    if (api_connect_id != 0) {
      api_failure(api_connect_id, Exception::Code::OFFLINE, "Connect failure.");
      api_connect_id = 0;
    }
    assert(false);
    context.scheduler.add_timeout_task(this, [this]() { api_disconnect(0); }, 0);
    status = LinkStatus::OFFLINE;

  } else if (enable_retry) {
    seed_accessor->connect();
    status = LinkStatus::CONNECTING;
  }

  context.link_status = status;
  check_api_connect();

  module_bundler.on_change_accessor_status(seed_status, node_status);
}

void ColonioImpl::relay_packet(std::unique_ptr<const Packet> packet, bool is_from_seed) {
  assert(packet->channel != APIChannel::NONE);
  assert(packet->module_channel != ModuleChannel::NONE);

  if ((packet->mode & PacketMode::RELAY_SEED) != 0x0) {
    if ((packet->mode & PacketMode::REPLY) != 0x0 && !is_from_seed) {
      if (seed_status == LinkStatus::ONLINE) {
        seed_accessor->relay_packet(std::move(packet));
      } else {
        assert(routing);
        node_accessor->relay_packet(std::get<0>(routing->get_route_to_seed()), std::move(packet));
      }
      return;

    } else if (packet->src_nid == context.local_nid) {
      assert(seed_status == LinkStatus::ONLINE);
      seed_accessor->relay_packet(std::move(packet));
      return;
    }
  }

  assert(routing);
  NodeID dst_nid = routing->get_relay_nid_1d(*packet);

#ifndef NDEBUG
  std::string dump = Utils::dump_packet(*packet);
#endif

  if (dst_nid == NodeID::THIS || dst_nid == context.local_nid) {
    // logd("Recv.(packet=%s)", dump.c_str());
    // Pass a packet to the target module.
    const Packet* p = packet.get();
    packet.release();
    context.scheduler.add_timeout_task(
        this,
        [this, p]() mutable {
          assert(p != nullptr);
          module_bundler.on_recv_packet(std::unique_ptr<const Packet>(p));
        },
        0);
    return;

  } else if (!dst_nid.is_special() || dst_nid == NodeID::NEXT) {
    NodeID src_nid = packet->src_nid;
    if (node_accessor->relay_packet(dst_nid, std::move(packet))) {
      if (src_nid == context.local_nid) {
        logd("Send.(dst=%s packet=%s)", dst_nid.to_str().c_str(), dump.c_str());
      } else {
        logd("Relay.(dst=%s packet=%s)", dst_nid.to_str().c_str(), dump.c_str());
      }
      return;
    }
  }

  node_accessor->update_link_status();

  logd("drop packet %s\n", dump.c_str());
}
}  // namespace colonio
