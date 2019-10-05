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
#pragma once

#include <cstdint>
#include <ctime>

#include "colonio/constant.hpp"

namespace colonio {

typedef uint64_t vaddr_t;

static const char PROTOCOL_VERSION[] = "A1";

/**
 * Pipe/server connection status.
 */
namespace LinkStatus {
typedef int Type;
static const Type OFFLINE   = 0;
static const Type CONNECTING    = 1;
static const Type ONLINE    = 2;
static const Type CLOSING   = 3;
}  // namespace ConnectStatus

/**
 * Specified node-id.
 */
namespace NID {
static const char NONE[]    = "";   ///< Set none if destination node-id is't yet determined.
static const char THIS[]    = ".";  ///< Send command to this(local) node.
static const char SEED[]    = "seed";   ///< Server is a special node.
static const char NEXT[]    = "next";   ///< Next node is nodes of connecting direct from this node.
}  // namespace NID

namespace ModuleChannel {
typedef uint16_t Type;
static const Type NONE  = 0;
static const Type SEED  = 1;
static const Type WEBRTC_CONNECT    = 2;
static const Type SYSTEM_ROUTING    = 15;
static const Type SYSTEM_ROUTING_2D = 16;
}  // namespace ModuleChannel

namespace CommandID {
typedef uint16_t Type;
static const Type ERROR = 0xFFFF;
static const Type FAILURE   = 0xFFFE;
static const Type SUCCESS   = 0xFFFD;

namespace Seed {
static const Type AUTH = 1;
static const Type HINT = 2;
static const Type PING = 3;
static const Type REQUIRE_RANDOM = 4;
}  // namespace Auth

namespace WebrtcConnect {
static const Type OFFER = 1;
static const Type ICE   = 2;
}  // namespace WebConnect

namespace Routing {
static const Type ROUTING   = 1;
}  // namespace Routing

namespace MapPaxos {
static const Type GET = 1;
static const Type SET = 2;
static const Type PREPARE = 3;
static const Type ACCEPT  = 4;
static const Type HINT    = 5;
static const Type BALANCE_ACCEPTOR = 6;
static const Type BALANCE_PROPOSER = 7;
}  // namespace KeyValuebStore

namespace PubSub2D {
static const Type PASS = 1;
static const Type KNOCK = 2;
static const Type DEFFUSE = 3;
}  // namespace PubSub2D
}  // namespace CommandID

namespace SeedHint {
typedef uint32_t Type;
static const Type NONE      = 0x0000;
static const Type ONLYONE   = 0x0001;
static const Type ASSIGNED  = 0x0002;
}  // namespace SeedStatus

/**
 * 
 */
namespace PacketMode {
typedef uint16_t Type;
static const Type NONE      = 0x0000;
static const Type REPLY     = 0x0001;
static const Type EXPLICIT  = 0x0002;
static const Type ONE_WAY   = 0x0004;
static const Type RELAY_SEED    = 0x0008;
static const Type NO_RETRY  = 0x0010;
}  // namespace CommandMode

// default values.
static const unsigned int MAP_PAXOS_RETRY_MAX = 5;
static const unsigned int PUBSUB2D_CACHE_TIME = 30000; // [msec]

static const uint32_t ORPHAN_NODES_MAX = 32;
static const unsigned int PACKET_RETRY_COUNT_MAX    = 5;
static const int64_t PACKET_RETRY_INTERVAL          = 10000;
static const int64_t CONNECT_LINK_TIMEOUT           = 30000;
static const unsigned int FIRST_LINK_RETRY_MAX      = 3;
static const int64_t  LINK_TRIAL_TIME_MIN           = 60000;
static const unsigned int LINKS_MIN                 = 4;
static const unsigned int LINKS_MAX                 = 24;
static const unsigned int ROUTING_UPDATE_PERIOD     = 1000;
static const unsigned int ROUTING_FORCE_UPDATE_TIMES    = 30;
static const unsigned int ROUTING_SEED_RANDOM_WAIT  = 60000;
static const unsigned int ROUTING_SEED_CONNECT_STEP = 10;
static const unsigned int ROUTING_SEED_DISCONNECT_STEP = 8;
static const int64_t SEED_CONNECT_INTERVAL          = 10000;
static const uint32_t PACKET_ID_NONE = 0x0;

// debug parameter
static const int DEBUG_PRINT_PACKET_SIZE = 32;
}  // namesapce colonio
