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

#include <cstdint>
#include <ctime>

#include "colonio/colonio.hpp"

namespace colonio {

static const char PROTOCOL_VERSION[] = "A2";

/**
 * Pipe/server connection state.
 */
namespace LinkState {
typedef int Type;
static const Type OFFLINE    = 0;
static const Type CONNECTING = 1;
static const Type ONLINE     = 2;
static const Type CLOSING    = 3;
}  // namespace LinkState

/**
 * Specified node-id.
 */
namespace NID {
static const char NONE[] = "";      ///< Set none if destination node-id isn't yet determined.
static const char THIS[] = ".";     ///< Send command to this(local) node.
static const char SEED[] = "seed";  ///< Server is a special node.
static const char NEXT[] = "next";  ///< Next node is nodes of connecting direct from this node.
}  // namespace NID

namespace SeedHint {
typedef uint32_t Type;
static const Type NONE           = 0x0000;
static const Type ONLY_ONE       = 0x0001;
static const Type REQUIRE_RANDOM = 0x0002;
}  // namespace SeedHint

/**
 *
 */
namespace PacketMode {
typedef uint16_t Type;
static const Type NONE       = 0x0000;
static const Type RESPONSE   = 0x0001;
static const Type EXPLICIT   = 0x0002;
static const Type ONE_WAY    = 0x0004;
static const Type RELAY_SEED = 0x0008;
static const Type NO_RETRY   = 0x0010;
}  // namespace PacketMode

// Packet header size including index and id.
static const uint32_t ESTIMATED_HEAD_SIZE = 64;

// default values.
static const unsigned int NODE_ACCESSOR_BUFFER_INTERVAL = 100;
static const uint32_t NODE_ACCESSOR_HOP_COUNT_MAX       = 64;
static const unsigned int NODE_ACCESSOR_PACKET_SIZE     = 8192;

static const unsigned int KVS_RETRY_MAX          = 5;
static const unsigned int KVS_RETRY_INTERVAL_MIN = 1000;  // [msec]
static const unsigned int KVS_RETRY_INTERVAL_MAX = 2000;  // [msec]

static const unsigned int SPREAD_CACHE_TIME = 30000;  // [msec]

static const unsigned int ROUTING_FORCE_UPDATE_COUNT        = 30;
static const unsigned int ROUTING_SEED_CONNECT_INTERVAL     = 10000;  // [msec]
static const unsigned int ROUTING_SEED_CONNECT_RATE         = 512;
static const unsigned int ROUTING_SEED_DISCONNECT_THRESHOLD = 30000;  // [msec]
static const unsigned int ROUTING_SEED_INFO_KEEP_THRESHOLD  = 60000;  // [msec]
static const unsigned int ROUTING_SEED_INFO_NIDS_COUNT      = 4;
static const double ROUTING_SEED_NEXT_POSITION              = 0.71828;
static const unsigned int ROUTING_UPDATE_PERIOD             = 1000;  // [msec]

// fixed parameters
static const uint32_t ORPHAN_NODES_MAX           = 32;
static const unsigned int PACKET_RETRY_COUNT_MAX = 5;
static const int64_t PACKET_RETRY_INTERVAL       = 10000;
static const int64_t CONNECT_LINK_TIMEOUT        = 30000;
static const unsigned int FIRST_LINK_RETRY_MAX   = 3;
static const int64_t LINK_TRIAL_TIME_MIN         = 60000;
static const unsigned int LINKS_MIN              = 4;
static const int64_t SEED_CONNECT_INTERVAL       = 10000;
static const int64_t SEED_SESSION_TIMEOUT        = 10 * 1000;  // [msec]
static const uint32_t PACKET_ID_NONE             = 0x0;

}  // namespace colonio
