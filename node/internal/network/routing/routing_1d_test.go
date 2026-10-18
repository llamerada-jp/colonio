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

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouting1D_subRoutine(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(4)
	reconcileResult := false
	reconcileCalled := 0

	r1d := newRouting1D(&routing1DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: nodeIDs[0],
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			reconcileCalled++
			return reconcileResult, nil
		},
	})

	require.Equal(t, 0, reconcileCalled)
	// reconcileNextNodes should be called once
	r1d.subRoutine()
	assert.Equal(t, 1, reconcileCalled)

	// if reconcileNextNodes returns false, it should not be called again
	reconcileResult = true
	r1d.subRoutine()
	assert.Equal(t, 2, reconcileCalled)
	r1d.subRoutine()
	assert.Equal(t, 2, reconcileCalled)

	// if nextNodeMatched is false, it should be called again
	r1d.nextNodeMatched = false
	r1d.subRoutine()
	assert.Equal(t, 3, reconcileCalled)
}

func TestRouting1D_updateNodeConnections(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(4)
	types.SortNodeIDs(nodeIDs)
	r1d := newRouting1D(&routing1DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: nodeIDs[0],
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			return true, nil
		},
	})

	tests := []struct {
		name              string
		neighborhoodInfos map[types.NodeID]*neighborhoodInfo
		nodeIDs           map[types.NodeID]struct{}
		expect            int
	}{
		{
			name:              "initial connections",
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{},
			nodeIDs: map[types.NodeID]struct{}{
				*nodeIDs[1]: {},
			},
			expect: requireExchangeRouting,
		},
		{
			name: "no connections",
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {
					secondNeighborhoods: map[types.NodeID]struct{}{},
				},
			},
			nodeIDs: map[types.NodeID]struct{}{},
			expect:  requireExchangeRouting,
		},
		{
			name: "new connection",
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {
					secondNeighborhoods: map[types.NodeID]struct{}{
						*nodeIDs[2]: {},
					},
				},
				*nodeIDs[2]: {
					secondNeighborhoods: map[types.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
			},
			nodeIDs: map[types.NodeID]struct{}{
				*nodeIDs[1]: {},
				*nodeIDs[2]: {},
				*nodeIDs[3]: {},
			},
			expect: requireExchangeRouting,
		},
		{
			name: "disconnected",
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {
					secondNeighborhoods: map[types.NodeID]struct{}{
						*nodeIDs[2]: {},
					},
				},
				*nodeIDs[2]: {
					secondNeighborhoods: map[types.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
			},
			nodeIDs: map[types.NodeID]struct{}{
				*nodeIDs[1]: {},
			},
			expect: requireExchangeRouting | requireUpdateConnections,
		},
	}

	for _, tt := range tests {
		t.Logf("%s", tt.name)

		r1d.neighborhoodInfos = tt.neighborhoodInfos
		r := r1d.updateNodeConnections(tt.nodeIDs)
		assert.Equal(t, tt.expect, r)
		assert.Len(t, r1d.neighborhoodInfos, len(tt.nodeIDs))
		for nodeID, info := range r1d.neighborhoodInfos {
			assert.Contains(t, tt.nodeIDs, nodeID)
			if _, ok := tt.neighborhoodInfos[nodeID]; ok {
				assert.Equal(t, tt.neighborhoodInfos[nodeID].secondNeighborhoods, info.secondNeighborhoods)
			} else {
				assert.Len(t, info.secondNeighborhoods, 0)
			}
		}
	}
}

func TestRouting1D_getNextStep_offline(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	localNodeID := nodeIDs[0]

	r1d := newRouting1D(&routing1DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: localNodeID,
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			return true, nil
		},
	})

	tests := []struct {
		dstNodeID *types.NodeID
		mode      networkTypes.PacketMode
		expect    *types.NodeID
	}{
		{
			dstNodeID: nodeIDs[1],
			mode:      0,
			expect:    &types.NodeLocal,
		},
		{
			dstNodeID: nodeIDs[1],
			mode:      networkTypes.PacketModeExplicit,
			expect:    nil,
		},
		{
			dstNodeID: localNodeID,
			mode:      0,
			expect:    &types.NodeLocal,
		},
		{
			dstNodeID: &types.NodeLocal,
			mode:      0,
			expect:    &types.NodeLocal,
		},
		{
			dstNodeID: &types.NodeNeighborhoods,
			mode:      0,
			expect:    &types.NodeNeighborhoods,
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)

		r := r1d.getNextStep(&networkTypes.Packet{
			DstNodeID: tt.dstNodeID,
			SrcNodeID: types.NewRandomNodeID(),
			Mode:      tt.mode,
		})
		assert.Equal(t, r, tt.expect)
	}
}

