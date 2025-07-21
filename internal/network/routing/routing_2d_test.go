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
	"math/rand"
	"testing"

	"github.com/fogleman/delaunay"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/shared"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouting2D_updateNodeConnections(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)

	tests := []struct {
		routeInfos  map[shared.NodeID]*routeInfo2D
		connections map[shared.NodeID]struct{}
		expect      map[shared.NodeID]*routeInfo2D
	}{
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{},
			connections: map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
			},
			expect: map[shared.NodeID]*routeInfo2D{},
		},
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[0]: {
					position: getRandomPosition(),
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[0]: {},
						*nodeIDs[1]: {},
					},
				},
			},
			connections: map[shared.NodeID]struct{}{},
			expect:      map[shared.NodeID]*routeInfo2D{},
		},
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[0]: {
					position: getRandomPosition(),
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[0]: {},
						*nodeIDs[1]: {},
					},
				},
				*nodeIDs[1]: {
					position: getRandomPosition(),
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
			},
			connections: map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
			},
			expect: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[0]: {
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[0]: {},
					},
				},
			},
		},
	}

	r := newRouting2D(&routing2DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: shared.NewRandomNodeID(),
		geometry:    geometry.NewPlaneCoordinateSystem(-100, 100, -100, 100),
	})

	for i, tt := range tests {
		t.Logf("test %d", i)

		r.routeInfos = tt.routeInfos
		r.updateNodeConnections(tt.connections)
		assert.Len(t, r.routeInfos, len(tt.expect))
		for nodeID, expect := range tt.expect {
			assert.Contains(t, r.routeInfos, nodeID)
			info := r.routeInfos[nodeID]
			assert.Equal(t, expect.rootNodeIDs, info.rootNodeIDs)
		}
	}
}

func TestRouting2D_getNextStep(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)

	tests := []struct {
		routeInfos map[shared.NodeID]*routeInfo2D
		dst        *geometry.Coordinate
		expect     *shared.NodeID
	}{
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{},
			dst:        getRandomPosition(),
			expect:     &shared.NodeLocal,
		},
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[0]: {
					position: &geometry.Coordinate{X: 10, Y: 10},
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[0]: {},
					},
				},
			},
			dst:    &geometry.Coordinate{X: -1, Y: -1},
			expect: &shared.NodeLocal,
		},
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[0]: {
					position: &geometry.Coordinate{X: 10, Y: 10},
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[0]: {},
					},
				},
				*nodeIDs[1]: {
					position: &geometry.Coordinate{X: -10, Y: 10},
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
			},
			dst:    &geometry.Coordinate{X: -9, Y: 9},
			expect: nodeIDs[1],
		},
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[0]: {
					position: &geometry.Coordinate{X: 10, Y: 10},
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
						*nodeIDs[2]: {},
					},
				},
				*nodeIDs[1]: {
					position: &geometry.Coordinate{X: 8, Y: 8},
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
				*nodeIDs[2]: {
					position: &geometry.Coordinate{X: 15, Y: 15},
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[2]: {},
					},
				},
			},
			dst:    &geometry.Coordinate{X: 11, Y: 11},
			expect: nodeIDs[1],
		},
	}

	r := newRouting2D(&routing2DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: shared.NewRandomNodeID(),
		geometry:    geometry.NewPlaneCoordinateSystem(-100, 100, -100, 100),
	})
	r.updateLocalPosition(&geometry.Coordinate{
		X: 0,
		Y: 0,
	})

	for i, tt := range tests {
		t.Logf("test %d", i)

		r.routeInfos = tt.routeInfos
		assert.Equal(t, *tt.expect, *r.getNextStep(tt.dst))
	}
}

