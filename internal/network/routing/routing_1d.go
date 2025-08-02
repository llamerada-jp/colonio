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
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/shared"
)

const (
	linksMin             = 4
	linkKeepLinkDuration = 1 * time.Minute
)

var levelRanges []*shared.NodeID = []*shared.NodeID{
	shared.NewNormalNodeID(0x0001FFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF), // 0
	shared.NewNormalNodeID(0x0007FFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF), // 1
	shared.NewNormalNodeID(0x001FFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF), // 2
	shared.NewNormalNodeID(0x007FFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF), // 3
	shared.NewNormalNodeID(0x01FFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF), // 4
	shared.NewNormalNodeID(0x07FFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF), // 5
	shared.NewNormalNodeID(0x1FFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF), // 6
	shared.NewNormalNodeID(0x7FFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF), // 7
}
var levelRangesCount int = len(levelRanges)

type routing1DConfig struct {
	logger             *slog.Logger
	localNodeID        *shared.NodeID
	reconcileNextNodes func(nextNodeIDs, disconnectedNodeIDs []*shared.NodeID) (bool, error)
}

type neighborhoodInfo struct {
	timestamp time.Time
	level     int
	// it is the first neighborhood of the node of the first neighborhood
	// it is strictly different from the second neighborhood of the graph logic
	// this set isn't containing the local node ID
	secondNeighborhoods map[shared.NodeID]struct{}
	oddScore            int
	scoreByRecv         int
}

type routeInfo1D struct {
	nodeID            *shared.NodeID
	firstNeighborhood *shared.NodeID
	level             int
	scoreBySend       int
}

type routing1D struct {
	config          *routing1DConfig
	mtx             sync.RWMutex
	nextNodeMatched bool
	// there is some overlap between frontward and backward.
	frontwardNextNodeIDs []*shared.NodeID
	backwardNextNodeIDs  []*shared.NodeID
	neighborhoodInfos    map[shared.NodeID]*neighborhoodInfo
	routeInfos           []*routeInfo1D
}

func newRouting1D(config *routing1DConfig) *routing1D {
	return &routing1D{
		config:            config,
		neighborhoodInfos: make(map[shared.NodeID]*neighborhoodInfo),
		routeInfos:        make([]*routeInfo1D, 0),
	}
}

func (r *routing1D) subRoutine() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if !r.nextNodeMatched {
		r.updateNextNodeMatched(nil)
	}
}

func (r *routing1D) getStability() (bool, []*shared.NodeID) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	nextNodeIDs := slices.Clone(r.backwardNextNodeIDs)
	slices.Reverse(nextNodeIDs)
	nextNodeIDs = append(nextNodeIDs, r.frontwardNextNodeIDs...)
	return r.nextNodeMatched, nextNodeIDs
}

func (r *routing1D) updateNodeConnections(connections map[shared.NodeID]struct{}) int {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	updated := false

	for nodeID := range connections {
		if _, ok := r.neighborhoodInfos[nodeID]; !ok {
			r.neighborhoodInfos[nodeID] = &neighborhoodInfo{
				timestamp:           time.Now(),
				level:               r.calcLevel(&nodeID),
				secondNeighborhoods: make(map[shared.NodeID]struct{}),
			}
			updated = true
		}
	}

	for nodeID := range r.neighborhoodInfos {
		if _, ok := connections[nodeID]; !ok {
			delete(r.neighborhoodInfos, nodeID)
			updated = true
		}
	}

	if !updated {
		return 0
	}
	r.updateRouteInfos()
	r.updateNextNodeIDs()
	if r.connectedToNextNodes() {
		return requireExchangeRouting
	} else {
		return requireExchangeRouting | requireUpdateConnections
	}
}

func (r *routing1D) getNextStep(packet *shared.Packet) *shared.NodeID {
	isExplicit := (packet.Mode & shared.PacketModeExplicit) != 0

	if packet.DstNodeID.Equal(&shared.NodeLocal) ||
		packet.DstNodeID.Equal(r.config.localNodeID) {
		return &shared.NodeLocal
	}

	if packet.DstNodeID.Equal(&shared.NodeNeighborhoods) {
		return &shared.NodeNeighborhoods
	}

	r.mtx.RLock()
	defer r.mtx.RUnlock()

	// there is no connected node
	if len(r.frontwardNextNodeIDs)+len(r.backwardNextNodeIDs) == 0 {
		if isExplicit {
			return nil
		}
		return &shared.NodeLocal
	}

	if isBetween(r.config.localNodeID, r.frontwardNextNodeIDs[0], packet.DstNodeID) {
		if isExplicit {
			return nil
		}
		return &shared.NodeLocal
	}

	if isBetween(r.backwardNextNodeIDs[0], r.config.localNodeID, packet.DstNodeID) {
		if isExplicit && !r.backwardNextNodeIDs[0].Equal(packet.DstNodeID) {
			return nil
		}
	}

	last := r.routeInfos[len(r.routeInfos)-1]
	if !packet.DstNodeID.Smaller(last.nodeID) {
		return last.firstNeighborhood
	}
	// TODO: binary search is better
	candidate := last
	for _, info := range r.routeInfos {
		if packet.DstNodeID.Smaller(info.nodeID) {
			candidate.scoreBySend += 1
			return candidate.firstNeighborhood
		}
		candidate = info
	}

	panic("next step node should be found in routing infos")
}

