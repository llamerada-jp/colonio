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
package routing

import (
	"testing"

	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouting1D_updateNodeConnection(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(6)
	util.SortNodeIDs(nodeIDs)
	r1d := newRouting1D(&routing1DConfig{
		localNodeID: nodeIDs[1],
	})

	tests := []struct {
		nodeIDs                 map[shared.NodeID]struct{}
		expectResult            bool
		expectConnectedInfosLen int
		expectPrevNodeID        *shared.NodeID
		expectNextNodeID        *shared.NodeID
	}{
		{
			nodeIDs: map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
			},
			expectResult:            true,
			expectConnectedInfosLen: 1,
			expectPrevNodeID:        nodeIDs[0],
			expectNextNodeID:        nodeIDs[0],
		},
		{
			nodeIDs: map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
				*nodeIDs[3]: {},
				*nodeIDs[5]: {},
			},
			expectResult:            true,
			expectConnectedInfosLen: 3,
			expectPrevNodeID:        nodeIDs[0],
			expectNextNodeID:        nodeIDs[3],
		},
		{
			nodeIDs: map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
				*nodeIDs[3]: {},
				*nodeIDs[4]: {},
				*nodeIDs[5]: {},
			},
			expectResult:            false,
			expectConnectedInfosLen: 4,
			expectPrevNodeID:        nodeIDs[0],
			expectNextNodeID:        nodeIDs[3],
		},
		{
			nodeIDs: map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
				*nodeIDs[4]: {},
				*nodeIDs[5]: {},
			},
			expectResult:            true,
			expectConnectedInfosLen: 3,
			expectPrevNodeID:        nodeIDs[0],
			expectNextNodeID:        nodeIDs[4],
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)

		r := r1d.updateNodeConnections(tt.nodeIDs)
		assert.Equal(t, tt.expectResult, r)
		assert.Len(t, r1d.connectedInfos, tt.expectConnectedInfosLen)
		assert.Equal(t, *tt.expectPrevNodeID, *r1d.prevNodeID)
		assert.Equal(t, *tt.expectNextNodeID, *r1d.nextNodeID)
	}
}

func TestRouting1D_getNextStep_offline(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(3)
	localNodeID := nodeIDs[0]

	r1d := newRouting1D(&routing1DConfig{
		localNodeID: localNodeID,
	})

	tests := []struct {
		dstNodeID *shared.NodeID
		mode      shared.PacketMode
		expect    *shared.NodeID
	}{
		{
			dstNodeID: nodeIDs[1],
			mode:      0,
			expect:    &shared.NodeIDThis,
		},
		{
			dstNodeID: nodeIDs[1],
			mode:      shared.PacketModeExplicit,
			expect:    &shared.NodeIDNone,
		},
		{
			dstNodeID: localNodeID,
			mode:      0,
			expect:    &shared.NodeIDThis,
		},
		{
			dstNodeID: &shared.NodeIDThis,
			mode:      0,
			expect:    &shared.NodeIDThis,
		},
		{
			dstNodeID: &shared.NodeIDNext,
			mode:      0,
			expect:    &shared.NodeIDNext,
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)

		r := r1d.getNextStep(&shared.Packet{
			DstNodeID: tt.dstNodeID,
			SrcNodeID: shared.NewRandomNodeID(),
			Mode:      tt.mode,
		})
		assert.Equal(t, *r, *tt.expect)
	}
}