func TestRouting2D_recvRoutingPacket(t *testing.T) {
	planeGeometry := geometry.NewPlaneCoordinateSystem(-100, 100, -100, 100)

	nodeIDs := testUtil.UniqueNodeIDs(4)
	positions := getUniquePositions(4, planeGeometry)
	updatedPositions := getUniquePositions(4, planeGeometry)

	tt := []struct {
		routeInfos map[shared.NodeID]*routeInfo2D
		src        *shared.NodeID
		content    *proto.Routing
		expect     map[shared.NodeID]*routeInfo2D
	}{
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{},
			src:        nodeIDs[1],
			content: &proto.Routing{
				R2DPosition: positions[1].Proto(),
				NodeRecords: map[string]*proto.RoutingNodeRecord{
					nodeIDs[2].String(): {
						R2DPosition: positions[2].Proto(),
					},
				},
			},
			expect: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[1]: {
					position: positions[1],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
				*nodeIDs[2]: {
					position: positions[2],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
			},
		},
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[2]: {
					position: positions[2],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
						*nodeIDs[2]: {},
					},
				},
				*nodeIDs[3]: {
					position: positions[3],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
			},
			src: nodeIDs[1],
			content: &proto.Routing{
				R2DPosition: positions[1].Proto(),
				NodeRecords: map[string]*proto.RoutingNodeRecord{
					nodeIDs[2].String(): {
						R2DPosition: positions[2].Proto(),
					},
				},
			},
			expect: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[1]: {
					position: positions[1],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
				*nodeIDs[2]: {
					position: positions[2],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
						*nodeIDs[2]: {},
					},
				},
			},
		},
		{
			routeInfos: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[2]: {
					position: positions[2],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
						*nodeIDs[2]: {},
					},
				},
				*nodeIDs[3]: {
					position: positions[3],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
						*nodeIDs[3]: {},
					},
				},
			},
			src: nodeIDs[1],
			content: &proto.Routing{
				R2DPosition: updatedPositions[1].Proto(),
				NodeRecords: map[string]*proto.RoutingNodeRecord{
					nodeIDs[2].String(): {
						R2DPosition: updatedPositions[2].Proto(),
					},
					nodeIDs[3].String(): {
						R2DPosition: updatedPositions[3].Proto(),
					},
				},
			},
			expect: map[shared.NodeID]*routeInfo2D{
				*nodeIDs[1]: {
					position: updatedPositions[1],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
					},
				},
				*nodeIDs[2]: {
					position: updatedPositions[2],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
						*nodeIDs[2]: {},
					},
				},
				*nodeIDs[3]: {
					position: positions[3],
					rootNodeIDs: map[shared.NodeID]struct{}{
						*nodeIDs[1]: {},
						*nodeIDs[3]: {},
					},
				},
			},
		},
	}

	r := newRouting2D(&routing2DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: nodeIDs[0],
		geometry:    planeGeometry,
	})
	r.updateLocalPosition(positions[0])

	for i, tt := range tt {
		t.Logf("test %d", i)

		r.routeInfos = tt.routeInfos
		r.recvRoutingPacket(tt.src, tt.content)
		require.Len(t, r.routeInfos, len(tt.expect))
		for nodeID, expect := range tt.expect {
			assert.Contains(t, r.routeInfos, nodeID)
			info := r.routeInfos[nodeID]
			assert.Equal(t, expect.rootNodeIDs, info.rootNodeIDs)
		}
	}
}