func TestRouting1D_getNextStep_online(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(8)
	types.SortNodeIDs(nodeIDs)
	localNodeID := nodeIDs[1]

	r1d := newRouting1D(&routing1DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: localNodeID,
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			return true, nil
		},
	})
	r1d.backwardNextNodeIDs = []*types.NodeID{nodeIDs[7], nodeIDs[0]}
	r1d.frontwardNextNodeIDs = []*types.NodeID{nodeIDs[2], nodeIDs[3]}
	r1d.routeInfos = []*routeInfo1D{
		{
			nodeID:            nodeIDs[0],
			firstNeighborhood: nodeIDs[0],
		},
		{
			nodeID:            nodeIDs[2],
			firstNeighborhood: nodeIDs[4],
		},
		{
			nodeID:            nodeIDs[3],
			firstNeighborhood: nodeIDs[4],
		},
		{
			nodeID:            nodeIDs[4],
			firstNeighborhood: nodeIDs[4],
		},
		{
			nodeID:            nodeIDs[5],
			firstNeighborhood: nodeIDs[5],
		},
	}

	one := types.NewNormalNodeID(0, 1)
	tests := []struct {
		dstNodeID *types.NodeID
		explicit  bool
		expect    *types.NodeID
	}{
		{
			dstNodeID: nodeIDs[0],
			explicit:  false,
			expect:    nodeIDs[0],
		},
		{
			dstNodeID: testUtil.UniqueNodeIDsWithRange(nodeIDs[0].Add(one), nodeIDs[1], 1)[0],
			explicit:  false,
			expect:    nodeIDs[0],
		},
		{
			dstNodeID: testUtil.UniqueNodeIDsWithRange(nodeIDs[0].Add(one), nodeIDs[1], 1)[0],
			explicit:  true,
			expect:    nil,
		},
		{
			dstNodeID: localNodeID,
			explicit:  false,
			expect:    &types.NodeLocal,
		},
		{
			dstNodeID: testUtil.UniqueNodeIDsWithRange(nodeIDs[1].Add(one), nodeIDs[2], 1)[0],
			explicit:  false,
			expect:    &types.NodeLocal,
		},
		{
			dstNodeID: testUtil.UniqueNodeIDsWithRange(nodeIDs[1].Add(one), nodeIDs[2], 1)[0],
			explicit:  true,
			expect:    nil,
		},
		{
			dstNodeID: testUtil.UniqueNodeIDsWithRange(nodeIDs[2].Add(one), nodeIDs[4], 1)[0],
			explicit:  false,
			expect:    nodeIDs[4],
		},
		{
			dstNodeID: testUtil.UniqueNodeIDsWithRange(nodeIDs[2].Add(one), nodeIDs[4], 1)[0],
			explicit:  true,
			expect:    nodeIDs[4],
		},
		{
			dstNodeID: testUtil.UniqueNodeIDsWithRange(nodeIDs[5].Add(one), types.NewNormalNodeID(^uint64(0), ^uint64(0)), 1)[0],
			explicit:  false,
			expect:    nodeIDs[5],
		},
		{
			dstNodeID: testUtil.UniqueNodeIDsWithRange(types.NewNormalNodeID(0, 0), nodeIDs[0], 1)[0],
			explicit:  false,
			expect:    nodeIDs[5],
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)
		mode := networkTypes.PacketModeNone
		if tt.explicit {
			mode = networkTypes.PacketModeExplicit
		}
		r := r1d.getNextStep(&networkTypes.Packet{
			DstNodeID: tt.dstNodeID,
			SrcNodeID: types.NewRandomNodeID(),
			Mode:      mode,
		})
		assert.Equal(t, r, tt.expect)
	}
}

