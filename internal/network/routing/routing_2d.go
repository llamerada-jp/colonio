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
	"log/slog"
	"math"
	"sync"

	"github.com/fogleman/delaunay"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/shared"
	"golang.org/x/exp/maps"
)

type routing2DConfig struct {
	logger      *slog.Logger
	localNodeID *shared.NodeID
	geometry    geometry.CoordinateSystem
}

type routeInfo2D struct {
	position    *geometry.Coordinate
	rootNodeIDs map[shared.NodeID]struct{}
}

type routing2D struct {
	config          *routing2DConfig
	mtx             sync.RWMutex
	localPosition   geometry.Coordinate
	neighborNodeIDs map[shared.NodeID]struct{}
	routeInfos      map[shared.NodeID]*routeInfo2D
}

func newRouting2D(config *routing2DConfig) *routing2D {
	return &routing2D{
		config:          config,
		neighborNodeIDs: make(map[shared.NodeID]struct{}),
		routeInfos:      make(map[shared.NodeID]*routeInfo2D),
	}
}

func (r *routing2D) updateNodeConnections(connections map[shared.NodeID]struct{}) bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	updated := false

	for nodeID, info := range r.routeInfos {
		for rootNodeIDs := range info.rootNodeIDs {
			if _, ok := connections[rootNodeIDs]; !ok {
				delete(info.rootNodeIDs, rootNodeIDs)
			}
			if len(info.rootNodeIDs) == 0 {
				delete(r.routeInfos, nodeID)
				updated = true
			}
		}
	}

	if !updated {
		return false
	}

	return r.neighborNodeIDChanged()
}

func (r *routing2D) getNextStep(dst *geometry.Coordinate) *shared.NodeID {
	candidateNodeID := &shared.NodeIDThis
	var candidateInfo *routeInfo2D
	min := r.config.geometry.GetDistance(&r.localPosition, dst)

	r.mtx.RLock()
	defer r.mtx.RUnlock()

	for nodeID, info := range r.routeInfos {
		nodeID := nodeID
		distance := r.config.geometry.GetDistance(info.position, dst)
		if distance < min {
			min = distance
			candidateNodeID = &nodeID
			candidateInfo = info
		}
	}

	// return immediately if the candidate is the local node
	if candidateInfo == nil {
		return candidateNodeID
	}

	// return immediately if the candidate is connected directly
	if _, ok := candidateInfo.rootNodeIDs[*candidateNodeID]; ok {
		return candidateNodeID
	}

	// search the nearest node connected to the candidate from candidate info
	min = math.MaxFloat64
	for nodeID := range candidateInfo.rootNodeIDs {
		nodeID := nodeID
		info := r.routeInfos[nodeID]
		distance := r.config.geometry.GetDistance(info.position, dst)
		if distance < min {
			min = distance
			candidateNodeID = &nodeID
		}
	}
	return candidateNodeID
}

func (r *routing2D) getNextNodePositions() map[shared.NodeID]*geometry.Coordinate {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	positions := make(map[shared.NodeID]*geometry.Coordinate)
	for nodeID, info := range r.routeInfos {
		if info.position == nil {
			continue
		}
		if _, ok := info.rootNodeIDs[nodeID]; !ok {
			continue
		}
		positions[nodeID] = info.position
	}

	return positions
}

func (r *routing2D) updateLocalPosition(pos *geometry.Coordinate) bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if r.localPosition == *pos {
		return false
	}

	r.localPosition = *pos
	r.neighborNodeIDChanged()

	return true
}