func (r *routing1D) countRecvPacket(from *shared.NodeID) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if info, ok := r.neighborhoodInfos[*from]; ok {
		info.scoreByRecv += 1
	}
}

func (r *routing1D) recvRoutingPacket(src *shared.NodeID, content *proto.Routing) (int, error) {
	if r.config.localNodeID.Equal(src) {
		panic("it is not allowed to receive packet from local node")
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	info, ok := r.neighborhoodInfos[*src]
	if !ok {
		return 0, nil
	}

	updated := false
	for nodeID := range info.secondNeighborhoods {
		if _, ok := content.NodeRecords[nodeID.String()]; !ok {
			delete(info.secondNeighborhoods, nodeID)
			updated = true
		}
	}

	oddScore := 0
	for nodeIDStr, record := range content.NodeRecords {
		nodeID, err := shared.NewNodeIDFromString(nodeIDStr)
		if err != nil {
			return 0, fmt.Errorf("failed to parse node id %w", err)
		}
		if nodeID.Equal(r.config.localNodeID) {
			oddScore = int(record.R1DScore)
			continue
		}
		if _, ok := info.secondNeighborhoods[*nodeID]; !ok {
			info.secondNeighborhoods[*nodeID] = struct{}{}
			updated = true
		}
	}
	info.oddScore = oddScore

	if !updated {
		return 0, nil
	}
	r.updateRouteInfos()
	r.updateNextNodeIDs()
	if r.connectedToNextNodes() {
		return 0, nil
	} else {
		return requireUpdateConnections, nil
	}
}

func (r *routing1D) setupRoutingPacket(content *proto.Routing) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.normalizeScore()

	for nodeID, info := range r.neighborhoodInfos {
		nodeIDStr := nodeID.String()
		if nodeRecord, ok := content.NodeRecords[nodeIDStr]; !ok {
			content.NodeRecords[nodeIDStr] = &proto.RoutingNodeRecord{
				R1DScore: int64(info.scoreByRecv),
			}
		} else {
			nodeRecord.R1DScore = int64(info.scoreByRecv)
		}
	}
}