func TestRouting1D_countRecvPacket(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	types.SortNodeIDs(nodeIDs)

	r1d := newRouting1D(&routing1DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: nodeIDs[0],
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			return true, nil
		},
	})
	r1d.backwardNextNodeIDs = []*types.NodeID{nodeIDs[1]}
	r1d.frontwardNextNodeIDs = []*types.NodeID{nodeIDs[1]}
	r1d.neighborhoodInfos[*nodeIDs[1]] = &neighborhoodInfo{
		secondNeighborhoods: map[types.NodeID]struct{}{
			*nodeIDs[0]: {},
		},
		scoreByRecv: 1,
	}
	r1d.routeInfos = []*routeInfo1D{
		{
			nodeID:            nodeIDs[1],
			firstNeighborhood: nodeIDs[1],
			scoreBySend:       3,
		},
	}

	r1d.countRecvPacket(nodeIDs[1])
	assert.Equal(t, 2, r1d.neighborhoodInfos[*nodeIDs[1]].scoreByRecv)
	assert.Equal(t, int64(3), r1d.routeInfos[0].scoreBySend)

	r1d.countRecvPacket(nodeIDs[2])
	assert.Equal(t, 2, r1d.neighborhoodInfos[*nodeIDs[1]].scoreByRecv)
	assert.Equal(t, int64(3), r1d.routeInfos[0].scoreBySend)
}