func TestRouting1D_getNextStep_online(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(6)
	util.SortNodeIDs(nodeIDs)
	localNodeID := nodeIDs[1]

	r1d := newRouting1D(&routing1DConfig{
		localNodeID: localNodeID,
	})
	r1d.prevNodeID = nodeIDs[0]
	r1d.nextNodeID = nodeIDs[2]
	r1d.routeInfos = []*routeInfo1D{
		{
			nodeID:          nodeIDs[0],
			connectedNodeID: nodeIDs[0],
		},
		{
			nodeID:          nodeIDs[2],
			connectedNodeID: nodeIDs[4],
		},
		{
			nodeID:          nodeIDs[3],
			connectedNodeID: nodeIDs[4],
		},
		{
			nodeID:          nodeIDs[4],
			connectedNodeID: nodeIDs[4],
		},
		{
			nodeID:          nodeIDs[5],
			connectedNodeID: nodeIDs[5],
		},
	}

	one := shared.NewNormalNodeID(0, 1)
	tests := []struct {
		dstNodeID *shared.NodeID
		explicit  bool
		expect    *shared.NodeID
	}{
		{
			dstNodeID: nodeIDs[0],
			explicit:  false,
			expect:    nodeIDs[0],
		},
		{
			dstNodeID: util.UniqueNodeIDsWithRange(nodeIDs[0].Add(one), nodeIDs[1], 1)[0],
			explicit:  false,
			expect:    nodeIDs[0],
		},
		{
			dstNodeID: util.UniqueNodeIDsWithRange(nodeIDs[0].Add(one), nodeIDs[1], 1)[0],
			explicit:  true,
			expect:    &shared.NodeIDNone,
		},
		{
			dstNodeID: localNodeID,
			explicit:  false,
			expect:    &shared.NodeIDThis,
		},
		{
			dstNodeID: util.UniqueNodeIDsWithRange(nodeIDs[1].Add(one), nodeIDs[2], 1)[0],
			explicit:  false,
			expect:    &shared.NodeIDThis,
		},
		{
			dstNodeID: util.UniqueNodeIDsWithRange(nodeIDs[1].Add(one), nodeIDs[2], 1)[0],
			explicit:  true,
			expect:    &shared.NodeIDNone,
		},
		{
			dstNodeID: util.UniqueNodeIDsWithRange(nodeIDs[2].Add(one), nodeIDs[4], 1)[0],
			explicit:  false,
			expect:    nodeIDs[4],
		},
		{
			dstNodeID: util.UniqueNodeIDsWithRange(nodeIDs[2].Add(one), nodeIDs[4], 1)[0],
			explicit:  true,
			expect:    nodeIDs[4],
		},
		{
			dstNodeID: util.UniqueNodeIDsWithRange(nodeIDs[5].Add(one), shared.NewNormalNodeID(^uint64(0), ^uint64(0)), 1)[0],
			explicit:  false,
			expect:    nodeIDs[5],
		},
		{
			dstNodeID: util.UniqueNodeIDsWithRange(shared.NewNormalNodeID(0, 0), nodeIDs[0], 1)[0],
			explicit:  false,
			expect:    nodeIDs[5],
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)
		mode := shared.PacketModeNone
		if tt.explicit {
			mode = shared.PacketModeExplicit
		}
		r := r1d.getNextStep(&shared.Packet{
			DstNodeID: tt.dstNodeID,
			SrcNodeID: shared.NewRandomNodeID(),
			Mode:      mode,
		})
		assert.Equal(t, *r, *tt.expect)
	}
}

func TestRouting1D_countRecvPacket(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(3)
	util.SortNodeIDs(nodeIDs)

	r1d := newRouting1D(&routing1DConfig{
		localNodeID: nodeIDs[0],
	})
	r1d.prevNodeID = nodeIDs[1]
	r1d.nextNodeID = nodeIDs[1]
	r1d.connectedInfos[*nodeIDs[1]] = &connectedInfo{
		connectedNodeIDs: map[shared.NodeID]struct{}{
			*nodeIDs[0]: {},
		},
		scoreByRecv: 1,
	}
	r1d.routeInfos = []*routeInfo1D{
		{
			nodeID:          nodeIDs[1],
			connectedNodeID: nodeIDs[1],
			scoreBySend:     3,
		},
	}

	r1d.countRecvPacket(nodeIDs[1])
	assert.Equal(t, 2, r1d.connectedInfos[*nodeIDs[1]].scoreByRecv)
	assert.Equal(t, 3, r1d.routeInfos[0].scoreBySend)

	r1d.countRecvPacket(nodeIDs[2])
	assert.Equal(t, 2, r1d.connectedInfos[*nodeIDs[1]].scoreByRecv)
	assert.Equal(t, 3, r1d.routeInfos[0].scoreBySend)
}