func (r *routing1D) getConnections() (map[shared.NodeID]struct{}, map[shared.NodeID]struct{}) {
	r.mtx.Lock()
	r.normalizeScore()
	r.mtx.Unlock()

	now := time.Now()
	required := make(map[shared.NodeID]struct{})
	keep := make(map[shared.NodeID]struct{})
	requiredLevelCount := make(map[int]int)
	candidatesByLevel := make(map[int]map[shared.NodeID]int)
	for lv := 0; lv < levelRangesCount; lv++ {
		candidatesByLevel[lv] = make(map[shared.NodeID]int)
	}

	r.mtx.RLock()
	defer r.mtx.RUnlock()
	for _, nodeID := range r.frontwardNextNodeIDs {
		required[*nodeID] = struct{}{}
	}
	for _, nodeID := range r.backwardNextNodeIDs {
		required[*nodeID] = struct{}{}
	}

	for neighborhoodNodeID, neighborhoodInfo := range r.neighborhoodInfos {
		// keep connections when the connection level is over the levelRangesCount
		if neighborhoodInfo.level == -1 {
			keep[neighborhoodNodeID] = struct{}{}
			continue
		}

		// keep connections when the number of connections is less than linksMin or the connection is yang
		if len(r.neighborhoodInfos) < linksMin ||
			len(neighborhoodInfo.secondNeighborhoods) < linksMin ||
			now.Before(neighborhoodInfo.timestamp.Add(linkKeepLinkDuration)) {
			required[neighborhoodNodeID] = struct{}{}
			requiredLevelCount[neighborhoodInfo.level]++
			continue
		}

		// keep connections when the nearest node from the connected node is this node
		largerNodeIDs := []*shared.NodeID{r.config.localNodeID}
		smallerNodeIDs := []*shared.NodeID{r.config.localNodeID}
		for nodeID := range neighborhoodInfo.secondNeighborhoods {
			if nodeID.Smaller(&neighborhoodNodeID) {
				smallerNodeIDs = append(smallerNodeIDs, &nodeID)
			} else {
				largerNodeIDs = append(largerNodeIDs, &nodeID)
			}
		}
		shared.SortNodeIDs(largerNodeIDs)
		shared.SortNodeIDs(smallerNodeIDs)
		if largerNodeIDs[0].Equal(r.config.localNodeID) ||
			smallerNodeIDs[len(smallerNodeIDs)-1].Equal(r.config.localNodeID) {
			required[neighborhoodNodeID] = struct{}{}
			requiredLevelCount[neighborhoodInfo.level]++
			continue
		}

		candidatesByLevel[neighborhoodInfo.level][neighborhoodNodeID] = neighborhoodInfo.scoreByRecv
	}

	for _, routeInfo := range r.routeInfos {
		if routeInfo.level == -1 {
			continue
		}
		if _, ok := required[*routeInfo.nodeID]; ok {
			continue
		}
		if _, ok := r.neighborhoodInfos[*routeInfo.nodeID]; ok {
			continue
		}

		candidatesByLevel[routeInfo.level][*routeInfo.nodeID] = routeInfo.scoreBySend
	}

	for level := 0; level < levelRangesCount; level++ {
		if requiredLevelCount[level] >= 2 {
			continue
		}

		for i := 0; i < 2-requiredLevelCount[level]; i++ {
			candidates := candidatesByLevel[level]
			if len(candidates) == 0 {
				break
			}

			maxScore := -1
			var maxNodeID *shared.NodeID
			for nodeID, score := range candidates {
				if maxScore < score {
					maxScore = score
					maxNodeID = &nodeID
				}
			}
			required[*maxNodeID] = struct{}{}
			delete(candidates, *maxNodeID)
		}
	}

	return required, keep
}

// private
func (r *routing1D) calcLevel(nodeID *shared.NodeID) int {
	sub := nodeID.Sub(r.config.localNodeID)
	for idx, l := range levelRanges {
		if sub.Smaller(l) {
			return idx
		}
	}
	return -1
}

func (r *routing1D) updateRouteInfos() {
	idMap := make(map[shared.NodeID]*shared.NodeID)

	for firstNeighborhood, neighborhoodInfo := range r.neighborhoodInfos {
		firstNeighborhood := firstNeighborhood
		idMap[firstNeighborhood] = &firstNeighborhood
		for secondNeighborhood := range neighborhoodInfo.secondNeighborhoods {
			if r.config.localNodeID.Equal(&secondNeighborhood) {
				continue
			}
			if _, ok := idMap[secondNeighborhood]; !ok {
				idMap[secondNeighborhood] = &firstNeighborhood
				continue
			}
			if idMap[secondNeighborhood].Equal(&secondNeighborhood) {
				continue
			}
			distance1 := idMap[secondNeighborhood].DistanceFrom(&secondNeighborhood)
			distance2 := firstNeighborhood.DistanceFrom(&secondNeighborhood)
			if distance2.Smaller(distance1) {
				idMap[secondNeighborhood] = &firstNeighborhood
			}
		}
	}

	r.routeInfos = slices.DeleteFunc(r.routeInfos, func(rInfo *routeInfo1D) bool {
		if _, ok := idMap[*rInfo.nodeID]; ok {
			return false
		}
		return true
	})

	for _, rInfo := range r.routeInfos {
		nodeID := idMap[*rInfo.nodeID]
		rInfo.firstNeighborhood = nodeID
		delete(idMap, *rInfo.nodeID)
	}

	for nodeID, connectedNodeID := range idMap {
		nodeID := nodeID
		r.routeInfos = append(r.routeInfos, &routeInfo1D{
			nodeID:            &nodeID,
			firstNeighborhood: connectedNodeID,
			level:             r.calcLevel(&nodeID),
			scoreBySend:       0,
		})
	}

	slices.SortFunc(r.routeInfos, func(a, b *routeInfo1D) int {
		return a.nodeID.Compare(b.nodeID)
	})
}

