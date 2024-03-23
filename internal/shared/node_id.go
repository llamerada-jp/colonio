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
	"fmt"
	"math/rand"

	"github.com/llamerada-jp/colonio/internal/proto"
)

const (
	strNone = ""
	strThis = "."
	strSeed = "seed"
	strNext = "next"

	typeNone   = uint32(0)
	typeNormal = uint32(1)
	typeThis   = uint32(2)
	typeSeed   = uint32(3)
	typeNext   = uint32(4)
)

// DO NOT USE NodeID pointer for map key.
type NodeID struct {
	t   uint32
	id0 uint64
	id1 uint64
}

func NewNodeIDFromProto(p *proto.NodeID) *NodeID {
	return &NodeID{
		t:   p.Type,
		id0: p.Id0,
		id1: p.Id1,
	}
}

func NewRandomNodeID() *NodeID {
	return &NodeID{
		t:   typeNormal,
		id0: rand.Uint64(),
		id1: rand.Uint64(),
	}
}

func (n *NodeID) Proto() *proto.NodeID {
	return &proto.NodeID{
		Type: uint32(n.t),
		Id0:  n.id0,
		Id1:  n.id1,
	}
}

func (n *NodeID) String() string {
	switch n.t {
	//case NidTypeNone:
	//	return NidNone

	case typeNormal:
		return fmt.Sprintf("%016x%016x", n.id0, n.id1)

	case typeThis:
		return strThis

	case typeSeed:
		return strSeed

	case typeNext:
		return strNext
	}

	return strNone
}

// Equal returns true if the two NodeIDs are equal.
// This function is no effect for key of map.
func (n *NodeID) Equal(o *NodeID) bool {
	return n.t == o.t && n.id0 == o.id0 && n.id1 == o.id1
}
