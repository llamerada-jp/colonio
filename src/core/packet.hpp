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

#include <cassert>
#include <memory>

#include "colonio.pb.h"
#include "definition.hpp"
#include "node_id.hpp"

namespace colonio {
class Packet {
 public:
  class Content {
   public:
    explicit Content(std::unique_ptr<const std::string> src);
    explicit Content(std::unique_ptr<const proto::PacketContent> src);

    const std::string& as_string() const;
    const proto::PacketContent& as_proto() const;

   private:
    mutable std::unique_ptr<const std::string> str;
    mutable std::unique_ptr<const proto::PacketContent> pb;
  };

  const NodeID dst_nid;
  const NodeID src_nid;
  const uint32_t id;
  const uint32_t hop_count;
  std::shared_ptr<Content> content;
  const PacketMode::Type mode;

  Packet(
      const NodeID& dst, const NodeID& src, uint32_t i, std::unique_ptr<const proto::PacketContent> c,
      PacketMode::Type m);
  Packet(const NodeID& dst, const NodeID& src, uint32_t i, uint32_t h, std::shared_ptr<Content> c, PacketMode::Type m);
};
}  // namespace colonio