func TestRouting1D_recvRoutingPacket(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(4)
	util.SortNodeIDs(nodeIDs)

	r1d := newRouting1D(&routing1DConfig{
		localNodeID: nodeIDs[0],
	})
	r1d.prevNodeID = nodeIDs[1]
	r1d.nextNodeID = nodeIDs[1]
	r1d.connectedInfos[*nodeIDs[1]] = &connectedInfo{
		connectedNodeIDs: map[shared.NodeID]struct{}{
			*nodeIDs[0]: {},
		},
	}
	r1d.connectedInfos[*nodeIDs[2]] = &connectedInfo{
		connectedNodeIDs: map[shared.NodeID]struct{}{
			*nodeIDs[0]: {},
		},
		oddScore: 1,
	}
	r1d.routeInfos = []*routeInfo1D{
		{
			nodeID:          nodeIDs[1],
			connectedNodeID: nodeIDs[1],
		},
	}

	r := r1d.recvRoutingPacket(nodeIDs[2], &proto.Routing{
		NodeRecords: map[string]*proto.RoutingNodeRecord{
			nodeIDs[0].String(): {
				R1DScore: 3,
			},
			nodeIDs[3].String(): {
				R1DScore: 4,
			},
		},
	})
	assert.True(t, r)
	assert.Len(t, r1d.connectedInfos, 2)
	info := r1d.connectedInfos[*nodeIDs[2]]
	assert.Equal(t, 3, info.oddScore)
	assert.Len(t, info.connectedNodeIDs, 2)
	assert.Contains(t, info.connectedNodeIDs, *nodeIDs[0], *nodeIDs[3])

	r = r1d.recvRoutingPacket(nodeIDs[2], &proto.Routing{
		NodeRecords: map[string]*proto.RoutingNodeRecord{
			nodeIDs[0].String(): {
				R1DScore: 4,
			},
		},
	})
	assert.True(t, r)
	info = r1d.connectedInfos[*nodeIDs[2]]
	assert.Equal(t, 4, info.oddScore)
	assert.Len(t, info.connectedNodeIDs, 1)
	assert.Contains(t, info.connectedNodeIDs, *nodeIDs[0])
}

func TestRouting1D_setupRoutingPacket(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(4)
	util.SortNodeIDs(nodeIDs)

	r1d := newRouting1D(&routing1DConfig{
		localNodeID: nodeIDs[0],
	})
	r1d.prevNodeID = nodeIDs[1]
	r1d.nextNodeID = nodeIDs[1]
	r1d.connectedInfos[*nodeIDs[1]] = &connectedInfo{
		connectedNodeIDs: map[shared.NodeID]struct{}{
			*nodeIDs[0]: {},
		},
	}
	r1d.connectedInfos[*nodeIDs[2]] = &connectedInfo{
		connectedNodeIDs: map[shared.NodeID]struct{}{
			*nodeIDs[0]: {},
		},
		scoreByRecv: 1,
	}

	content := &proto.Routing{
		NodeRecords: map[string]*proto.RoutingNodeRecord{
			nodeIDs[1].String(): {
				R2DPosition: &proto.Coordinate{
					X: 1,
					Y: 2,
				},
			},
		},
	}
	r1d.setupRoutingPacket(content)
	assert.Len(t, content.NodeRecords, 2)
	assert.Contains(t, content.NodeRecords, nodeIDs[1].String(), nodeIDs[2].String())
	assert.Equal(t, float64(1), content.NodeRecords[nodeIDs[1].String()].R2DPosition.GetX())
	assert.Equal(t, float64(2), content.NodeRecords[nodeIDs[1].String()].R2DPosition.GetY())
	assert.Equal(t, int64(1), content.NodeRecords[nodeIDs[2].String()].GetR1DScore())
}

func TestRouting1D_getConnections(t *testing.T) {
	// TODO: implement tests after simulation
}

func TestRouting1D_calcLevel(t *testing.T) {
	prev := shared.NewNormalNodeID(0, 0)
	for expect, r := range levelRanges {
		nodeID := util.UniqueNodeIDsWithRange(prev, r, 1)[0]
		localNodeID := shared.NewRandomNodeID()
		r1d := newRouting1D(&routing1DConfig{
			localNodeID: localNodeID,
		})
		level := r1d.calcLevel(localNodeID.Add(nodeID))
		assert.Equal(t, expect, level)
		prev = r
	}

	// expect -1 if nodeID is not in the range
	nodeID := util.UniqueNodeIDsWithRange(prev, shared.NewNormalNodeID(^uint64(0), ^uint64(0)), 1)[0]
	localNodeID := shared.NewRandomNodeID()
	r1d := newRouting1D(&routing1DConfig{
		localNodeID: localNodeID,
	})
	level := r1d.calcLevel(localNodeID.Add(nodeID))
	assert.Equal(t, -1, level)
}

