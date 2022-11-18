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
#include "packet.hpp"

#include <cassert>

namespace colonio {
Packet::Content::Content(std::unique_ptr<const std::string> src) : str(std::move(src)) {
}

Packet::Content::Content(std::unique_ptr<const proto::PacketContent> src) : pb(std::move(src)) {
}

const std::string& Packet::Content::as_string() const {
  if (!str) {
    assert(pb);
    std::string* p = new std::string();
    str.reset(p);
    pb->SerializeToString(p);
  }
  return *str;
}

const proto::PacketContent& Packet::Content::as_proto() const {
  if (!pb) {
    assert(str);
    proto::PacketContent* p = new proto::PacketContent();
    pb.reset(p);
    p->ParseFromString(*str);
  }
  return *pb;
}

Packet::Packet(
    const NodeID& dst, const NodeID& src, uint32_t i, std::unique_ptr<const proto::PacketContent> c,
    PacketMode::Type m) :
    dst_nid(dst), src_nid(src), id(i), hop_count(0), content(new Content(std::move(c))), mode(m) {
}

Packet::Packet(
    const NodeID& dst, const NodeID& src, uint32_t i, uint32_t h, std::shared_ptr<Content> c, PacketMode::Type m) :
    dst_nid(dst), src_nid(src), id(i), hop_count(h), content(c), mode(m) {
  assert(h < 100);  // detect wrong hops.
}

std::unique_ptr<Packet> Packet::make_copy() const {
  return std::make_unique<Packet>(dst_nid, src_nid, id, hop_count, content, mode);
}
}  // namespace colonio