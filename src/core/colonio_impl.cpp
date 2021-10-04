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
#include "coord_system_plane.hpp"
#include "coord_system_sphere.hpp"
#include "logger.hpp"
#include "map_paxos/map_paxos_api.hpp"
#include "pubsub_2d/pubsub_2d_api.hpp"
#include "routing_1d.hpp"
#include "scheduler.hpp"
#include "utils.hpp"

namespace colonio {
ColonioImpl::ColonioImpl(Context& context, APIDelegate& api_delegate_, APIBundler& api_bundler_) :
    APIBase(context, api_delegate_, APIChannel::COLONIO),
    api_delegate(api_delegate_),
    api_bundler(api_bundler_),
    module_bundler(*this, *this, *this),
    enable_retry(true),
    api_connect_id(0),
    node_accessor(nullptr),
    routing(nullptr),
    link_status(LinkStatus::OFFLINE) {
}

ColonioImpl::~ColonioImpl() {
}

LinkStatus::Type ColonioImpl::get_status() {
  return link_status;
}

void ColonioImpl::api_on_recv_call(const api::Call& call) {
  switch (call.param_case()) {
    case api::Call::ParamCase::kColonioConnect:
      api_connect(call.id(), call.colonio_connect());
      break;

    case api::Call::ParamCase::kColonioDisconnect:
      api_disconnect(call.id());
      break;

    case api::Call::ParamCase::kColonioSetPosition:
      api_set_position(call.id(), call.colonio_set_position());
      break;

    case api::Call::ParamCase::kColonioSend:
      api_send(call.id(), call.colonio_send());
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

void ColonioImpl::node_accessor_on_change_online_links(NodeAccessor& na, const std::set<NodeID>& nids) {
  if (routing) {
    routing->on_change_online_links(nids);
  }

#ifndef NDEBUG
  {
    picojson::array a;
    for (auto nid : nids) {
      a.push_back(picojson::value(nid.to_str()));
    }
    logd("links").map("nids", picojson::value(a));
  }
#endif
}

void ColonioImpl::node_accessor_on_change_status(NodeAccessor& na) {
  context.scheduler.add_timeout_task(
      this, [this]() { on_change_accessor_status(); }, 0);
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
  context.scheduler.add_timeout_task(
      this, [this, nids]() { module_bundler.module_2d_on_change_nearby(nids); }, 0);
}

void ColonioImpl::routing_on_module_2d_change_nearby_position(
    Routing& routing, const std::map<NodeID, Coordinate>& positions) {
  context.scheduler.add_timeout_task(
      this, [this, positions]() { module_bundler.module_2d_on_change_nearby_position(positions); }, 0);
}

void ColonioImpl::seed_accessor_on_change_status(SeedAccessor& sa) {
  context.scheduler.add_timeout_task(
      this, [this]() { on_change_accessor_status(); }, 0);
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
        coord_system = std::make_unique<CoordSystemSphere>(context.random, cs2d_config);
      } else if (type == "plane") {
        coord_system = std::make_unique<CoordSystemPlane>(context.random, cs2d_config);
      }
    }

    // routing
    routing = std::make_unique<Routing>(
        context, *this, *this, APIChannel::COLONIO, coord_system.get(),
        Utils::get_json<picojson::object>(config, "routing"));
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

  if (link_status != LinkStatus::OFFLINE && link_status != LinkStatus::CLOSING) {
    loge("duplicate connection.");
    return;

  } else {
    logi("connect start").map("url", url);
  }

  api_connect_id = id;
  enable_retry   = true;

  seed_accessor = std::make_unique<SeedAccessor>(context, *this, url, token);
  node_accessor = std::make_unique<NodeAccessor>(context, *this, *this);
  module_bundler.registrate(node_accessor.get(), false, false);

  context.scheduler.add_timeout_task(
      this, [this]() { on_change_accessor_status(); }, 0);
}

void ColonioImpl::api_disconnect(uint32_t id) {
  enable_retry = false;
  seed_accessor->disconnect();
  node_accessor->disconnect_all([this, id]() {
    link_status = LinkStatus::OFFLINE;
    module_bundler.on_change_accessor_status(LinkStatus::OFFLINE, LinkStatus::OFFLINE);

    module_bundler.clear();

    node_accessor.reset();
    routing.reset();

    seed_accessor.reset();

    context.scheduler.remove_task(this);
    api_success(id);
  });
}

void ColonioImpl::api_set_position(uint32_t id, const api::colonio::SetPosition& param) {
  if (coord_system) {
    Coordinate new_position = Coordinate::from_pb(param.position());
    Coordinate old_position = coord_system->get_local_position();
    if (new_position != old_position) {
      coord_system->set_local_position(new_position);
      context.scheduler.add_timeout_task(
          this, [this, new_position]() { this->routing->on_change_local_position(new_position); }, 0);
      context.scheduler.add_timeout_task(
          this, [this, new_position]() { module_bundler.module_2d_on_change_local_position(new_position); }, 0);
      logd("current position").map("coordinate", Convert::coordinate2json(new_position));
    }

    std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
    reply->set_id(id);
    api::colonio::SetPositionReply* reply_param = reply->mutable_colonio_set_position();
    new_position.to_pb(reply_param->mutable_position());
    api_reply(std::move(reply));

  } else {
    api_failure(id, ErrorCode::CONFLICT_WITH_SETTING, "coordinate system was not enabled");
  }
}

void ColonioImpl::api_send(uint32_t id, const api::colonio::Send& param) {
  send_packet();
}

void ColonioImpl::check_api_connect() {
  if (api_connect_id != 0 && link_status == LinkStatus::ONLINE && api_connect_reply) {
    assert(seed_accessor->get_auth_status() == AuthStatus::SUCCESS);

    std::unique_ptr<api::Reply> reply = std::make_unique<api::Reply>();
    reply->set_id(api_connect_id);
    context.local_nid.to_pb(api_connect_reply->mutable_local_nid());
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
      Pubsub2DAPI::make_entry(context, api_bundler, api_delegate, module_bundler, *coord_system, module_config);
      module->set_type(api::colonio::ConnectReply_ModuleType_PUBSUB_2D);

    } else if (type == "mapPaxos") {
      MapPaxosAPI::make_entry(context, api_bundler, api_delegate, module_bundler, module_config);
      module->set_type(api::colonio::ConnectReply_ModuleType_MAP);

    } else {
      colonio_fatal("unsupported algorithm(%s)", type.c_str());
    }
  }

