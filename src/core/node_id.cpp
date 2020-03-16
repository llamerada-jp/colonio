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
extern "C" {
#include "md5/md5.h"
}

#include <cassert>
#include <functional>
#include <set>
#include <string>
#include <tuple>

#include "convert.hpp"
#include "core.pb.h"
#include "definition.hpp"
#include "internal_exception.hpp"
#include "node_id.hpp"
#include "utils.hpp"

namespace colonio {
// Node types.
namespace Type {
static const int NONE   = 0;
static const int NORMAL = 1;
static const int THIS   = 2;
static const int SEED   = 3;
static const int NEXT   = 4;
}  // namespace Type

const NodeID NodeID::NONE(Type::NONE);
const NodeID NodeID::SEED(Type::SEED);
const NodeID NodeID::THIS(Type::THIS);
const NodeID NodeID::NEXT(Type::NEXT);

const NodeID NodeID::NID_MAX(0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF);
const NodeID NodeID::NID_MIN(0x0000000000000000, 0x0000000000000000);

const NodeID NodeID::QUARTER(0x4000000000000000, 0x0000000000000000);

const NodeID NodeID::RANGE_0(0x0001FFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF);
const NodeID NodeID::RANGE_1(0x0007FFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF);
const NodeID NodeID::RANGE_2(0x001FFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF);
const NodeID NodeID::RANGE_3(0x007FFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF);
const NodeID NodeID::RANGE_4(0x01FFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF);
const NodeID NodeID::RANGE_5(0x07FFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF);
const NodeID NodeID::RANGE_6(0x1FFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF);
const NodeID NodeID::RANGE_7(0x7FFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF);

/**
 * Default constructor, it set NONE value.
 */
NodeID::NodeID() : type(Type::NONE), id{0, 0} {
}

/**
 * Copy constructor, copy type and id.
 * @param src Copy source.
 */
NodeID::NodeID(const NodeID& src) : type(src.type), id{src.id[0], src.id[1]} {
}

/**
 * Constructor with a special node type.
 * @param type_ A special node type without normal.
 */
NodeID::NodeID(int type_) : type(type_), id{0, 0} {
  assert(type_ != Type::NORMAL);
}

/**
 * Constructor with a node-id.
 * @param id0 ID of the first half 64bit.
 * @param id1 ID of the latter half 64bit.
 */
NodeID::NodeID(uint64_t id0, uint64_t id1) : type(Type::NORMAL), id{id0, id1} {
}

/**
 * Make a node-id from a string.
 * Rule:
 * from_str("") => NodeID::NONE
 * from_str(".") => NodeID::THIS
 * from_str("seed") => NodeID::SEED
 * from_str("next") => NodeID::NEXT
 * from_str("0123456789abcdef0123456789ABCDEF") => Normal node-id(0x0123456789ABCDEF0123456789ABCDEF)
 * other => Exception
 * @param str A source string value.
 * @return A node-id.
 */
NodeID NodeID::from_str(const std::string& str) {
  bool is_normal = true;
  uint64_t id0   = 0;
  uint64_t id1   = 0;

  if (str.size() == 32) {
    for (unsigned int i = 0; i < 16; i++) {
      char c0 = str[i];
      char c1 = str[i + 16];

      if ('0' <= c0 && c0 <= '9') {
        id0 = (id0 << 4) + (c0 - '0');

      } else if ('a' <= c0 && c0 <= 'f') {
        id0 = (id0 << 4) + (c0 - 'a' + 10);

      } else if ('A' <= c0 && c0 <= 'F') {
        id0 = (id0 << 4) + (c0 - 'A' + 10);

      } else {
        is_normal = false;
        break;
      }

      if ('0' <= c1 && c1 <= '9') {
        id1 = (id1 << 4) + (c1 - '0');

      } else if ('a' <= c1 && c1 <= 'f') {
        id1 = (id1 << 4) + (c1 - 'a' + 10);

      } else if ('A' <= c1 && c1 <= 'F') {
        id1 = (id1 << 4) + (c1 - 'A' + 10);

      } else {
        is_normal = false;
        break;
      }
    }

  } else {
    is_normal = false;
  }

  if (is_normal) {
    return NodeID(id0, id1);

  } else {
    if (str == NID::NONE) {
      return NONE;

    } else if (str == NID::THIS) {
      return THIS;

    } else if (str == NID::SEED) {
      return SEED;

    } else if (str == NID::NEXT) {
      return NEXT;

    } else {
      colonio_throw(Exception::Code::INCORRECT_DATA_FORMAT, "illegal node-id string. (string : %s)", str.c_str());
    }
  }
}

/**
 * Make a node-id from a packet of Protocol Buffers.
 * @param pb A source packet value.
 * @return A node-id.
 * @exception Raise when a illegal packet was selected.
 */
NodeID NodeID::from_pb(const core::NodeID& pb) {
  switch (pb.type()) {
    case Type::NONE:
      return NodeID::NONE;

    case Type::NORMAL:
      return NodeID(pb.id0(), pb.id1());

    case Type::THIS:
      return NodeID::THIS;

    case Type::SEED:
      return NodeID::SEED;

    case Type::NEXT:
      return NodeID::NEXT;

    default:
      colonio_throw(
          Exception::Code::INCORRECT_DATA_FORMAT, "illegal node-id type in Protocol Buffers. (type : %d)", pb.type());
  }
}

/**
 * Make a node-id from md5 hash of a input string.
 * @param pb A source string value.
 * @return A node-id.
 */
NodeID NodeID::make_hash_from_str(const std::string& str) {
  MD5_CTX ctx;
  unsigned char md5[16];

  MD5_Init(&ctx);
  MD5_Update(&ctx, str.c_str(), str.size());
  MD5_Final(md5, &ctx);

  uint64_t id0 = (static_cast<uint64_t>(md5[0]) << 0x38) | (static_cast<uint64_t>(md5[1]) << 0x30) |
                 (static_cast<uint64_t>(md5[2]) << 0x28) | (static_cast<uint64_t>(md5[3]) << 0x20) |
                 (static_cast<uint64_t>(md5[4]) << 0x18) | (static_cast<uint64_t>(md5[5]) << 0x10) |
                 (static_cast<uint64_t>(md5[6]) << 0x08) | (static_cast<uint64_t>(md5[7]) << 0x00);
  uint64_t id1 = (static_cast<uint64_t>(md5[8]) << 0x38) | (static_cast<uint64_t>(md5[9]) << 0x30) |
                 (static_cast<uint64_t>(md5[10]) << 0x28) | (static_cast<uint64_t>(md5[11]) << 0x20) |
                 (static_cast<uint64_t>(md5[12]) << 0x18) | (static_cast<uint64_t>(md5[13]) << 0x10) |
                 (static_cast<uint64_t>(md5[14]) << 0x08) | (static_cast<uint64_t>(md5[15]) << 0x00);

  return NodeID(id0, id1);
}

/**
 * Make a rundom node-id.
 * @return A node-id.
 */
NodeID NodeID::make_random() {
  return NodeID(Utils::get_rnd_64(), Utils::get_rnd_64());
}

NodeID& NodeID::operator=(const NodeID& src) {
  type  = src.type;
  id[0] = src.id[0];
  id[1] = src.id[1];

  return *this;
}

NodeID& NodeID::operator+=(const NodeID& src) {
  assert(type == Type::NORMAL);
  assert(src.type == Type::NORMAL);

  std::tie(id[0], id[1]) = add_mod(id[0], id[1], src.id[0], src.id[1]);

  return *this;
}

bool NodeID::operator==(const NodeID& b) const {
  if (type == b.type) {
    if (type == Type::NORMAL) {
      return (id[0] == b.id[0] && id[1] == b.id[1]);

    } else {
      return true;
    }

  } else {
    return false;
  }
}

bool NodeID::operator!=(const NodeID& b) const {
  return !(*this == b);
}

bool NodeID::operator<(const NodeID& b) const {
  if (type == b.type) {
    if (type == Type::NORMAL) {
      return compare(*this, b) == -1;

    } else {
      return false;
    }

  } else {
    return type < b.type;
  }
}

bool NodeID::operator>(const NodeID& b) const {
  if (type == b.type) {
    if (type == Type::NORMAL) {
      return compare(*this, b) == 1;

    } else {
      return false;
    }

  } else {
    return type > b.type;
  }
}

NodeID NodeID::operator+(const NodeID& b) const {
  assert(type == Type::NORMAL);
  assert(b.type == Type::NORMAL);

  uint64_t c0, c1;
  std::tie(c0, c1) = add_mod(id[0], id[1], b.id[0], b.id[1]);

  return NodeID(c0, c1);
}

NodeID NodeID::operator-(const NodeID& b) const {
  uint64_t c0, c1;
  std::tie(c0, c1) = add_mod(id[0], id[1], ~b.id[0], ~b.id[1]);
  std::tie(c0, c1) = add_mod(c0, c1, 0x0, 0x1);

  return NodeID(c0, c1);
}

/**
 * Calculate central node-id of A and B.
 * Node-id is placed on torus.
 * If node-id of A is larger than B, return value is smaller than A or larger than B.
 * A and B must be NORMAL node-id.
 * @param a Node-id A.
 * @param b Node-id B.
 * @return Central node-id.
 */
NodeID NodeID::center_mod(const NodeID& a, const NodeID& b) {
  assert(a.type == Type::NORMAL);
  assert(b.type == Type::NORMAL);

  uint64_t a0, a1, b0, b1;
  std::tie(a0, a1) = shift_right(a.id[0], a.id[1]);
  std::tie(b0, b1) = shift_right(b.id[0], b.id[1]);

  uint64_t c0, c1;
  std::tie(c0, c1) = add_mod(a0, a1, b0, b1);

  if (a.id[1] & 0x01 && b.id[1] & 0x1) {
    std::tie(c0, c1) = add_mod(c0, c1, 0x00, 0x01);
  }

  if (b < a) {
    std::tie(c0, c1) = add_mod(c0, c1, 0x8000000000000000, 0x0000000000000000);
  }

  return NodeID(c0, c1);
}

void NodeID::get_raw(uint64_t* id0, uint64_t* id1) const {
  *id0 = id[0];
  *id1 = id[1];
}

/**
 * Calculate a distance between this node-id and node-id of A.
 * @param a Node-id A.
 * @return A distance between this node-id and node-id of A.
 */
NodeID NodeID::distance_from(const NodeID& a) const {
  assert(type == Type::NORMAL);
  assert(a.type == Type::NORMAL);

  uint64_t a0, a1, b0, b1;

  if (*this < a) {
    a0 = a.id[0];
    a1 = a.id[1];
    b0 = ~id[0];
    b1 = ~id[1];

  } else {
    a0 = id[0];
    a1 = id[1];
    b0 = ~a.id[0];
    b1 = ~a.id[1];
  }

  std::tie(a0, a1) = add_mod(a0, a1, b0, b1);
  std::tie(a0, a1) = add_mod(a0, a1, 0x0, 0x1);

  if (a0 >= 0x8000000000000000) {
    std::tie(a0, a1) = add_mod(~a0, ~a1, 0x0, 0x1);
  }

  return NodeID(a0, a1);
}

/**
 * Check a node-id is placed between node-id of A and B.
 * Node-id is placed on torus.
 * If node-id of A is larger than B, return true if this < B or A <= this.
 * @param a Node-id A.
 * @param b Node-id B.
 * @return True if A <= this < B.
 */
bool NodeID::is_between(const NodeID& a, const NodeID& b) const {
  assert(type == Type::NORMAL);
  assert(a.type == Type::NORMAL);
  assert(b.type == Type::NORMAL);

  switch (compare(a, b)) {
    case -1: {
      if (compare(a, *this) != 1 && compare(*this, b) == -1) {
        return true;
      } else {
        return false;
      }
    } break;

    case 0: {
      return false;
    } break;

    case 1: {
      if ((compare(a, *this) != 1 && compare(*this, NID_MAX) != 1) ||
          (compare(NID_MIN, *this) != 1 && compare(*this, b) == -1)) {
        return true;
      } else {
        return false;
      }
    } break;

    default: {
      assert(false);
      return false;
    } break;
  }
}

bool NodeID::is_special() const {
  return type != Type::NORMAL;
}

/**
 * Calculate log(2) from this node-id.
 * @param return log2(node-id)
 */
int NodeID::log2() const {
  static const uint64_t TOP = 0x8000000000000000;
  for (unsigned int i = 0; i <= sizeof(id[0]) * 8; i++) {
    if ((TOP >> i) & id[0]) {
      return 127 - i;
    }
  }
  for (unsigned int i = 0; i <= sizeof(id[1]) * 8; i++) {
    if ((TOP >> i) & id[1]) {
      return 63 - i;
    }
  }
  return 0;
}

/**
 * Convert a node-id to a string.
 * @param nid A source node-id.
 * @return Converted value as a string.
 */
std::string NodeID::to_str() const {
  switch (type) {
    case Type::NONE: {
      return NID::NONE;
    } break;

    case Type::NORMAL: {
      return Convert::int2str(id[0]) + Convert::int2str(id[1]);
    } break;

    case Type::THIS: {
      return NID::THIS;
    } break;

    case Type::SEED: {
      return NID::SEED;
    } break;

    case Type::NEXT: {
      return NID::NEXT;
    } break;

    default: {
      /// @todo error
      assert(false);
      return NID::NONE;
    } break;
  }
}

void NodeID::to_pb(core::NodeID* pb) const {
  if (type == Type::NORMAL) {
    pb->set_type(Type::NORMAL);
    pb->set_id0(id[0]);
    pb->set_id1(id[1]);

  } else {
    pb->set_type(type);
    pb->clear_id0();
    pb->clear_id1();
  }
}

/**
 * Convert a node-id to a JSON.
 * Deprecated. For only debug.
 * @param nid A source node-id.
 * @return Node-id as a JSON.
 */
picojson::value NodeID::to_json() const {
  return picojson::value(to_str());
}

/**
 * Compare a pair of NORMAL value like the java's Compare interface.
 * @return -1 if a < b. 0 if a == b. 1 if a > b.
 */
int NodeID::compare(const NodeID& a, const NodeID& b) {
  assert(a.type == Type::NORMAL);
  assert(b.type == Type::NORMAL);

  if (a.id[0] < b.id[0]) {
    return -1;
  } else if (a.id[0] > b.id[0]) {
    return 1;
  }

  if (a.id[1] < b.id[1]) {
    return -1;
  } else if (a.id[1] == b.id[1]) {
    return 0;
  } else {
    return 1;
  }
}

/**
 * Calculate unsigned add node-id A and B on module MAX space.
 * @param a0 Node-id A's upper value.
 * @param a1 Node-id A's lower value.
 * @param a0 Node-id B's upper value.
 * @param a1 Node-id B's lower value.
 * @return Calucated value's upper and lower pair.
 */
std::tuple<uint64_t, uint64_t> NodeID::add_mod(uint64_t a0, uint64_t a1, uint64_t b0, uint64_t b1) {
  uint64_t d0, d1;
  uint64_t c0, c1;

  // Lower.
  c1 = (0x7FFFFFFFFFFFFFFF & a1) + (0x7FFFFFFFFFFFFFFF & b1);
  d1 = (a1 >> 63) + (b1 >> 63) + (c1 >> 63);
  c1 = ((d1 << 63) & 0x8000000000000000) | (c1 & 0x7FFFFFFFFFFFFFFF);
  // Upper.
  c0 = (0x3FFFFFFFFFFFFFFF & a0) + (0x3FFFFFFFFFFFFFFF & b0) + (d1 >> 1);
  d0 = (a0 >> 62) + (b0 >> 62) + (c0 >> 62);
  c0 = ((d0 << 62) & 0xC000000000000000) | (c0 & 0x3FFFFFFFFFFFFFFF);

  return std::forward_as_tuple(c0, c1);
}

/**
 * Calucate 1 bit right shift for node-id.
 * @param a0 Node-id's upper value.
 * @param a1 Node-id's lower value.
 * @return Calucated value's upper and lower pair.
 */
std::tuple<uint64_t, uint64_t> NodeID::shift_right(uint64_t a0, uint64_t a1) {
  uint64_t c0, c1;

  c1 = ((a0 << 63) & 0x8000000000000000) | ((a1 >> 1) & 0x7FFFFFFFFFFFFFFF);
  c0 = a0 >> 1;

  return std::forward_as_tuple(c0, c1);
}
}  // namespace colonio