func TestRouting1D_updateRouteInfos(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(6)
	util.SortNodeIDs(nodeIDs)
	for nodeIDs[3].DistanceFrom(nodeIDs[2]).Smaller(nodeIDs[3].DistanceFrom(nodeIDs[4])) {
		nodeIDs = util.UniqueNodeIDs(6)
		util.SortNodeIDs(nodeIDs)
	}

	tests := []struct {
		connected        map[shared.NodeID][]*shared.NodeID
		routeInfos       []*routeInfo1D
		expectRouteInfos []*routeInfo1D
	}{
		{
			connected:        map[shared.NodeID][]*shared.NodeID{},
			routeInfos:       []*routeInfo1D{},
			expectRouteInfos: []*routeInfo1D{},
		},
		{
			connected: map[shared.NodeID][]*shared.NodeID{},
			routeInfos: []*routeInfo1D{
				{
					nodeID:          nodeIDs[1],
					connectedNodeID: nodeIDs[2],
					scoreBySend:     1,
				},
			},
			expectRouteInfos: []*routeInfo1D{},
		},
		{ // add
			connected: map[shared.NodeID][]*shared.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[2]},
				*nodeIDs[2]: {nodeIDs[1], nodeIDs[3]},
			},
			routeInfos: []*routeInfo1D{
				{
					nodeID:          nodeIDs[1],
					connectedNodeID: nodeIDs[1],
					scoreBySend:     1,
				},
				{
					nodeID:          nodeIDs[2],
					connectedNodeID: nodeIDs[1],
					scoreBySend:     2,
				},
			},
			expectRouteInfos: []*routeInfo1D{
				{
					nodeID:          nodeIDs[1],
					connectedNodeID: nodeIDs[1],
					scoreBySend:     1,
				},
				{
					nodeID:          nodeIDs[2],
					connectedNodeID: nodeIDs[2],
					scoreBySend:     2,
				},
				{
					nodeID:          nodeIDs[3],
					connectedNodeID: nodeIDs[2],
					scoreBySend:     0,
				},
			},
		},
		{ // remove
			connected: map[shared.NodeID][]*shared.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[2]},
			},
			routeInfos: []*routeInfo1D{
				{
					nodeID:          nodeIDs[1],
					connectedNodeID: nodeIDs[1],
					scoreBySend:     1,
				},
				{
					nodeID:          nodeIDs[2],
					connectedNodeID: nodeIDs[2],
					scoreBySend:     2,
				},
				{
					nodeID:          nodeIDs[3],
					connectedNodeID: nodeIDs[2],
					scoreBySend:     0,
				},
			},
			expectRouteInfos: []*routeInfo1D{
				{
					nodeID:          nodeIDs[1],
					connectedNodeID: nodeIDs[1],
					scoreBySend:     1,
				},
				{
					nodeID:          nodeIDs[2],
					connectedNodeID: nodeIDs[1],
					scoreBySend:     2,
				},
			},
		},
		{ // reroute to nearest node
			connected: map[shared.NodeID][]*shared.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[2]},
				*nodeIDs[2]: {nodeIDs[1], nodeIDs[3]},
				*nodeIDs[4]: {nodeIDs[3]}},
			routeInfos: []*routeInfo1D{
				{
					nodeID:          nodeIDs[1],
					connectedNodeID: nodeIDs[1],
					scoreBySend:     1,
				},
				{
					nodeID:          nodeIDs[2],
					connectedNodeID: nodeIDs[2],
					scoreBySend:     2,
				},
				{
					nodeID:          nodeIDs[3],
					connectedNodeID: nodeIDs[2],
					scoreBySend:     3,
				},
			},
			expectRouteInfos: []*routeInfo1D{
				{
					nodeID:          nodeIDs[1],
					connectedNodeID: nodeIDs[1],
					scoreBySend:     1,
				},
				{
					nodeID:          nodeIDs[2],
					connectedNodeID: nodeIDs[2],
					scoreBySend:     2,
				},
				{
					nodeID:          nodeIDs[3],
					connectedNodeID: nodeIDs[4],
					scoreBySend:     3,
				},
				{
					nodeID:          nodeIDs[4],
					connectedNodeID: nodeIDs[4],
					scoreBySend:     0,
				},
			},
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)

		r1d := newRouting1D(&routing1DConfig{
			localNodeID: nodeIDs[0],
		})
		r1d.connectedInfos = make(map[shared.NodeID]*connectedInfo)
		for connectedNodeID, connected := range tt.connected {
			c := make(map[shared.NodeID]struct{})
			for _, nodeID := range connected {
				c[*nodeID] = struct{}{}
			}
			r1d.connectedInfos[connectedNodeID] = &connectedInfo{
				connectedNodeIDs: c,
			}
		}
		r1d.routeInfos = tt.routeInfos

		r1d.updateRouteInfos()
		require.Len(t, r1d.routeInfos, len(tt.expectRouteInfos))
		for i, routeInfo := range r1d.routeInfos {
			assert.Equal(t, *tt.expectRouteInfos[i].nodeID, *routeInfo.nodeID)
			assert.Equal(t, *tt.expectRouteInfos[i].connectedNodeID, *routeInfo.connectedNodeID)
			assert.Equal(t, tt.expectRouteInfos[i].scoreBySend, routeInfo.scoreBySend)
		}
	}
}

