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

#include <set>
#include <string>
#include <tuple>

namespace colonio {
namespace Protocol {
class NodeID;
}  // namespace Protocol

/**
 * NodeID is the ID it assigned for each node process run in any devices.
 * ID is unique in a system (not unique in systems).
 * ID has a 128-bit address space. In addition to the normal address,
 * it has a special state indicating none(empty), seed, this node, and next nodes.
 */
class NodeID {
 public:
  /// NONE is meaning no one node is not pointed like the null.
  static const NodeID NONE;
  /// SEED is meaning seed server in this system.
  static const NodeID SEED;
  /// THIS is meaning this node.
  static const NodeID THIS;
  /// NEXT is meaning all neighboring nodes that arrive from this node in 1 step.
  static const NodeID NEXT;

  /// Max node ID in normal addressing.
  static const NodeID NID_MAX;
  /// Minimum node ID in normal addressing.
  static const NodeID NID_MIN;

  /// Quarter node ID is meaning 1/4 of max node ID in normal addressing.
  static const NodeID QUARTER;

  static const NodeID RANGE_0;
  static const NodeID RANGE_1;
  static const NodeID RANGE_2;
  static const NodeID RANGE_3;
  static const NodeID RANGE_4;
  static const NodeID RANGE_5;
  static const NodeID RANGE_6;
  static const NodeID RANGE_7;

  static NodeID from_str(const std::string& str);
  static NodeID from_pb(const Protocol::NodeID& pb);
  static NodeID make_hash_from_str(const std::string& str);
  static NodeID make_random();

  NodeID();
  NodeID(const NodeID& src);

  NodeID& operator=(const NodeID& src);
  NodeID& operator+=(const NodeID& src);
  bool operator==(const NodeID& b) const;
  bool operator!=(const NodeID& b) const;
  bool operator<(const NodeID& b) const;
  bool operator>(const NodeID& b) const;
  NodeID operator+(const NodeID& b) const;
  NodeID operator-(const NodeID& b) const;

  static NodeID center_mod(const NodeID& a, const NodeID& b);

  void get_raw(uint64_t* id0, uint64_t* id1) const;
  NodeID distance_from(const NodeID& a) const;
  bool is_between(const NodeID& a, const NodeID& b) const;
  bool is_special() const;
  int log2() const;
  std::string to_str() const;
  void to_pb(Protocol::NodeID* pb) const;
  picojson::value to_json() const;

 private:
  int type;
  uint64_t id[2];

  explicit NodeID(int type_);
  NodeID(uint64_t id0, uint64_t id1);

  static int compare(const NodeID& a, const NodeID& b);
  static std::tuple<uint64_t, uint64_t> add_mod(uint64_t a0, uint64_t a1, uint64_t b0, uint64_t b1);
  static std::tuple<uint64_t, uint64_t> shift_right(uint64_t a0, uint64_t a1);
};
}  // namespace colonio