func TestRouting1D_recvRoutingPacket(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(4)
	types.SortNodeIDs(nodeIDs)

	r1d := newRouting1D(&routing1DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: nodeIDs[0],
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			return true, nil
		},
	})
	r1d.backwardNextNodeIDs = []*types.NodeID{nodeIDs[1]}
	r1d.frontwardNextNodeIDs = []*types.NodeID{nodeIDs[1]}
	r1d.neighborhoodInfos[*nodeIDs[1]] = &neighborhoodInfo{
		secondNeighborhoods: map[types.NodeID]struct{}{
			*nodeIDs[0]: {},
		},
	}
	r1d.neighborhoodInfos[*nodeIDs[2]] = &neighborhoodInfo{
		secondNeighborhoods: map[types.NodeID]struct{}{
			*nodeIDs[0]: {},
		},
		oddScore: 1,
	}
	r1d.routeInfos = []*routeInfo1D{
		{
			nodeID:            nodeIDs[1],
			firstNeighborhood: nodeIDs[1],
		},
	}

	r, err := r1d.recvRoutingPacket(nodeIDs[2], &proto.Routing{
		NodeRecords: map[string]*proto.RoutingNodeRecord{
			nodeIDs[0].String(): {
				R1DScore: 3,
			},
			nodeIDs[3].String(): {
				R1DScore: 4,
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, requireUpdateConnections, r)
	assert.Len(t, r1d.neighborhoodInfos, 2)
	info := r1d.neighborhoodInfos[*nodeIDs[2]]
	assert.Equal(t, 3, info.oddScore)
	assert.Len(t, info.secondNeighborhoods, 2)
	assert.Contains(t, info.secondNeighborhoods, *nodeIDs[0], *nodeIDs[3])

	r, err = r1d.recvRoutingPacket(nodeIDs[2], &proto.Routing{
		NodeRecords: map[string]*proto.RoutingNodeRecord{
			nodeIDs[0].String(): {
				R1DScore: 4,
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, requireUpdateConnections, r)
	info = r1d.neighborhoodInfos[*nodeIDs[2]]
	assert.Equal(t, 4, info.oddScore)
	assert.Len(t, info.secondNeighborhoods, 1)
	assert.Contains(t, info.secondNeighborhoods, *nodeIDs[0])
}

func TestRouting1D_setupRoutingPacket(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(4)
	types.SortNodeIDs(nodeIDs)

	r1d := newRouting1D(&routing1DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: nodeIDs[0],
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			return true, nil
		},
	})
	r1d.backwardNextNodeIDs = []*types.NodeID{nodeIDs[1]}
	r1d.frontwardNextNodeIDs = []*types.NodeID{nodeIDs[1]}
	r1d.neighborhoodInfos[*nodeIDs[1]] = &neighborhoodInfo{
		secondNeighborhoods: map[types.NodeID]struct{}{
			*nodeIDs[0]: {},
		},
	}
	r1d.neighborhoodInfos[*nodeIDs[2]] = &neighborhoodInfo{
		secondNeighborhoods: map[types.NodeID]struct{}{
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
	nodeIDs := testUtil.UniqueNodeIDs(20)
	t.Logf("nodeIDs: %s", nodeIDListToString(nodeIDs))

	// Sort dummy nodeIDs
	localNodeID := nodeIDs[0]
	oppositeNodeID := localNodeID.Add(types.NewNormalNodeID(0x8000000000000000, 0x0))
	largerNodeIDs := []*types.NodeID{}
	smallerNodeIDs := []*types.NodeID{}
	frontwardNodeIDs := []*types.NodeID{}
	backwardNodeIDs := []*types.NodeID{}
	for _, nodeID := range nodeIDs[1:] {
		if nodeID.Smaller(localNodeID) {
			smallerNodeIDs = append(smallerNodeIDs, nodeID)
		} else {
			largerNodeIDs = append(largerNodeIDs, nodeID)
		}
		if oppositeNodeID.IsBetween(nodeID, localNodeID) {
			frontwardNodeIDs = append(frontwardNodeIDs, nodeID)
		} else {
			backwardNodeIDs = append(backwardNodeIDs, nodeID)
		}
	}
	types.SortNodeIDs(largerNodeIDs)
	types.SortNodeIDs(smallerNodeIDs)
	sortedNodeIDs := append(largerNodeIDs, smallerNodeIDs...)

	r1d := newRouting1D(&routing1DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: localNodeID,
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			return true, nil
		},
	})
	r1d.frontwardNextNodeIDs = sortedNodeIDs[0:2]
	r1d.backwardNextNodeIDs = sortedNodeIDs[len(sortedNodeIDs)-2:]
	for _, nodeID := range sortedNodeIDs {
		r1d.neighborhoodInfos[*nodeID] = &neighborhoodInfo{
			level: r1d.calcLevel(nodeID),
		}
	}

	required, keep := r1d.getConnections()
	t.Logf("required: %s", nodeIDMapToString(required))
	t.Logf("keep: %s", nodeIDMapToString(keep))
	// next nodes should be included in required connections
	for _, nodeID := range append(sortedNodeIDs[0:2], sortedNodeIDs[len(sortedNodeIDs)-2:]...) {
		assert.Contains(t, required, *nodeID)
	}
	// backward nodes should be included in keep connections
	for _, nodeID := range backwardNodeIDs {
		assert.Contains(t, keep, *nodeID)
	}
	// required nodes should be included in frontward nodes
	for nodeID := range required {
		// skip check backward next nodes
		if nodeID.Equal(sortedNodeIDs[len(sortedNodeIDs)-1]) ||
			nodeID.Equal(sortedNodeIDs[len(sortedNodeIDs)-2]) {
			continue
		}
		assert.Contains(t, frontwardNodeIDs, &nodeID)
	}
}

func nodeIDListToString(nodeIDs []*types.NodeID) string {
	nodeIDStr := ""
	for i, nodeID := range nodeIDs {
		if i != 0 {
			nodeIDStr += ", "
		}
		nodeIDStr += nodeID.String()
	}
	return nodeIDStr
}

func nodeIDMapToString(nodeIDs map[types.NodeID]struct{}) string {
	nodeIDStr := ""
	for nodeID := range nodeIDs {
		if nodeIDStr != "" {
			nodeIDStr += ", "
		}
		nodeIDStr += nodeID.String()
	}
	return nodeIDStr
}

func TestRouting1D_calcLevel(t *testing.T) {
	prev := types.NewNormalNodeID(0, 0)
	for expect, r := range levelRanges {
		nodeID := testUtil.UniqueNodeIDsWithRange(prev, r, 1)[0]
		localNodeID := types.NewRandomNodeID()
		r1d := newRouting1D(&routing1DConfig{
			logger:      testUtil.Logger(t),
			localNodeID: localNodeID,
			reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
				return true, nil
			},
		})
		level := r1d.calcLevel(localNodeID.Add(nodeID))
		assert.Equal(t, expect, level)
		prev = r
	}

	// expect -1 if nodeID is not in the range
	nodeID := testUtil.UniqueNodeIDsWithRange(prev, types.NewNormalNodeID(^uint64(0), ^uint64(0)), 1)[0]
	localNodeID := types.NewRandomNodeID()
	r1d := newRouting1D(&routing1DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: localNodeID,
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			return true, nil
		},
	})
	level := r1d.calcLevel(localNodeID.Add(nodeID))
	assert.Equal(t, -1, level)
}