func TestRouting2D_setupRoutingPacket(t *testing.T) {
	planeGeometry := geometry.NewPlaneCoordinateSystem(-100, 100, -100, 100)

	nodeIDs := testUtil.UniqueNodeIDs(5)
	positions := getUniquePositions(5, planeGeometry)

	r := newRouting2D(&routing2DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: nodeIDs[3],
		geometry:    planeGeometry,
	})
	r.updateLocalPosition(positions[3])
	r.routeInfos = map[shared.NodeID]*routeInfo2D{
		*nodeIDs[0]: {
			position: positions[0],
			rootNodeIDs: map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
			},
		},
		*nodeIDs[1]: {
			position: positions[1],
			rootNodeIDs: map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
				*nodeIDs[1]: {},
			},
		},
		*nodeIDs[2]: {
			position: positions[2],
			rootNodeIDs: map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
				*nodeIDs[1]: {},
			},
		},
	}

	content := &proto.Routing{
		NodeRecords: map[string]*proto.RoutingNodeRecord{
			nodeIDs[0].String(): {
				R1DScore: 10,
			},
			nodeIDs[4].String(): {
				R1DScore: 1,
			},
		},
	}
	r.setupRoutingPacket(content)

	assert.InDelta(t, positions[3].X, content.GetR2DPosition().X, 0.01)
	assert.InDelta(t, positions[3].Y, content.GetR2DPosition().Y, 0.01)

	require.Len(t, content.GetNodeRecords(), 3)
	for _, nodeID := range []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[4]} {
		record := content.GetNodeRecords()[nodeID.String()]
		if nodeID.Equal(nodeIDs[0]) {
			assert.Equal(t, int64(10), record.R1DScore)
		} else if nodeID.Equal(nodeIDs[4]) {
			assert.Equal(t, int64(1), record.R1DScore)
		}

		if nodeID.Equal(nodeIDs[0]) || nodeID.Equal(nodeIDs[1]) {
			assert.InDelta(t, r.routeInfos[*nodeID].position.X, record.GetR2DPosition().X, 0.01)
			assert.InDelta(t, r.routeInfos[*nodeID].position.Y, record.GetR2DPosition().Y, 0.01)
		}
	}
}

func TestRouting2D_neighborNodeIDChanged(t *testing.T) {
	planeGeometry := geometry.NewPlaneCoordinateSystem(-100, 100, -100, 100)

	nodeIDs := testUtil.UniqueNodeIDs(100)
	positions := getUniquePositions(100, planeGeometry)
	points := make([]delaunay.Point, 100)
	pointsMap := make(map[delaunay.Point]*shared.NodeID)
	for i := 0; i < 100; i++ {
		points[i] = delaunay.Point{
			X: positions[i].X,
			Y: positions[i].Y,
		}
		pointsMap[points[i]] = nodeIDs[i]
	}

	r := newRouting2D(&routing2DConfig{
		logger:      testUtil.Logger(t),
		localNodeID: nodeIDs[0],
		geometry:    planeGeometry,
	})

	r.updateLocalPosition(positions[0])
	r.routeInfos = make(map[shared.NodeID]*routeInfo2D)
	for i := 1; i < 100; i++ {
		r.routeInfos[*nodeIDs[i]] = &routeInfo2D{
			position: positions[i],
		}
	}
	assert.True(t, r.neighborNodeIDChanged())

	expect := make(map[shared.NodeID]struct{})
	triangulation, err := delaunay.Triangulate(points)

	require.NoError(t, err)
	ts := triangulation.Triangles
	hs := triangulation.Halfedges
	for i, h := range hs {
		if i > h {
			p := points[ts[i]]
			q := points[ts[nextHalfEdge(i)]]
			if nodeID := pointsMap[p]; nodeID.Equal(nodeIDs[0]) {
				expect[*pointsMap[q]] = struct{}{}
			}
			if nodeID := pointsMap[q]; nodeID.Equal(nodeIDs[0]) {
				expect[*pointsMap[p]] = struct{}{}
			}
		}
	}

	assert.Equal(t, expect, r.neighborNodeIDs)
}

func getUniquePositions(n int, g geometry.CoordinateSystem) []*geometry.Coordinate {
	positions := make([]*geometry.Coordinate, n)
	for i := 0; i < n; i++ {
		for {
			positions[i] = getRandomPosition()
			equal := false
			for j := 0; j < i; j++ {
				if g.GetDistance(positions[i], positions[j]) < g.GetPrecision() {
					equal = true
					break
				}
			}
			if !equal {
				break
			}
		}
	}
	return positions
}

func getRandomPosition() *geometry.Coordinate {
	return &geometry.Coordinate{
		X: rand.Float64()*200 - 100,
		Y: rand.Float64()*200 - 100,
	}
}

func nextHalfEdge(e int) int {
	if e%3 == 2 {
		return e - 2
	}
	return e + 1
}
