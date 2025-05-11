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
	"github.com/stretchr/testify/require"
)

func TestNodeID_NewNodeIDFromProto(t *testing.T) {
	tests := []struct {
		input    *proto.NodeID
		expect   *NodeID
		hasError bool
	}{
		{
			input: &proto.NodeID{
				Id0: 1,
				Id1: 1,
			},
			expect: &NodeID{
				t:   typeNormal,
				id0: 1,
				id1: 1,
			},
		},
		{
			input:    nil,
			expect:   nil,
			hasError: true,
		},
	}

	for _, test := range tests {
		nodeID, err := NewNodeIDFromProto(test.input)
		if test.expect != nil {
			assert.True(t, test.expect.Equal(nodeID))
		}
		if test.hasError {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestNodeID_NewNodeIDFromString(t *testing.T) {
	tests := []struct {
		input    string
		expect   *NodeID
		hasError bool
	}{
		{
			input:  "00000000000000010000000000000001",
			expect: NewNormalNodeID(1, 1),
		},
		{
			input:    strLocal,
			hasError: true,
		},
		{
			input:    strNeighborhoods,
			hasError: true,
		},
		{
			input:    "invalid",
			hasError: true,
		},
		{
			input:    "0x000000000000010000000000000001",
			hasError: true,
		},
	}

	for _, test := range tests {
		t.Log(test.input)
		nodeID, err := NewNodeIDFromString(test.input)
		if test.expect != nil {
			assert.True(t, test.expect.Equal(nodeID))
		}
		if test.hasError {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestNodeID_NewNormalNodeID(t *testing.T) {
	nodeID := NewNormalNodeID(1, 2)
	assert.Equal(t, typeNormal, nodeID.t)
	assert.Equal(t, uint64(1), nodeID.id0)
	assert.Equal(t, uint64(2), nodeID.id1)
}

func TestNodeID_NewRandomNodeID(t *testing.T) {
	nodeID := NewRandomNodeID()
	assert.Equal(t, typeNormal, nodeID.t)
	// It is a random number, so it may rarely be zero
	assert.NotEqual(t, 0, nodeID.id0)
	assert.NotEqual(t, 0, nodeID.id1)
}

func TestNodeID_Copy(t *testing.T) {
	tests := []*NodeID{
		NewRandomNodeID(),
		&NodeLocal,
		&NodeNeighborhoods,
	}

	for _, nodeID := range tests {
		copied := nodeID.Copy()
		assert.True(t, nodeID.Equal(copied))
	}
}

func TestNodeID_Proto(t *testing.T) {
	nodeID := NewRandomNodeID()
	protoNodeID := nodeID.Proto()
	assert.Equal(t, nodeID.id0, protoNodeID.Id0)
	assert.Equal(t, nodeID.id1, protoNodeID.Id1)
}

func TestNodeID_String(t *testing.T) {
	tests := []struct {
		nodeID *NodeID
		expect string
	}{
		{
			nodeID: NewNormalNodeID(1, 2),
			expect: "00000000000000010000000000000002",
		},
		{
			nodeID: &NodeLocal,
			expect: strLocal,
		},
		{
			nodeID: &NodeNeighborhoods,
			expect: strNeighborhoods,
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expect, test.nodeID.String())
	}
}

func TestNodeID_Raw(t *testing.T) {
	nodeID := NewRandomNodeID()
	id0, id1 := nodeID.Raw()
	assert.Equal(t, nodeID.id0, id0)
	assert.Equal(t, nodeID.id1, id1)
}

func TestNodeID_IsNormal(t *testing.T) {
	tests := []struct {
		nodeID *NodeID
		expect bool
	}{
		{
			nodeID: NewNormalNodeID(1, 2),
			expect: true,
		},
		{
			nodeID: NewRandomNodeID(),
			expect: true,
		},
		{
			nodeID: &NodeLocal,
			expect: false,
		},
		{
			nodeID: &NodeNeighborhoods,
			expect: false,
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expect, test.nodeID.IsNormal())
	}
}

func TestNodeID_Equal(t *testing.T) {
	patterns := []*NodeID{
		NewNormalNodeID(0, 1),
		NewNormalNodeID(0, 2),
		nil,
		&NodeLocal,
		&NodeNeighborhoods,
	}

	for i, nodeID1 := range patterns {
		for j, nodeID2 := range patterns {
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
			a:      &NodeLocal,
			b:      NewNormalNodeID(0, 0),
			expect: false,
		},
		{
			a:      NewNormalNodeID(9, 9),
			b:      &NodeLocal,
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
			a:      &NodeLocal,
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

func TestNodeID_ConvertNodeIDSetToStringMap(t *testing.T) {
	src := map[NodeID]struct{}{
		*NewNormalNodeID(1, 2): {},
		*NewNormalNodeID(3, 4): {},
	}

	dst := ConvertNodeIDSetToStringMap(src)
	assert.Len(t, dst, 2)
	assert.Contains(t, dst, "00000000000000010000000000000002")
	assert.Contains(t, dst, "00000000000000030000000000000004")
}

func TestNodeID_MapKey(t *testing.T) {
	origin := NewRandomNodeID()
	clone, err := NewNodeIDFromProto(origin.Proto())
	require.NoError(t, err)

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
