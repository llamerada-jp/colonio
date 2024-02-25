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

var (
	NodeIDNone = NodeID{t: typeNone}
	NodeIDThis = NodeID{t: typeThis}
	NodeIDSeed = NodeID{t: typeSeed}
	NodeIDNext = NodeID{t: typeNext}
)

// DO NOT USE NodeID pointer for map key.
type NodeID struct {
	t   uint32
	id0 uint64
	id1 uint64
}

func NewNodeIDFromProto(p *proto.NodeID) *NodeID {
	switch p.Type {
	case typeNormal:
		return &NodeID{
			t:   typeNormal,
			id0: p.Id0,
			id1: p.Id1,
		}

	case typeNone, typeThis, typeSeed, typeNext:
		return &NodeID{
			t:   p.Type,
			id0: 0,
			id1: 0,
		}

	default:
		return nil
	}
}

func NewNodeIDFromString(s string) *NodeID {
	switch s {
	case strNone:
		return &NodeIDNone

	case strThis:
		return &NodeIDThis

	case strSeed:
		return &NodeIDSeed

	case strNext:
		return &NodeIDNext

	default:
		if len(s) != 32 {
			return nil
		}
		id0Str := s[:16]
		id1Str := s[16:]
		r := &NodeID{
			t: typeNormal,
		}
		if _, err := fmt.Sscanf(id0Str, "%016x", &r.id0); err != nil {
			return nil
		}
		if _, err := fmt.Sscanf(id1Str, "%016x", &r.id1); err != nil {
			return nil
		}
		return r
	}
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

func (n *NodeID) Proto() *proto.NodeID {
	return &proto.NodeID{
		Type: uint32(n.t),
		Id0:  n.id0,
		Id1:  n.id1,
	}
}

func (n *NodeID) String() string {
	switch n.t {
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

func (n *NodeID) Raw() (uint64, uint64) {
	return n.id0, n.id1
}

func (n *NodeID) IsNormal() bool {
	return n.t == typeNormal
}

// TODO: remove
// see https://go.dev/ref/spec#Comparison_operators
func (n *NodeID) Equal(o *NodeID) bool {
	return n.t == o.t && n.id0 == o.id0 && n.id1 == o.id1
}

// TODO: remove
// see https://go.dev/ref/spec#Comparison_operators
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