func TestRouting1D_updateRouteInfos(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(6)
	types.SortNodeIDs(nodeIDs)
	for nodeIDs[3].DistanceFrom(nodeIDs[2]).Smaller(nodeIDs[3].DistanceFrom(nodeIDs[4])) {
		nodeIDs = testUtil.UniqueNodeIDs(6)
		types.SortNodeIDs(nodeIDs)
	}

	tests := []struct {
		connected        map[types.NodeID][]*types.NodeID
		routeInfos       []*routeInfo1D
		expectRouteInfos []*routeInfo1D
	}{
		{
			connected:        map[types.NodeID][]*types.NodeID{},
			routeInfos:       []*routeInfo1D{},
			expectRouteInfos: []*routeInfo1D{},
		},
		{
			connected: map[types.NodeID][]*types.NodeID{},
			routeInfos: []*routeInfo1D{
				{
					nodeID:            nodeIDs[1],
					firstNeighborhood: nodeIDs[2],
					scoreBySend:       1,
				},
			},
			expectRouteInfos: []*routeInfo1D{},
		},
		{ // add
			connected: map[types.NodeID][]*types.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[2]},
				*nodeIDs[2]: {nodeIDs[1], nodeIDs[3]},
			},
			routeInfos: []*routeInfo1D{
				{
					nodeID:            nodeIDs[1],
					firstNeighborhood: nodeIDs[1],
					scoreBySend:       1,
				},
				{
					nodeID:            nodeIDs[2],
					firstNeighborhood: nodeIDs[1],
					scoreBySend:       2,
				},
			},
			expectRouteInfos: []*routeInfo1D{
				{
					nodeID:            nodeIDs[1],
					firstNeighborhood: nodeIDs[1],
					scoreBySend:       1,
				},
				{
					nodeID:            nodeIDs[2],
					firstNeighborhood: nodeIDs[2],
					scoreBySend:       2,
				},
				{
					nodeID:            nodeIDs[3],
					firstNeighborhood: nodeIDs[2],
					scoreBySend:       0,
				},
			},
		},
		{ // remove
			connected: map[types.NodeID][]*types.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[2]},
			},
			routeInfos: []*routeInfo1D{
				{
					nodeID:            nodeIDs[1],
					firstNeighborhood: nodeIDs[1],
					scoreBySend:       1,
				},
				{
					nodeID:            nodeIDs[2],
					firstNeighborhood: nodeIDs[2],
					scoreBySend:       2,
				},
				{
					nodeID:            nodeIDs[3],
					firstNeighborhood: nodeIDs[2],
					scoreBySend:       0,
				},
			},
			expectRouteInfos: []*routeInfo1D{
				{
					nodeID:            nodeIDs[1],
					firstNeighborhood: nodeIDs[1],
					scoreBySend:       1,
				},
				{
					nodeID:            nodeIDs[2],
					firstNeighborhood: nodeIDs[1],
					scoreBySend:       2,
				},
			},
		},
		{ // reroute to nearest node
			connected: map[types.NodeID][]*types.NodeID{
				*nodeIDs[1]: {nodeIDs[0], nodeIDs[2]},
				*nodeIDs[2]: {nodeIDs[1], nodeIDs[3]},
				*nodeIDs[4]: {nodeIDs[3]}},
			routeInfos: []*routeInfo1D{
				{
					nodeID:            nodeIDs[1],
					firstNeighborhood: nodeIDs[1],
					scoreBySend:       1,
				},
				{
					nodeID:            nodeIDs[2],
					firstNeighborhood: nodeIDs[2],
					scoreBySend:       2,
				},
				{
					nodeID:            nodeIDs[3],
					firstNeighborhood: nodeIDs[2],
					scoreBySend:       3,
				},
			},
			expectRouteInfos: []*routeInfo1D{
				{
					nodeID:            nodeIDs[1],
					firstNeighborhood: nodeIDs[1],
					scoreBySend:       1,
				},
				{
					nodeID:            nodeIDs[2],
					firstNeighborhood: nodeIDs[2],
					scoreBySend:       2,
				},
				{
					nodeID:            nodeIDs[3],
					firstNeighborhood: nodeIDs[4],
					scoreBySend:       3,
				},
				{
					nodeID:            nodeIDs[4],
					firstNeighborhood: nodeIDs[4],
					scoreBySend:       0,
				},
			},
		},
	}

	for i, tt := range tests {
		t.Logf("test %d", i)

		r1d := newRouting1D(&routing1DConfig{
			logger:      testUtil.Logger(t),
			localNodeID: nodeIDs[0],
			reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
				return true, nil
			},
		})
		r1d.neighborhoodInfos = make(map[types.NodeID]*neighborhoodInfo)
		for connectedNodeID, connected := range tt.connected {
			c := make(map[types.NodeID]struct{})
			for _, nodeID := range connected {
				c[*nodeID] = struct{}{}
			}
			r1d.neighborhoodInfos[connectedNodeID] = &neighborhoodInfo{
				secondNeighborhoods: c,
			}
		}
		r1d.routeInfos = tt.routeInfos

		r1d.updateRouteInfos()
		require.Len(t, r1d.routeInfos, len(tt.expectRouteInfos))
		for i, routeInfo := range r1d.routeInfos {
			assert.Equal(t, *tt.expectRouteInfos[i].nodeID, *routeInfo.nodeID)
			assert.Equal(t, *tt.expectRouteInfos[i].firstNeighborhood, *routeInfo.firstNeighborhood)
			assert.Equal(t, tt.expectRouteInfos[i].scoreBySend, routeInfo.scoreBySend)
		}
	}
}

