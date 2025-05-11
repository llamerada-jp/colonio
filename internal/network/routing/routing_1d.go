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
	"slices"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
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
	localNodeID *shared.NodeID
}

type connectedInfo struct {
	timestamp        time.Time
	level            int
	connectedNodeIDs map[shared.NodeID]struct{}
	oddScore         int
	scoreByRecv      int
}

type routeInfo1D struct {
	nodeID          *shared.NodeID
	connectedNodeID *shared.NodeID
	level           int
	scoreBySend     int
}

type routing1D struct {
	config         *routing1DConfig
	mtx            sync.RWMutex
	prevNodeID     *shared.NodeID
	nextNodeID     *shared.NodeID
	connectedInfos map[shared.NodeID]*connectedInfo
	routeInfos     []*routeInfo1D
}

func newRouting1D(config *routing1DConfig) *routing1D {
	return &routing1D{
		config:         config,
		connectedInfos: make(map[shared.NodeID]*connectedInfo),
		routeInfos:     make([]*routeInfo1D, 0),
	}
}

func (r *routing1D) updateNodeConnections(connections map[shared.NodeID]struct{}) bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	updated := false

	for nodeID := range connections {
		if _, ok := r.connectedInfos[nodeID]; !ok {
			r.connectedInfos[nodeID] = &connectedInfo{
				timestamp:        time.Now(),
				level:            r.calcLevel(&nodeID),
				connectedNodeIDs: make(map[shared.NodeID]struct{}),
			}
			updated = true
		}
	}

	for nodeID := range r.connectedInfos {
		if _, ok := connections[nodeID]; !ok {
			delete(r.connectedInfos, nodeID)
			updated = true
		}
	}

	if !updated {
		return false
	}
	r.updateRouteInfos()
	return r.nextNodeIDChanged()
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
	if r.nextNodeID == nil {
		if isExplicit {
			return nil
		}
		return &shared.NodeLocal
	}

	if isBetween(r.config.localNodeID, r.nextNodeID, packet.DstNodeID) {
		if isExplicit {
			return nil
		}
		return &shared.NodeLocal
	}

	if isBetween(r.prevNodeID, r.config.localNodeID, packet.DstNodeID) {
		if isExplicit && !r.prevNodeID.Equal(packet.DstNodeID) {
			return nil
		}
		return r.prevNodeID
	}

	last := r.routeInfos[len(r.routeInfos)-1]
	if !packet.DstNodeID.Smaller(last.nodeID) {
		return last.connectedNodeID
	}
	// TODO: binary search is better
	candidate := last
	for _, info := range r.routeInfos {
		if packet.DstNodeID.Smaller(info.nodeID) {
			candidate.scoreBySend += 1
			return candidate.connectedNodeID
		}
		candidate = info
	}

	panic("next step node should be found in routing infos")
}

func (r *routing1D) countRecvPacket(from *shared.NodeID) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if info, ok := r.connectedInfos[*from]; ok {
		info.scoreByRecv += 1
	}
}

