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
	"slices"
	"strconv"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
)

const (
	strLocal         = "local"
	strNeighborhoods = "neighbor"

	typeNormal = uint32(iota)
	typeLocal
	typeNeighborhoods
)

var (
	NodeLocal         = NodeID{t: typeLocal}
	NodeNeighborhoods = NodeID{t: typeNeighborhoods}
)

// DO NOT USE NodeID pointer for map key.
type NodeID struct {
	t   uint32
	id0 uint64
	id1 uint64
}

func NewNodeIDFromProto(p *proto.NodeID) (*NodeID, error) {
	if p == nil {
		return nil, fmt.Errorf("the source should not be nil")
	}

	return &NodeID{
		t:   typeNormal,
		id0: p.Id0,
		id1: p.Id1,
	}, nil
}

func NewNodeIDFromString(s string) (*NodeID, error) {
	if len(s) != 32 {
		return nil, fmt.Errorf("invalid node id string: %s", s)
	}
	for _, r := range s {
		if !('0' <= r && r <= '9') && !('a' <= r && r <= 'f') {
			return nil, fmt.Errorf("invalid node id string: %s", s)
		}
	}
	id0, _ := strconv.ParseUint(s[:16], 16, 64)
	id1, _ := strconv.ParseUint(s[16:], 16, 64)
	r := &NodeID{
		t:   typeNormal,
		id0: id0,
		id1: id1,
	}
	return r, nil
}

func NewNormalNodeID(id0, id1 uint64) *NodeID {
	return &NodeID{
		t:   typeNormal,
		id0: id0,
		id1: id1,
	}
}

func NewRandomNodeID() *NodeID {
	return &NodeID{
		t:   typeNormal,
		id0: rand.Uint64(),
		id1: rand.Uint64(),
	}
}

func (n *NodeID) Copy() *NodeID {
	return &NodeID{
		t:   n.t,
		id0: n.id0,
		id1: n.id1,
	}
}

func (n *NodeID) Proto() *proto.NodeID {
	if n.t != typeNormal {
		panic("invalid node id type for `Proto`")
	}
	return &proto.NodeID{
		Id0: n.id0,
		Id1: n.id1,
	}
}

func (n *NodeID) String() string {
	switch n.t {
	case typeNormal:
		return fmt.Sprintf("%016x%016x", n.id0, n.id1)

	case typeLocal:
		return strLocal

	case typeNeighborhoods:
		return strNeighborhoods
	}

	panic("invalid node id type")
}

func (n *NodeID) Raw() (uint64, uint64) {
	if n.t != typeNormal {
		panic("invalid node id type for `Raw`")
	}
	return n.id0, n.id1
}

func (n *NodeID) IsNormal() bool {
	return n.t == typeNormal
}

func (n *NodeID) Equal(o *NodeID) bool {
	if n == nil && o == nil {
		return true
	} else if n == nil || o == nil {
		return false
	}
	return n.t == o.t && n.id0 == o.id0 && n.id1 == o.id1
}

func (n *NodeID) Smaller(o *NodeID) bool {
	if n.t != o.t {
		return n.t < o.t
	}

	if n.t != typeNormal {
		return false
	}

	if n.id0 != o.id0 {
		return n.id0 < o.id0
	} else {
		return n.id1 < o.id1
	}
}

func (n *NodeID) Compare(o *NodeID) int {
	if *n == *o {
		return 0
	} else if n.Smaller(o) {
		return -1
	}
	return 1
}

func (n *NodeID) Add(o *NodeID) *NodeID {
	if n.t != typeNormal || o.t != typeNormal {
		panic("invalid node id type for `Add`")
	}

	r0, r1 := addMod(n.id0, n.id1, o.id0, o.id1)

	return &NodeID{
		t:   typeNormal,
		id0: r0,
		id1: r1,
	}
}

func (n *NodeID) Sub(o *NodeID) *NodeID {
	if n.t != typeNormal || o.t != typeNormal {
		panic("invalid node id type for `Sub`")
	}

	r0, r1 := addMod(n.id0, n.id1, ^o.id0, ^o.id1)
	r0, r1 = addMod(r0, r1, 0x0, 0x1)

	return &NodeID{
		t:   typeNormal,
		id0: r0,
		id1: r1,
	}
}

func (n *NodeID) DistanceFrom(o *NodeID) *NodeID {
	if n.t != typeNormal || o.t != typeNormal {
		panic("invalid node id type for distance")
	}

	var a0, a1, b0, b1 uint64
	if n.Smaller(o) {
		a0 = o.id0
		a1 = o.id1
		b0 = ^n.id0
		b1 = ^n.id1
	} else {
		a0 = n.id0
		a1 = n.id1
		b0 = ^o.id0
		b1 = ^o.id1
	}

	a0, a1 = addMod(a0, a1, b0, b1)
	a0, a1 = addMod(a0, a1, 0x0, 0x1)

	if a0 >= 0x8000000000000000 {
		a0, a1 = addMod(^a0, ^a1, 0x0, 0x1)
	}

	return &NodeID{
		t:   typeNormal,
		id0: a0,
		id1: a1,
	}
}

func addMod(a0, a1, b0, b1 uint64) (uint64, uint64) {
	// Lower.
	c1 := (0x7FFFFFFFFFFFFFFF & a1) + (0x7FFFFFFFFFFFFFFF & b1)
	d1 := (a1 >> 63) + (b1 >> 63) + (c1 >> 63)
	c1 = ((d1 << 63) & 0x8000000000000000) | (c1 & 0x7FFFFFFFFFFFFFFF)
	// Upper.
	c0 := (0x3FFFFFFFFFFFFFFF & a0) + (0x3FFFFFFFFFFFFFFF & b0) + (d1 >> 1)
	d0 := (a0 >> 62) + (b0 >> 62) + (c0 >> 62)
	c0 = ((d0 << 62) & 0xC000000000000000) | (c0 & 0x3FFFFFFFFFFFFFFF)

	return c0, c1
}

func ConvertNodeIDSetToStringMap(set map[NodeID]struct{}) map[string]struct{} {
	r := map[string]struct{}{}
	for k := range set {
		r[k.String()] = struct{}{}
	}
	return r
}

func ConvertNodeIDsToProto(ids []*NodeID) []*proto.NodeID {
	r := make([]*proto.NodeID, len(ids))
	for i, id := range ids {
		if id == nil {
			r[i] = nil
		} else {
			r[i] = id.Proto()
		}
	}
	return r
}

func ConvertNodeIDsFromProto(ids []*proto.NodeID) ([]*NodeID, error) {
	if ids == nil {
		return nil, nil
	}

	result := make([]*NodeID, len(ids))
	var err error
	for i, nodeID := range ids {
		result[i], err = NewNodeIDFromProto(nodeID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse node id: %w", err)
		}
	}
	return result, nil
}

func SortNodeIDs(nodeIDs []*NodeID) {
	slices.SortFunc(nodeIDs, func(a, b *NodeID) int {
		return a.Compare(b)
	})
}