func TestRouting1D_updateNextNodeIDs(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(6)
	types.SortNodeIDs(nodeIDs)

	tests := []struct {
		name                 string
		localNodeID          *types.NodeID
		neighborhoodInfos    map[types.NodeID]*neighborhoodInfo
		frontwardNextNodeIDs []*types.NodeID
		backwardNextNodeIDs  []*types.NodeID
	}{
		{
			name:                 "no neighborhoods",
			localNodeID:          nodeIDs[0],
			neighborhoodInfos:    map[types.NodeID]*neighborhoodInfo{},
			frontwardNextNodeIDs: nil,
			backwardNextNodeIDs:  nil,
		},
		{
			name:        "one neighborhood",
			localNodeID: nodeIDs[0],
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {},
			},
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[1]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[1]},
		},
		{
			name:        "normal neighborhoods",
			localNodeID: nodeIDs[2],
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[0]: {
					secondNeighborhoods: map[types.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
				*nodeIDs[1]: {
					secondNeighborhoods: map[types.NodeID]struct{}{
						*nodeIDs[0]: {},
					},
				},
				*nodeIDs[3]: {
					secondNeighborhoods: map[types.NodeID]struct{}{
						*nodeIDs[4]: {},
						*nodeIDs[5]: {},
					},
				},
				*nodeIDs[4]: {
					secondNeighborhoods: map[types.NodeID]struct{}{
						*nodeIDs[3]: {},
					},
				},
			},
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[3], nodeIDs[4]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[1], nodeIDs[0]},
		},
		{
			name:        "frontward next nodes crosses over 0",
			localNodeID: nodeIDs[4],
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[0]: {},
				*nodeIDs[1]: {},
				*nodeIDs[2]: {},
				*nodeIDs[3]: {},
				*nodeIDs[5]: {},
			},
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[5], nodeIDs[0]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[3], nodeIDs[2]},
		},
		{
			name:        "backward next nodes crosses over 0",
			localNodeID: nodeIDs[1],
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[0]: {},
				*nodeIDs[2]: {
					secondNeighborhoods: map[types.NodeID]struct{}{
						*nodeIDs[3]: {},
						*nodeIDs[4]: {},
						*nodeIDs[5]: {},
					},
				},
			},
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[2], nodeIDs[3]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[0], nodeIDs[5]},
		},
	}

	for _, tt := range tests {
		t.Logf("%s", tt.name)

		r1d := newRouting1D(&routing1DConfig{
			logger:      testUtil.Logger(t),
			localNodeID: tt.localNodeID,
			reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
				return true, nil
			},
		})
		r1d.neighborhoodInfos = tt.neighborhoodInfos

		r1d.updateNextNodeIDs()
		assert.Equal(t, tt.frontwardNextNodeIDs, r1d.frontwardNextNodeIDs)
		assert.Equal(t, tt.backwardNextNodeIDs, r1d.backwardNextNodeIDs)
	}
}