func (r *routing1D) recvRoutingPacket(src *shared.NodeID, content *proto.Routing) (bool, error) {
	if r.config.localNodeID.Equal(src) {
		panic("it is not allowed to receive packet from local node")
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	info, ok := r.connectedInfos[*src]
	if !ok {
		return false, nil
	}

	updated := false
	for nodeID := range info.connectedNodeIDs {
		if _, ok := content.NodeRecords[nodeID.String()]; !ok {
			delete(info.connectedNodeIDs, nodeID)
			updated = true
		}
	}

	oddScore := 0
	for nodeIDStr, record := range content.NodeRecords {
		nodeID, err := shared.NewNodeIDFromString(nodeIDStr)
		if err != nil {
			return false, fmt.Errorf("failed to parse node id %w", err)
		}
		if nodeID.Equal(r.config.localNodeID) {
			oddScore = int(record.R1DScore)
			continue
		}
		if _, ok := info.connectedNodeIDs[*nodeID]; !ok {
			info.connectedNodeIDs[*nodeID] = struct{}{}
			updated = true
		}
	}
	info.oddScore = oddScore

	if !updated {
		return false, nil
	}
	r.updateRouteInfos()
	return r.nextNodeIDChanged(), nil
}

func (r *routing1D) setupRoutingPacket(content *proto.Routing) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.normalizeScore()

	for nodeID, info := range r.connectedInfos {
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
	if r.prevNodeID != nil {
		required[*r.nextNodeID] = struct{}{}
		required[*r.prevNodeID] = struct{}{}
	}

	for connectedNodeID, connectedInfo := range r.connectedInfos {
		connectedNodeID := connectedNodeID
		// keep connections when the connection level is over the levelRangesCount
		if connectedInfo.level == -1 {
			keep[connectedNodeID] = struct{}{}
			continue
		}

		// keep connections when the number of connections is less than linksMin or the connection is yang
		if len(required) < linksMin ||
			len(connectedInfo.connectedNodeIDs) < linksMin ||
			now.Before(connectedInfo.timestamp.Add(linkKeepLinkDuration)) {
			required[connectedNodeID] = struct{}{}
			requiredLevelCount[connectedInfo.level]++
			continue
		}

		// keep connections when the nearest node from the connected node is this node
		n, p := getNextAndPrevNodeID((&connectedNodeID), connectedInfo.connectedNodeIDs)
		if n.Equal(r.config.localNodeID) || p.Equal(r.config.localNodeID) {
			required[connectedNodeID] = struct{}{}
			requiredLevelCount[connectedInfo.level]++
			continue
		}

		candidatesByLevel[connectedInfo.level][connectedNodeID] = connectedInfo.scoreByRecv
	}

	for _, routeInfo := range r.routeInfos {
		if routeInfo.level == -1 {
			continue
		}
		if _, ok := required[*routeInfo.nodeID]; ok {
			continue
		}
		if _, ok := r.connectedInfos[*routeInfo.nodeID]; ok {
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
				nodeID := nodeID
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

	for nodeID, cInfo := range r.connectedInfos {
		nodeID := nodeID
		idMap[nodeID] = &nodeID
		for connectedNodeID := range cInfo.connectedNodeIDs {
			if r.config.localNodeID.Equal(&connectedNodeID) {
				continue
			}
			if _, ok := idMap[connectedNodeID]; !ok {
				idMap[connectedNodeID] = &nodeID
				continue
			}
			if idMap[connectedNodeID].Equal(&connectedNodeID) {
				continue
			}
			distance1 := idMap[connectedNodeID].DistanceFrom(&connectedNodeID)
			distance2 := nodeID.DistanceFrom(&connectedNodeID)
			if distance2.Smaller(distance1) {
				idMap[connectedNodeID] = &nodeID
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
		rInfo.connectedNodeID = nodeID
		delete(idMap, *rInfo.nodeID)
	}

	for nodeID, connectedNodeID := range idMap {
		nodeID := nodeID
		r.routeInfos = append(r.routeInfos, &routeInfo1D{
			nodeID:          &nodeID,
			connectedNodeID: connectedNodeID,
			level:           r.calcLevel(&nodeID),
			scoreBySend:     0,
		})
	}

	slices.SortFunc(r.routeInfos, func(a, b *routeInfo1D) int {
		if a.nodeID.Smaller(b.nodeID) {
			return -1
		}
		if a.nodeID.Equal(b.nodeID) {
			return 0
		}
		return 1
	})
}

func (r *routing1D) nextNodeIDChanged() bool {
	// not connected any node
	if len(r.connectedInfos) == 0 {
		if r.nextNodeID == nil {
			return false
		}
		r.nextNodeID = nil
		r.prevNodeID = nil
		return true
	}

	connectedNext := &shared.NodeLocal
	connectedPrev := (*shared.NodeID)(nil)
	connectedMin := &shared.NodeLocal
	connectedMax := (*shared.NodeID)(nil)
	knownNodeIDs := make(map[shared.NodeID]struct{})

	for nodeID, info := range r.connectedInfos {
		nodeID := nodeID
		if r.config.localNodeID.Smaller(&nodeID) && nodeID.Smaller(connectedNext) {
			connectedNext = &nodeID
		}
		if nodeID.Smaller(r.config.localNodeID) && (connectedPrev == nil || connectedPrev.Smaller(&nodeID)) {
			connectedPrev = &nodeID
		}
		if nodeID.Smaller(connectedMin) {
			connectedMin = &nodeID
		}
		if connectedMax == nil || connectedMax.Smaller(&nodeID) {
			connectedMax = &nodeID
		}

		knownNodeIDs[nodeID] = struct{}{}
		for connectedNodeID := range info.connectedNodeIDs {
			knownNodeIDs[connectedNodeID] = struct{}{}
		}
	}
	if connectedNext.Equal(&shared.NodeLocal) {
		connectedNext = connectedMin
	}
	if connectedPrev == nil {
		connectedPrev = connectedMax
	}

	n, p := getNextAndPrevNodeID(r.config.localNodeID, knownNodeIDs)
	if connectedNext.Equal(r.nextNodeID) && connectedNext.Equal(n) &&
		connectedPrev.Equal(r.prevNodeID) && connectedPrev.Equal(p) {
		return false
	}

	r.nextNodeID, r.prevNodeID = n, p
	return true
}

func (r *routing1D) normalizeScore() {
	sum := 0
	for _, connectedInfo := range r.connectedInfos {
		sum += connectedInfo.scoreByRecv
	}
	rate := float64(1)
	if sum >= 1024*1024 {
		rate = float64(1024*1024) / float64(sum)
	}
	for _, connectedInfo := range r.connectedInfos {
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

func getNextAndPrevNodeID(baseNodeID *shared.NodeID, nodeIDs map[shared.NodeID]struct{}) (*shared.NodeID, *shared.NodeID) {
	nextCandidate := &shared.NodeLocal
	prevCandidate := (*shared.NodeID)(nil)
	minNodeID := &shared.NodeLocal
	maxNodeID := (*shared.NodeID)(nil)
	for nodeID := range nodeIDs {
		nodeID := nodeID
		if baseNodeID.Equal(&nodeID) {
			continue
		}
		if baseNodeID.Smaller(&nodeID) && nodeID.Smaller(nextCandidate) {
			nextCandidate = &nodeID
		}
		if nodeID.Smaller(baseNodeID) && (prevCandidate == nil || prevCandidate.Smaller(&nodeID)) {
			prevCandidate = &nodeID
		}
		if nodeID.Smaller(minNodeID) {
			minNodeID = &nodeID
		}
		if maxNodeID == nil || maxNodeID.Smaller(&nodeID) {
			maxNodeID = &nodeID
		}
	}
	if nextCandidate.Equal(&shared.NodeLocal) {
		nextCandidate = minNodeID
		if nextCandidate.Equal(&shared.NodeLocal) {
			nextCandidate = nil
		}
	}
	if prevCandidate == nil {
		prevCandidate = maxNodeID
	}

	return nextCandidate, prevCandidate
}
