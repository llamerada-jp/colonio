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
	"testing"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/stretchr/testify/assert"
)

func TestNodeID_NewProto(t *testing.T) {
	tests := []struct {
		input  *proto.NodeID
		expect *NodeID
	}{
		{
			input: &proto.NodeID{
				Type: typeNone,
				Id0:  1,
				Id1:  1,
			},
			expect: &NodeIDNone,
		},
		{
			input: &proto.NodeID{
				Type: typeNormal,
				Id0:  1,
				Id1:  1,
			},
			expect: &NodeID{
				t:   typeNormal,
				id0: 1,
				id1: 1,
			},
		},
		{
			input: &proto.NodeID{
				Type: 99, // invalid
			},
			expect: nil,
		},
	}

	for _, test := range tests {
		nodeID := NewNodeIDFromProto(test.input)
		if test.expect == nil {
			assert.Nil(t, nodeID)
		} else {
			assert.True(t, test.expect.Equal(nodeID))
		}
	}
}

func TestNodeID_NewString(t *testing.T) {
	tests := []struct {
		input  string
		expect *NodeID
	}{
		{
			input:  strNone,
			expect: &NodeIDNone,
		},
		{
			input:  "00000000000000010000000000000001",
			expect: NewNormalNodeID(1, 1),
		},
		{
			input:  "invalid",
			expect: nil,
		},
		{
			input:  strThis,
			expect: &NodeIDThis,
		},
		{
			input:  strSeed,
			expect: &NodeIDSeed,
		},
		{
			input:  strNext,
			expect: &NodeIDNext,
		},
	}

	for _, test := range tests {
		nodeID := NewNodeIDFromString(test.input)
		if test.expect == nil {
			assert.Nil(t, nodeID)
		} else {
			assert.True(t, test.expect.Equal(nodeID))
		}
	}
}

func TestNodeID_Equal(t *testing.T) {
	patterns := []*NodeID{
		&NodeIDNone,
		NewNormalNodeID(0, 1),
		NewNormalNodeID(0, 2),
		&NodeIDThis,
		&NodeIDSeed,
		&NodeIDNext,
	}

	for i, nodeID1 := range patterns {
		for j, nodeID2 := range patterns {
			nodeID2 = NewNodeIDFromString(nodeID2.String())

			if i == j {
				assert.True(t, nodeID1.Equal(nodeID2))
			} else {
				assert.False(t, nodeID1.Equal(nodeID2))
			}
		}
	}
}

func TestNodeID_Smaller(t *testing.T) {
	tests := []struct {
		a      *NodeID
		b      *NodeID
		expect bool
	}{
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 0),
			expect: false,
		},
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 1),
			expect: true,
		},
		{
			a:      NewNormalNodeID(0, 1),
			b:      NewNormalNodeID(0, 0),
			expect: false,
		},
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(1, 0),
			expect: true,
		},
		{
			a:      NewNormalNodeID(1, 0),
			b:      NewNormalNodeID(0, 0),
			expect: false,
		},
		{
			a:      &NodeIDNone,
			b:      NewNormalNodeID(0, 0),
			expect: true,
		},
		{
			a:      NewNormalNodeID(0, 0),
			b:      &NodeIDNone,
			expect: false,
		},
		{
			a:      &NodeIDThis,
			b:      NewNormalNodeID(0, 0),
			expect: false,
		},
		{
			a:      NewNormalNodeID(9, 9),
			b:      &NodeIDThis,
			expect: true,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expect, tt.a.Smaller(tt.b))
	}
}

func TestNodeID_Compare(t *testing.T) {
	tests := []struct {
		a      *NodeID
		b      *NodeID
		expect int
	}{
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 0),
			expect: 0,
		},
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 1),
			expect: -1,
		},
		{
			a:      NewNormalNodeID(0, 1),
			b:      NewNormalNodeID(0, 0),
			expect: 1,
		},
		{
			a:      &NodeIDNone,
			b:      NewNormalNodeID(0, 0),
			expect: -1,
		},
		{
			a:      &NodeIDThis,
			b:      NewNormalNodeID(0, 0),
			expect: 1,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expect, tt.a.Compare(tt.b))
	}
}

func TestNodeID_Add(t *testing.T) {
	tests := []struct {
		a      *NodeID
		b      *NodeID
		expect *NodeID
	}{
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 0),
			expect: NewNormalNodeID(0, 0),
		},
		{
			a:      NewNormalNodeID(0, 1),
			b:      NewNormalNodeID(0, 0),
			expect: NewNormalNodeID(0, 1),
		},
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 1),
			expect: NewNormalNodeID(0, 1),
		},
		{
			a:      NewNormalNodeID(^uint64(0), ^uint64(0)),
			b:      NewNormalNodeID(^uint64(0), ^uint64(0)),
			expect: NewNormalNodeID(^uint64(0), ^uint64(0)-1),
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expect, tt.a.Add(tt.b), fmt.Sprintf("%v + %v", tt.a, tt.b))
	}
}

func TestNodeID_Sub(t *testing.T) {
	tests := []struct {
		a      *NodeID
		b      *NodeID
		expect *NodeID
	}{
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 0),
			expect: NewNormalNodeID(0, 0),
		},
		{
			a:      NewNormalNodeID(0, 1),
			b:      NewNormalNodeID(0, 0),
			expect: NewNormalNodeID(0, 1),
		},
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 1),
			expect: NewNormalNodeID(^uint64(0), ^uint64(0)),
		},
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(1, 0),
			expect: NewNormalNodeID(^uint64(0), 0),
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expect, tt.a.Sub(tt.b), fmt.Sprintf("%v - %v", tt.a, tt.b))
	}
}

func TestNodeID_DistanceFrom(t *testing.T) {
	tests := []struct {
		a      *NodeID
		b      *NodeID
		expect *NodeID
	}{
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 0),
			expect: NewNormalNodeID(0, 0),
		},
		{
			a:      NewNormalNodeID(0, 1),
			b:      NewNormalNodeID(0, 0),
			expect: NewNormalNodeID(0, 1),
		},
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(0, 1),
			expect: NewNormalNodeID(0, 1),
		},
		{
			a:      NewNormalNodeID(0, 0),
			b:      NewNormalNodeID(^uint64(0), ^uint64(0)),
			expect: NewNormalNodeID(0, 1),
		},
		{
			a:      NewNormalNodeID(^uint64(0), ^uint64(0)),
			b:      NewNormalNodeID(0, 0),
			expect: NewNormalNodeID(0, 1),
		},
	}

	for _, tt := range tests {
		assert.True(t, tt.expect.Equal(tt.a.DistanceFrom(tt.b)))
	}
}

func TestNodeID_MapKey(t *testing.T) {
	origin := NewRandomNodeID()
	clone := NewNodeIDFromProto(origin.Proto())

	m := make(map[NodeID]int)
	m[*origin] = 1
	m[*clone] = 2

	if len(m) != 1 {
		assert.Fail(t, "NodeID should be equal")
	}

	other := NewRandomNodeID()
	for origin.Equal(other) {
		other = NewRandomNodeID()
	}

	m[*other] = 3
	assert.Len(t, m, 2)
	assert.Equal(t, 2, m[*origin])
	assert.Equal(t, 3, m[*other])
}