func TestRouting1D_nextNodeIDChanged(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(4)
	util.SortNodeIDs(nodeIDs)

	tests := []struct {
		localNodeID *shared.NodeID
		prevNodeID  *shared.NodeID
		nextNodeID  *shared.NodeID
		connected   map[shared.NodeID][]*shared.NodeID
		expect      bool
		expectPrev  *shared.NodeID
		expectNext  *shared.NodeID
	}{
		{
			localNodeID: nodeIDs[0],
			prevNodeID:  nodeIDs[2],
			nextNodeID:  nodeIDs[1],
			connected: map[shared.NodeID][]*shared.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[2]},
				*nodeIDs[2]: {nodeIDs[0], nodeIDs[1]},
			},
			expect:     false,
			expectPrev: nodeIDs[2],
			expectNext: nodeIDs[1],
		},
		{
			localNodeID: nodeIDs[0],
			prevNodeID:  nodeIDs[2],
			nextNodeID:  nodeIDs[1],
			connected: map[shared.NodeID][]*shared.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[2]},
				*nodeIDs[2]: {nodeIDs[0], nodeIDs[1]},
				*nodeIDs[3]: {nodeIDs[0], nodeIDs[1]},
			},
			expect:     true,
			expectPrev: nodeIDs[3],
			expectNext: nodeIDs[1],
		},
		{
			localNodeID: nodeIDs[0],
			prevNodeID:  nodeIDs[2],
			nextNodeID:  nodeIDs[1],
			connected: map[shared.NodeID][]*shared.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[2]},
				*nodeIDs[2]: {nodeIDs[0], nodeIDs[3]},
			},
			expect:     true,
			expectPrev: nodeIDs[3],
			expectNext: nodeIDs[1],
		},
		{
			localNodeID: nodeIDs[0],
			prevNodeID:  nodeIDs[2],
			nextNodeID:  nodeIDs[1],
			connected: map[shared.NodeID][]*shared.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[3]},
			},
			expect:     true,
			expectPrev: nodeIDs[3],
			expectNext: nodeIDs[1],
		},
		{
			localNodeID: nodeIDs[0],
			prevNodeID:  nodeIDs[2],
			nextNodeID:  nodeIDs[1],
			connected:   map[shared.NodeID][]*shared.NodeID{},
			expect:      true,
			expectPrev:  &shared.NodeIDNone,
			expectNext:  &shared.NodeIDNone,
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)

		r1d := newRouting1D(&routing1DConfig{
			localNodeID: tt.localNodeID,
		})
		r1d.nextNodeID = tt.nextNodeID
		r1d.prevNodeID = tt.prevNodeID
		r1d.connectedInfos = make(map[shared.NodeID]*connectedInfo)
		for nodeID, connected := range tt.connected {
			connectedNodeIDs := make(map[shared.NodeID]struct{})
			for _, nodeID := range connected {
				connectedNodeIDs[*nodeID] = struct{}{}
			}
			r1d.connectedInfos[nodeID] = &connectedInfo{
				connectedNodeIDs: connectedNodeIDs,
			}
		}

		r := r1d.nextNodeIDChanged()
		assert.Equal(t, tt.expect, r)
		assert.Equal(t, tt.expectNext, r1d.nextNodeID)
		assert.Equal(t, tt.expectPrev, r1d.prevNodeID)
	}
}