func TestRouting1D_updateNextNodeMatched(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(6)
	types.SortNodeIDs(nodeIDs)

	tests := []struct {
		name                       string
		frontwardNextNodeIDs       []*types.NodeID
		backwardNextNodeIDs        []*types.NodeID
		neighborhoodInfos          map[types.NodeID]*neighborhoodInfo
		reconcileNextNodeIDs       []*types.NodeID
		reconcileDisconnected      []*types.NodeID
		reconcileResult            bool
		previousNextNodeIDs        []*types.NodeID
		expectIsStable             bool
		expectBackwardNextNodeIDs  []*types.NodeID
		expectFrontwardNextNodeIDs []*types.NodeID
	}{
		{
			name:                       "no next nodes",
			frontwardNextNodeIDs:       []*types.NodeID{},
			backwardNextNodeIDs:        []*types.NodeID{},
			neighborhoodInfos:          map[types.NodeID]*neighborhoodInfo{},
			reconcileNextNodeIDs:       []*types.NodeID{},
			reconcileDisconnected:      nil,
			reconcileResult:            true,
			previousNextNodeIDs:        []*types.NodeID{},
			expectIsStable:             true,
			expectBackwardNextNodeIDs:  []*types.NodeID{},
			expectFrontwardNextNodeIDs: []*types.NodeID{},
		},
		{
			name:                 "having next nodes",
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[5], nodeIDs[4]},
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {},
				*nodeIDs[2]: {},
				*nodeIDs[3]: {},
				*nodeIDs[4]: {},
				*nodeIDs[5]: {},
			},
			reconcileNextNodeIDs:       []*types.NodeID{nodeIDs[4], nodeIDs[5], nodeIDs[1], nodeIDs[2]},
			reconcileDisconnected:      nil,
			reconcileResult:            true,
			previousNextNodeIDs:        nil,
			expectIsStable:             true,
			expectBackwardNextNodeIDs:  []*types.NodeID{nodeIDs[4], nodeIDs[5]},
			expectFrontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
		},
		{
			name:                 "having next nodes, having previous nodes and no disconnected nodes",
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[5], nodeIDs[4]},
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {},
				*nodeIDs[2]: {},
				*nodeIDs[3]: {},
				*nodeIDs[4]: {},
				*nodeIDs[5]: {},
			},
			reconcileNextNodeIDs:       []*types.NodeID{nodeIDs[4], nodeIDs[5], nodeIDs[1], nodeIDs[2]},
			reconcileDisconnected:      []*types.NodeID{},
			reconcileResult:            true,
			previousNextNodeIDs:        []*types.NodeID{nodeIDs[1], nodeIDs[2], nodeIDs[3], nodeIDs[4]},
			expectIsStable:             true,
			expectBackwardNextNodeIDs:  []*types.NodeID{nodeIDs[4], nodeIDs[5]},
			expectFrontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
		},
		{
			name:                 "having next nodes, having previous nodes and disconnected nodes",
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[5], nodeIDs[4]},
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {},
				*nodeIDs[2]: {},
				*nodeIDs[4]: {},
				*nodeIDs[5]: {},
			},
			reconcileNextNodeIDs:       []*types.NodeID{nodeIDs[4], nodeIDs[5], nodeIDs[1], nodeIDs[2]},
			reconcileDisconnected:      []*types.NodeID{nodeIDs[3]},
			reconcileResult:            true,
			previousNextNodeIDs:        []*types.NodeID{nodeIDs[1], nodeIDs[2], nodeIDs[3], nodeIDs[4]},
			expectIsStable:             true,
			expectBackwardNextNodeIDs:  []*types.NodeID{nodeIDs[4], nodeIDs[5]},
			expectFrontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
		},
		{
			name:                 "should not be stable",
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[5], nodeIDs[4]},
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {},
				*nodeIDs[2]: {},
				*nodeIDs[3]: {},
				*nodeIDs[4]: {},
				*nodeIDs[5]: {},
			},
			reconcileNextNodeIDs:       []*types.NodeID{nodeIDs[4], nodeIDs[5], nodeIDs[1], nodeIDs[2]},
			reconcileDisconnected:      nil,
			reconcileResult:            false,
			previousNextNodeIDs:        nil,
			expectIsStable:             false,
			expectBackwardNextNodeIDs:  []*types.NodeID{nodeIDs[4], nodeIDs[5]},
			expectFrontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
		},
	}

	for _, tt := range tests {
		t.Logf("%s", tt.name)

		r1d := newRouting1D(&routing1DConfig{
			logger:      testUtil.Logger(t),
			localNodeID: nodeIDs[0],
			reconcileNextNodes: func(n, d []*types.NodeID) (bool, error) {
				assert.Equal(t, tt.reconcileNextNodeIDs, n)
				assert.Equal(t, tt.reconcileDisconnected, d)
				return tt.reconcileResult, nil
			},
		})

		r1d.frontwardNextNodeIDs = tt.frontwardNextNodeIDs
		r1d.backwardNextNodeIDs = tt.backwardNextNodeIDs
		r1d.neighborhoodInfos = tt.neighborhoodInfos

		r1d.updateNextNodeMatched(tt.previousNextNodeIDs)

		isStable, backwardNextNodeIDs, frontwardNextNodeIDs := r1d.getStability()
		assert.Equal(t, tt.expectIsStable, isStable)
		assert.Equal(t, tt.expectBackwardNextNodeIDs, backwardNextNodeIDs)
		assert.Equal(t, tt.expectFrontwardNextNodeIDs, frontwardNextNodeIDs)
	}
}