  check_api_connect();
}

void ColonioImpl::on_change_accessor_status() {
  LinkStatus::Type seed_status = seed_accessor->get_status();
  LinkStatus::Type node_status = node_accessor->get_status();

  logd("link status")
      .map_int("seed", seed_status)
      .map_int("node", node_status)
      .map_int("auth", seed_accessor->get_auth_status())
      .map_bool("onlyone", seed_accessor->is_only_one());

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
    loge("connect failure");
    if (api_connect_id != 0) {
      api_failure(api_connect_id, ErrorCode::OFFLINE, "Connect failure.");
      api_connect_id = 0;
    }

    context.scheduler.add_timeout_task(
        this, [this]() { api_disconnect(0); }, 0);
    status = LinkStatus::OFFLINE;

  } else if (enable_retry) {
    seed_accessor->connect();
    status = LinkStatus::CONNECTING;
  }

  link_status = status;
  check_api_connect();

  module_bundler.on_change_accessor_status(seed_status, node_status);
}

void ColonioImpl::relay_packet(std::unique_ptr<const Packet> packet, bool is_from_seed) {
  assert(packet->channel != APIChannel::NONE);
  assert(packet->module_channel != ModuleChannel::NONE);

  if ((packet->mode & PacketMode::RELAY_SEED) != 0x0) {
    LinkStatus::Type seed_status = seed_accessor->get_status();

    if ((packet->mode & PacketMode::REPLY) != 0x0 && !is_from_seed) {
      if (seed_status == LinkStatus::ONLINE) {
        seed_accessor->relay_packet(std::move(packet));
      } else {
        assert(routing);
        node_accessor->relay_packet(routing->get_route_to_seed(), std::move(packet));
      }
      return;

    } else if (packet->src_nid == context.local_nid) {
      if (seed_status == LinkStatus::ONLINE) {
        seed_accessor->relay_packet(std::move(packet));
      } else {
        logw("drop packet").map("packet", *packet);
      }
      return;
    }
  }

  assert(routing);
  NodeID dst_nid = routing->get_relay_nid_1d(*packet);

  if (dst_nid == NodeID::THIS || dst_nid == context.local_nid) {
    logd("receive packet").map("packet", *packet);
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
#ifndef NDEBUG
    NodeID src_nid    = packet->src_nid;
    const Packet copy = *packet;
#endif
    if (node_accessor->relay_packet(dst_nid, std::move(packet))) {
#ifndef NDEBUG
      if (src_nid == context.local_nid) {
        logd("send packet").map("dst", dst_nid).map("packet", copy);
      } else {
        logd("relay packet").map("dst", dst_nid).map("packet", copy);
      }
#endif
      return;
    }
  }

  node_accessor->update_link_status();

  if (packet) {
    logw("drop packet").map("packet", *packet);
  }
}
}  // namespace colonio
