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

#include <cstdint>
#include <ctime>

#include "colonio/constant.hpp"

namespace colonio {

typedef uint64_t vaddr_t;

static const char PROTOCOL_VERSION[] = "A1";

typedef uint32_t CallID;

/**
 * Pipe/server connection status.
 */
namespace LinkStatus {
typedef int Type;
static const Type OFFLINE    = 0;
static const Type CONNECTING = 1;
static const Type ONLINE     = 2;
static const Type CLOSING    = 3;
}  // namespace LinkStatus

/**
 * Specified node-id.
 */
namespace NID {
static const char NONE[] = "";      ///< Set none if destination node-id isn't yet determined.
static const char THIS[] = ".";     ///< Send command to this(local) node.
static const char SEED[] = "seed";  ///< Server is a special node.
static const char NEXT[] = "next";  ///< Next node is nodes of connecting direct from this node.
}  // namespace NID

namespace APIChannel {
typedef uint16_t Type;
static const Type NONE    = 0;
static const Type COLONIO = 1;
}  // namespace APIChannel

namespace ModuleChannel {
typedef uint16_t Type;
static const Type NONE = 0;

namespace Colonio {
static const Type MAIN           = 1;
static const Type SEED_ACCESSOR  = 2;
static const Type NODE_ACCESSOR  = 3;
static const Type SYSTEM_ROUTING = 4;
// static const Type SYSTEM_ROUTING_2D = 16;
}  // namespace Colonio

namespace MapPaxos {
static const Type MAP_PAXOS = 1;
}

namespace Pubsub2D {
static const Type PUBSUB_2D = 1;
}
}  // namespace ModuleChannel

namespace CommandID {
typedef uint16_t Type;
static const Type ERROR   = 0xFFFF;
static const Type FAILURE = 0xFFFE;
static const Type SUCCESS = 0xFFFD;

namespace Seed {
static const Type AUTH           = 1;
static const Type HINT           = 2;
static const Type PING           = 3;
static const Type REQUIRE_RANDOM = 4;
}  // namespace Seed

namespace WebrtcConnect {
static const Type OFFER = 1;
static const Type ICE   = 2;
}  // namespace WebrtcConnect

namespace Routing {
static const Type ROUTING = 1;
}  // namespace Routing

namespace MapPaxos {
static const Type GET              = 1;
static const Type SET              = 2;
static const Type PREPARE          = 3;
static const Type ACCEPT           = 4;
static const Type HINT             = 5;
static const Type BALANCE_ACCEPTOR = 6;
static const Type BALANCE_PROPOSER = 7;
}  // namespace MapPaxos

namespace Pubsub2D {
static const Type PASS    = 1;
static const Type KNOCK   = 2;
static const Type DEFFUSE = 3;
}  // namespace Pubsub2D
}  // namespace CommandID

namespace SeedHint {
typedef uint32_t Type;
static const Type NONE     = 0x0000;
static const Type ONLYONE  = 0x0001;
static const Type ASSIGNED = 0x0002;
}  // namespace SeedHint

/**
 *
 */
namespace PacketMode {
typedef uint16_t Type;
static const Type NONE       = 0x0000;
static const Type REPLY      = 0x0001;
static const Type EXPLICIT   = 0x0002;
static const Type ONE_WAY    = 0x0004;
static const Type RELAY_SEED = 0x0008;
static const Type NO_RETRY   = 0x0010;
}  // namespace PacketMode

// Packet header size including index and id.
static const uint32_t ESTIMATED_HEAD_SIZE = 64;

// default values.
static const unsigned int MAP_PAXOS_RETRY_MAX          = 5;
static const unsigned int MAP_PAXOS_RETRY_INTERVAL_MIN = 1000;  // [msec]
static const unsigned int MAP_PAXOS_RETRY_INTERVAL_MAX = 2000;  // [msec]

static const unsigned int PUBSUB_2D_CACHE_TIME = 30000;  // [msec]

// fixed parameters
static const uint32_t ORPHAN_NODES_MAX                  = 32;
static const unsigned int PACKET_RETRY_COUNT_MAX        = 5;
static const int64_t PACKET_RETRY_INTERVAL              = 10000;
static const int64_t CONNECT_LINK_TIMEOUT               = 30000;
static const unsigned int FIRST_LINK_RETRY_MAX          = 3;
static const int64_t LINK_TRIAL_TIME_MIN                = 60000;
static const unsigned int LINKS_MIN                     = 4;
static const unsigned int LINKS_MAX                     = 24;
static const unsigned int NODE_ACCESSOR_PACKET_SIZE     = 8192;
static const unsigned int NODE_ACCESSOR_BUFFER_INTERVAL = 100;
static const unsigned int ROUTING_UPDATE_PERIOD         = 1000;
static const unsigned int ROUTING_FORCE_UPDATE_TIMES    = 30;
static const unsigned int ROUTING_SEED_RANDOM_WAIT      = 60000;
static const unsigned int ROUTING_SEED_CONNECT_STEP     = 10;
static const unsigned int ROUTING_SEED_DISCONNECT_STEP  = 8;
static const int64_t SEED_CONNECT_INTERVAL              = 10000;
static const uint32_t PACKET_ID_NONE                    = 0x0;
static const unsigned int EVENT_QUEUE_LIMIT             = 100;

// debug parameter
static const int DEBUG_PRINT_PACKET_SIZE = 32;
}  // namespace colonio
