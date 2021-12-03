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

#include "definition.hpp"
#include "node_id.hpp"

namespace colonio {
class Packet {
 public:
  const NodeID dst_nid;
  const NodeID src_nid;
  const uint32_t hop_count;
  const uint32_t id;
  std::shared_ptr<const std::string> content;
  const PacketMode::Type mode;
  const Channel::Type channel;
  const CommandID::Type command_id;

  template<typename T>
  void parse_content(T* dst) const {
    assert(content.get() != nullptr);
    if (!dst->ParseFromString(*content)) {
      /// @todo error
      assert(false);
    }
  }
};
}  // namespace colonio