func TestRouting1D_connectedToNextNodes(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(6)
	types.SortNodeIDs(nodeIDs)

	r1d := newRouting1D(&routing1DConfig{
		localNodeID: nodeIDs[0],
		reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
			return true, nil
		},
	})

	tests := []struct {
		name                 string
		neighborhoodInfos    map[types.NodeID]*neighborhoodInfo
		frontwardNextNodeIDs []*types.NodeID
		backwardNextNodeIDs  []*types.NodeID
		expect               bool
	}{
		{
			name:                 "no next nodes",
			neighborhoodInfos:    map[types.NodeID]*neighborhoodInfo{},
			frontwardNextNodeIDs: []*types.NodeID{},
			backwardNextNodeIDs:  []*types.NodeID{},
			expect:               true,
		},
		{
			name: "only one next node",
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {},
			},
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[1]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[1]},
			expect:               true,
		},
		{
			name: "connected to next nodes",
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {},
				*nodeIDs[2]: {},
				*nodeIDs[3]: {},
				*nodeIDs[4]: {},
				*nodeIDs[5]: {},
			},
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[4], nodeIDs[5]},
			expect:               true,
		},
		{
			name: "not connected to next nodes",
			neighborhoodInfos: map[types.NodeID]*neighborhoodInfo{
				*nodeIDs[1]: {},
				*nodeIDs[2]: {},
				*nodeIDs[3]: {},
				*nodeIDs[4]: {},
			},
			frontwardNextNodeIDs: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
			backwardNextNodeIDs:  []*types.NodeID{nodeIDs[4], nodeIDs[5]},
			expect:               false,
		},
	}

	for _, tt := range tests {
		t.Logf("%s", tt.name)

		r1d.neighborhoodInfos = tt.neighborhoodInfos
		r1d.frontwardNextNodeIDs = tt.frontwardNextNodeIDs
		r1d.backwardNextNodeIDs = tt.backwardNextNodeIDs
		r := r1d.connectedToNextNodes()
		assert.Equal(t, tt.expect, r)
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

		r1d := newRouting1D(&routing1DConfig{
			logger: testUtil.Logger(t),
			reconcileNextNodes: func(_, _ []*types.NodeID) (bool, error) {
				return true, nil
			},
		})
		nodeIDs := testUtil.UniqueNodeIDs(len(tt.score))
		for i, score := range tt.score {
			r1d.neighborhoodInfos[*nodeIDs[i]] = &neighborhoodInfo{
				scoreByRecv: score,
			}
			r1d.routeInfos = append(r1d.routeInfos, &routeInfo1D{
				nodeID:      nodeIDs[i],
				scoreBySend: int64(score),
			})
		}
		r1d.normalizeScore()
		for i, nodeID := range nodeIDs {
			assert.Equal(t, tt.expect[i], r1d.neighborhoodInfos[*nodeID].scoreByRecv)
			assert.Equal(t, int64(tt.expect[i]), r1d.routeInfos[i].scoreBySend)
		}
	}
}

func TestRouting1D_isBetween(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	types.SortNodeIDs(nodeIDs)

	tests := []struct {
		a, b, target *types.NodeID
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

		assert.Equal(t, tt.expect, tt.target.IsBetween(tt.a, tt.b))
	}
}