func (r *routing2D) recvRoutingPacket(src *shared.NodeID, content *proto.Routing) bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	updated := false
	contentPosition := geometry.NewCoordinateFromProto(content.GetR2DPosition())

	if info, ok := r.routeInfos[*src]; !ok {
		updated = true
		r.routeInfos[*src] = &routeInfo2D{
			position: contentPosition,
			rootNodeIDs: map[shared.NodeID]struct{}{
				*src: {},
			},
		}
	} else if _, ok := info.rootNodeIDs[*src]; !ok || !info.position.Equal(contentPosition) {
		updated = true
		info.position = geometry.NewCoordinateFromProto(content.GetR2DPosition())
		info.rootNodeIDs[*src] = struct{}{}
	}

	for nodeIDStr, record := range content.GetNodeRecords() {
		nodeID := shared.NewNodeIDFromString(nodeIDStr)
		if nodeID.Equal(r.config.localNodeID) {
			continue
		}
		if info, ok := r.routeInfos[*nodeID]; !ok {
			if record.GetR2DPosition() == nil {
				continue
			}
			nodePosition := geometry.NewCoordinateFromProto(record.GetR2DPosition())
			updated = true
			r.routeInfos[*nodeID] = &routeInfo2D{
				position: nodePosition,
				rootNodeIDs: map[shared.NodeID]struct{}{
					*src: {},
				},
			}
		} else {
			if _, ok := info.rootNodeIDs[*src]; !ok {
				updated = true
				info.rootNodeIDs[*src] = struct{}{}
			}
			if record.GetR2DPosition() == nil {
				continue
			}
			nodePosition := geometry.NewCoordinateFromProto(record.GetR2DPosition())
			// update position if the node is not connected directly
			if _, ok := info.rootNodeIDs[*nodeID]; !ok && !info.position.Equal(nodePosition) {
				updated = true
				info.position = nodePosition
			}
		}
	}

	for nodeID, info := range r.routeInfos {
		if nodeID.Equal(src) {
			continue
		}
		if _, ok := info.rootNodeIDs[*src]; !ok {
			continue
		}
		if _, ok := content.GetNodeRecords()[nodeID.String()]; !ok {
			updated = true
			delete(info.rootNodeIDs, *src)
			if len(info.rootNodeIDs) == 0 {
				delete(r.routeInfos, nodeID)
			}
		}
	}

	if !updated {
		return false
	}

	return r.neighborNodeIDChanged()
}

func (r *routing2D) setupRoutingPacket(content *proto.Routing) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	content.R2DPosition = r.localPosition.Proto()

	for nodeID, info := range r.routeInfos {
		// skip if the node is not connected directly
		if _, ok := info.rootNodeIDs[nodeID]; !ok {
			continue
		}
		nodeIDStr := nodeID.String()
		if nodeRecord, ok := content.NodeRecords[nodeIDStr]; !ok {
			content.NodeRecords[nodeIDStr] = &proto.RoutingNodeRecord{
				R2DPosition: info.position.Proto(),
			}
		} else {
			nodeRecord.R2DPosition = info.position.Proto()
		}
	}
}

func (r *routing2D) getConnections() map[shared.NodeID]struct{} {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	return r.neighborNodeIDs
}

// private
func (r *routing2D) neighborNodeIDChanged() bool {
	neighborNodeIDs := make(map[shared.NodeID]struct{})

	if len(r.routeInfos) < 3 {
		for nodeID := range r.routeInfos {
			neighborNodeIDs[nodeID] = struct{}{}
		}

	} else {
		points := make(map[delaunay.Point][]*shared.NodeID)
		points[delaunay.Point{
			X: r.localPosition.X,
			Y: r.localPosition.Y,
		}] = []*shared.NodeID{r.config.localNodeID}
		for nodeID, info := range r.routeInfos {
			point := delaunay.Point{
				X: info.position.X,
				Y: info.position.Y,
			}
			points[point] = append(points[point], &nodeID)
		}

		// shift the node position if the same position is found
		precision := r.config.geometry.GetPrecision()
		for point, nodeIDs := range points {
			if len(nodeIDs) == 1 {
				continue
			}
			delete(points, point)
			for _, nodeID := range nodeIDs {
				id0, id1 := nodeID.Raw()
				newPosition := delaunay.Point{
					X: point.X + precision*float64(id0)/math.MaxUint64,
					Y: point.Y + precision*float64(id1)/math.MaxUint64,
				}
				points[newPosition] = []*shared.NodeID{nodeID}
			}
		}

		pointSlice := make([]delaunay.Point, 0, len(points))
		for point := range points {
			pointSlice = append(pointSlice, point)
		}
		triangulation, err := delaunay.Triangulate(pointSlice)
		if err != nil {
			r.config.logger.Error("failed to triangulate", slog.String("err", err.Error()))
			return false
		}

		for i, h := range triangulation.Halfedges {
			if h <= i {
				continue
			}
			p1 := &pointSlice[triangulation.Triangles[i]]
			j := i + 1
			if i%3 == 2 {
				j = i - 2
			}
			p2 := &pointSlice[triangulation.Triangles[j]]

			n1 := p2n(points, p1)
			n2 := p2n(points, p2)

			if n1 == nil || n2 == nil {
				continue
			}
			if *n1 == *r.config.localNodeID {
				neighborNodeIDs[*n2] = struct{}{}
			}
			if *n2 == *r.config.localNodeID {
				neighborNodeIDs[*n1] = struct{}{}
			}
		}
	}

	if !maps.Equal(neighborNodeIDs, r.neighborNodeIDs) {
		r.neighborNodeIDs = neighborNodeIDs
		return true
	}

	return false
}

func p2n(points map[delaunay.Point][]*shared.NodeID, p *delaunay.Point) *shared.NodeID {
	if nodeIDs, ok := points[*p]; ok {
		return nodeIDs[0]
	}
	return nil
}