// updateNextNodeIDs updates frontward and backward next node IDs.
func (r *routing1D) updateNextNodeIDs() {
	previousNextNodeIDs := append(r.backwardNextNodeIDs, r.frontwardNextNodeIDs...)
	defer r.updateNextNodeMatched(previousNextNodeIDs)

	// not connected any node
	if len(r.neighborhoodInfos) == 0 {
		r.frontwardNextNodeIDs = nil
		r.backwardNextNodeIDs = nil
		return
	}

	knownNodeIDs := make(map[shared.NodeID]struct{})
	for nodeID, info := range r.neighborhoodInfos {
		knownNodeIDs[nodeID] = struct{}{}
		for nodeID2 := range info.secondNeighborhoods {
			knownNodeIDs[nodeID2] = struct{}{}
		}
	}

	if len(knownNodeIDs) == 1 {
		for nodeID := range knownNodeIDs {
			r.frontwardNextNodeIDs = []*shared.NodeID{&nodeID}
			r.backwardNextNodeIDs = []*shared.NodeID{&nodeID}
		}
		return
	}

	largerNodeIDs := make([]*shared.NodeID, 0)
	smallerNodeIDs := make([]*shared.NodeID, 0)
	for nodeID := range knownNodeIDs {
		if nodeID.Smaller(r.config.localNodeID) {
			smallerNodeIDs = append(smallerNodeIDs, &nodeID)
		} else {
			largerNodeIDs = append(largerNodeIDs, &nodeID)
		}
	}
	shared.SortNodeIDs(largerNodeIDs)
	shared.SortNodeIDs(smallerNodeIDs)

	nodeIDs := append(largerNodeIDs, smallerNodeIDs...)
	if len(nodeIDs) >= constants.ONE_SIDE_NEXT_COUNT*2 {
		r.frontwardNextNodeIDs = nodeIDs[0:constants.ONE_SIDE_NEXT_COUNT]
		r.backwardNextNodeIDs = nodeIDs[len(nodeIDs)-constants.ONE_SIDE_NEXT_COUNT:]
	} else {
		r.frontwardNextNodeIDs = nodeIDs[0 : len(nodeIDs)/2]
		r.backwardNextNodeIDs = nodeIDs[len(nodeIDs)/2:]
	}
	slices.Reverse(r.backwardNextNodeIDs)
}

func (r *routing1D) updateNextNodeMatched(previousNextNodeIDs []*shared.NodeID) {
	nextNodeIDs := slices.Clone(r.backwardNextNodeIDs)
	slices.Reverse(nextNodeIDs)
	nextNodeIDs = append(nextNodeIDs, r.frontwardNextNodeIDs...)
	var disconnectedNodeIDs []*shared.NodeID
	if len(previousNextNodeIDs) != 0 {
		disconnectedNodeIDs = make([]*shared.NodeID, 0)
		for _, oldNodeID := range previousNextNodeIDs {
			if _, ok := r.neighborhoodInfos[*oldNodeID]; !ok {
				disconnectedNodeIDs = append(disconnectedNodeIDs, oldNodeID)
			}
		}
	}

	nextNodeMatched, err := r.config.reconcileNextNodes(nextNodeIDs, disconnectedNodeIDs)
	if err != nil {
		r.config.logger.Error("failed to reconcile next nodes", "error", err)
		return
	}
	r.nextNodeMatched = nextNodeMatched
}

// connectedToNextNodes returns true if connected to all next nodes.
func (r *routing1D) connectedToNextNodes() bool {
	for _, nextNodeID := range r.frontwardNextNodeIDs {
		if _, ok := r.neighborhoodInfos[*nextNodeID]; !ok {
			return false
		}
	}
	for _, nextNodeID := range r.backwardNextNodeIDs {
		if _, ok := r.neighborhoodInfos[*nextNodeID]; !ok {
			return false
		}
	}
	return true
}

func (r *routing1D) normalizeScore() {
	sum := 0
	for _, connectedInfo := range r.neighborhoodInfos {
		sum += connectedInfo.scoreByRecv
	}
	rate := float64(1)
	if sum >= 1024*1024 {
		rate = float64(1024*1024) / float64(sum)
	}
	for _, connectedInfo := range r.neighborhoodInfos {
		connectedInfo.scoreByRecv = int(float64(connectedInfo.scoreByRecv) * rate)
	}

	sum = 0
	for _, routeInfo := range r.routeInfos {
		sum += routeInfo.scoreBySend
	}
	rate = float64(1)
	if sum >= 1024*1024 {
		rate = float64(1024*1024) / float64(sum)
	}
	for _, routeInfo := range r.routeInfos {
		routeInfo.scoreBySend = int(float64(routeInfo.scoreBySend) * rate)
	}
}

func isBetween(a, b, target *shared.NodeID) bool {
	if a.Equal(b) {
		panic("a and b should be different")
	}

	// a < b : a <= target && target < b
	if a.Smaller(b) {
		return !target.Smaller(a) && target.Smaller(b)
	}

	// a > b : a <= target || target < b
	return !target.Smaller(a) || target.Smaller(b)
}
