/*
 * Copyright 2017- Yuji Ito <llamerada.jp@gmail.com>
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
package shared

import (
	"github.com/llamerada-jp/colonio/internal/proto"
)

type PacketMode uint16

const (
	PacketModeNone      = PacketMode(000000)
	PacketModeResponse  = PacketMode(0x0001)
	PacketModeExplicit  = PacketMode(0x0002)
	PacketModeOneWay    = PacketMode(0x0004)
	PacketModeRelaySeed = PacketMode(0x0008)
	PacketModeNoRetry   = PacketMode(0x0010)
)

type Packet struct {
	DstNodeID *NodeID
	SrcNodeID *NodeID
	ID        uint32
	HopCount  uint32
	Mode      PacketMode
	Content   *proto.PacketContent
}