func TestRouting1D_normalizeScore(t *testing.T) {
	tests := []struct {
		score  []int
		expect []int
	}{
		{
			score:  []int{1, 2, 3},
			expect: []int{1, 2, 3},
		},
		{
			score:  []int{1024 * 1024, 1024 * 1024},
			expect: []int{512 * 1024, 512 * 1024},
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)

		r1d := newRouting1D(&routing1DConfig{})
		nodeIDs := util.UniqueNodeIDs(len(tt.score))
		for i, score := range tt.score {
			r1d.connectedInfos[*nodeIDs[i]] = &connectedInfo{
				scoreByRecv: score,
			}
			r1d.routeInfos = append(r1d.routeInfos, &routeInfo1D{
				nodeID:      nodeIDs[i],
				scoreBySend: score,
			})
		}
		r1d.normalizeScore()
		for i, nodeID := range nodeIDs {
			assert.Equal(t, tt.expect[i], r1d.connectedInfos[*nodeID].scoreByRecv)
			assert.Equal(t, tt.expect[i], r1d.routeInfos[i].scoreBySend)
		}
	}
}

func TestRouting1D_isBetween(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(3)
	util.SortNodeIDs(nodeIDs)

	tests := []struct {
		a, b, target *shared.NodeID
		expect       bool
	}{
		{
			a:      nodeIDs[0],
			b:      nodeIDs[2],
			target: nodeIDs[1],
			expect: true,
		},
		{
			a:      nodeIDs[1],
			b:      nodeIDs[0],
			target: nodeIDs[2],
			expect: true,
		},
		{
			a:      nodeIDs[2],
			b:      nodeIDs[1],
			target: nodeIDs[0],
			expect: true,
		},
		{
			a:      nodeIDs[0],
			b:      nodeIDs[1],
			target: nodeIDs[2],
			expect: false,
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)

		assert.Equal(t, tt.expect, isBetween(tt.a, tt.b, tt.target))
	}
}

func TestRouting1D_getNextAndPrevNodeID(t *testing.T) {
	nodeIDs := util.UniqueNodeIDs(4)
	util.SortNodeIDs(nodeIDs)

	tests := []struct {
		base       *shared.NodeID
		nodeIDs    []*shared.NodeID
		expectNext *shared.NodeID
		expectPrev *shared.NodeID
	}{
		{
			base:       nodeIDs[0],
			nodeIDs:    []*shared.NodeID{},
			expectNext: &shared.NodeIDNone,
			expectPrev: &shared.NodeIDNone,
		},
		{
			base:       nodeIDs[0],
			nodeIDs:    []*shared.NodeID{nodeIDs[0]},
			expectNext: &shared.NodeIDNone,
			expectPrev: &shared.NodeIDNone,
		},
		{
			base:       nodeIDs[0],
			nodeIDs:    []*shared.NodeID{nodeIDs[1]},
			expectNext: nodeIDs[1],
			expectPrev: nodeIDs[1],
		},
		{
			base:       nodeIDs[0],
			nodeIDs:    []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[2], nodeIDs[3]},
			expectNext: nodeIDs[1],
			expectPrev: nodeIDs[3],
		},
		{
			base:       nodeIDs[3],
			nodeIDs:    []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[2]},
			expectNext: nodeIDs[0],
			expectPrev: nodeIDs[2],
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)

		nodeIDs := make(map[shared.NodeID]struct{})
		for _, nodeID := range tt.nodeIDs {
			nodeIDs[*nodeID] = struct{}{}
		}
		next, prev := getNextAndPrevNodeID(tt.base, nodeIDs)
		assert.Equal(t, *tt.expectNext, *next)
		assert.Equal(t, *tt.expectPrev, *prev)
	}
}
