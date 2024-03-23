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
package node_id

import (
	"fmt"
	"math/rand"

	"github.com/llamerada-jp/colonio/internal/proto"
)

type NodeID struct {
	t   Type
	id0 uint64
	id1 uint64
}

func NewFromProto(p *proto.NodeID) *NodeID {
	return &NodeID{
		t:   Type(p.Type),
		id0: p.Id0,
		id1: p.Id1,
	}
}

func NewRandom() *NodeID {
	return &NodeID{
		t:   TypeNormal,
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

	case TypeNormal:
		return fmt.Sprintf("%016x%016x", n.id0, n.id1)

	case TypeThis:
		return StrThis

	case TypeSeed:
		return StrSeed

	case TypeNext:
		return StrNext
	}

	return StrNone
}
