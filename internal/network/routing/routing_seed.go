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
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
)

const MAX_SEED_DISTANCE = 99999

type routingSeedHandler interface {
	seedSetEnabled(bool)
}

type routingSeedConfig struct {
	handler routingSeedHandler

	// should be copied from config.RoutingExchangeInterval
	routingExchangeInterval time.Duration
	// should be copied from config.SeedConnectRate
	seedConnectRate uint
	// should be copied from config.SeedReconnectDuration
	seedReconnectDuration time.Duration
}

type seedRouteInfo struct {
	timestamp time.Time
	distance  uint
}

type routingSeed struct {
	config *routingSeedConfig

	mtx              sync.RWMutex
	nextTowardSeed   *shared.NodeID
	distanceFromSeed uint
	seedRouteInfos   map[shared.NodeID]*seedRouteInfo
}

func newRoutingSeed(config *routingSeedConfig) *routingSeed {
	r := &routingSeed{
		config:           config,
		nextTowardSeed:   &shared.NodeIDNone,
		distanceFromSeed: MAX_SEED_DISTANCE,
		seedRouteInfos:   make(map[shared.NodeID]*seedRouteInfo),
	}

	return r
}

func (rs *routingSeed) updateSeedState(online bool) bool {
	rs.mtx.Lock()
	defer rs.mtx.Unlock()

	_, onlineBefore := rs.seedRouteInfos[shared.NodeIDThis]
	if online == onlineBefore {
		return false
	}

	if online {
		rs.seedRouteInfos[shared.NodeIDThis] = &seedRouteInfo{
			timestamp: time.Now(),
			distance:  1,
		}

	} else {
		delete(rs.seedRouteInfos, shared.NodeIDThis)
	}

	return rs.updateNextTowardSeed()
}

func (rs *routingSeed) updateNodeConnections(connections map[shared.NodeID]struct{}) bool {
	rs.mtx.Lock()
	defer rs.mtx.Unlock()

	nextCandidate := &shared.NodeIDNone
	distanceCandidate := uint(MAX_SEED_DISTANCE)
	for nodeID, info := range rs.seedRouteInfos {
		nodeID := nodeID
		if nodeID.Equal(&shared.NodeIDThis) {
			continue
		}

		_, enabled := connections[nodeID]

		// remove record of distance if the node is disconnected
		if !enabled {
			delete(rs.seedRouteInfos, nodeID)
			continue
		}
		if info.distance < distanceCandidate {
			nextCandidate = &nodeID
			distanceCandidate = info.distance
		}
	}

	// disconnected from next node
	if _, ok := rs.seedRouteInfos[*rs.nextTowardSeed]; !ok {
		rs.nextTowardSeed = nextCandidate
		return true
	}

	return false
}

func (rs *routingSeed) getNextStep() *shared.NodeID {
	rs.mtx.RLock()
	defer rs.mtx.RUnlock()

	return rs.nextTowardSeed
}

func (r *routingSeed) recvRoutingPacket(src *shared.NodeID, content *proto.Routing) bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.seedRouteInfos[*src] = &seedRouteInfo{
		timestamp: time.Now(),
		distance:  uint(content.SeedDistance),
	}

	return r.updateNextTowardSeed()
}

func (rs *routingSeed) setupRoutingPacket(content *proto.Routing) {
	rs.mtx.RLock()
	defer rs.mtx.RUnlock()

	if rs.nextTowardSeed.Equal(&shared.NodeIDThis) {
		content.SeedDistance = 1
	} else if info, ok := rs.seedRouteInfos[*rs.nextTowardSeed]; ok {
		content.SeedDistance = uint32(info.distance) + 1
	} else {
		content.SeedDistance = MAX_SEED_DISTANCE
	}
}

func (rs *routingSeed) subRoutine() bool {
	rs.mtx.Lock()
	defer rs.mtx.Unlock()

	if rs.config.seedConnectRate == 0 {
		rs.config.handler.seedSetEnabled(true)
	} else {

		seedRouteInfo := rs.seedRouteInfos[shared.NodeIDThis]
		if seedRouteInfo == nil {
			rs.reviewConnectionToSeed()
		} else if time.Now().After(seedRouteInfo.timestamp.Add(rs.config.seedReconnectDuration)) {
			rs.reviewDisconnectFromSeed()
		}
	}

	if rs.cleanupSeedRouteInfos() {
		return rs.updateNextTowardSeed()
	} else {
		return false
	}
}

func (rs *routingSeed) cleanupSeedRouteInfos() bool {
	now := time.Now()
	updated := false
	for nodeID, info := range rs.seedRouteInfos {
		if nodeID.Equal(&shared.NodeIDThis) {
			continue
		}
		if now.After(info.timestamp.Add(rs.config.routingExchangeInterval * 10)) {
			delete(rs.seedRouteInfos, nodeID)
			updated = true
		}
	}
	return updated
}

func (rs *routingSeed) updateNextTowardSeed() bool {
	prevNext := rs.nextTowardSeed
	prevDistance := rs.distanceFromSeed

	if _, ok := rs.seedRouteInfos[*rs.nextTowardSeed]; !ok {
		rs.nextTowardSeed = &shared.NodeIDNone
		rs.distanceFromSeed = MAX_SEED_DISTANCE
	}

	for nodeID, info := range rs.seedRouteInfos {
		if info.distance < rs.distanceFromSeed {
			rs.nextTowardSeed = &nodeID
			rs.distanceFromSeed = info.distance
		}
	}

	if !prevNext.Equal(rs.nextTowardSeed) || prevDistance != rs.distanceFromSeed {
		return true
	}
	return false
}

func (rs *routingSeed) reviewConnectionToSeed() {
	// skip if connected to seed already
	if rs.nextTowardSeed.Equal(&shared.NodeIDThis) {
		return
	}

	// should connect if there is no connection to next node
	if len(rs.seedRouteInfos) == 0 {
		rs.config.handler.seedSetEnabled(true)
		return
	}

	// skip if there is a node that is closer to seed
	info := rs.seedRouteInfos[*rs.nextTowardSeed]
	if info.distance <= rs.config.seedConnectRate {
		return
	}

	// 1/3 chance of connection to seed
	if rand.Uint32()%3 == 0 {
		rs.config.handler.seedSetEnabled(true)
	}
}

func (rs *routingSeed) reviewDisconnectFromSeed() {
	// skip if not connected to seed
	if !rs.nextTowardSeed.Equal(&shared.NodeIDThis) {
		return
	}

	// skip if there is no nodes that closer to seed
	info := rs.seedRouteInfos[shared.NodeIDThis]
	if info != nil && info.distance > rs.config.seedConnectRate {
		return
	}

	// 1/3 change of disconnect from seed
	if rand.Uint32()%3 == 0 {
		rs.config.handler.seedSetEnabled(false)
	}
}
