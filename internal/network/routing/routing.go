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
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/observation"
	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
)

const (
	connectionUpdateInterval = 10 * time.Second
)

type Handler interface {
	RoutingSetSeedEnabled(bool)
	RoutingUpdateConnection(required, keep map[shared.NodeID]struct{})
	RoutingUpdateNextNodePositions(map[shared.NodeID]*geometry.Coordinate)
}

type Config struct {
	Ctx              context.Context
	Logger           *slog.Logger
	LocalNodeID      *shared.NodeID
	Handler          Handler
	Observation      observation.Caller
	Transferer       *transferer.Transferer
	CoordinateSystem geometry.CoordinateSystem

	// should be copied from config.RoutingExchangeInterval
	RoutingExchangeInterval time.Duration
	// should be copied from config.SeedConnectRate
	SeedConnectRate uint
	// should be copied from config.SeedReconnectDuration
	SeedReconnectDuration time.Duration
}

type Routing struct {
	config                  *Config
	r1d                     *routing1D
	r2d                     *routing2D
	rs                      *routingSeed
	mtx                     sync.Mutex
	requireUpdateRoute      bool
	lastRouteUpdate         time.Time
	requireUpdateConnection bool
	lastConnectionUpdate    time.Time
}

func NewRouting(config *Config) *Routing {
	r := &Routing{
		config:               config,
		mtx:                  sync.Mutex{},
		lastRouteUpdate:      time.Now(),
		lastConnectionUpdate: time.Now(),
	}

	r.r1d = newRouting1D(&routing1DConfig{
		localNodeID: config.LocalNodeID,
	})

	if config.CoordinateSystem != nil {
		r.r2d = newRouting2D(&routing2DConfig{
			logger:      config.Logger,
			localNodeID: config.LocalNodeID,
			geometry:    config.CoordinateSystem,
		})
	}

	r.rs = newRoutingSeed(&routingSeedConfig{
		handler:                 r,
		routingExchangeInterval: config.RoutingExchangeInterval,
		seedConnectRate:         config.SeedConnectRate,
		seedReconnectDuration:   config.SeedReconnectDuration,
	})

	transferer.SetRequestHandler[proto.PacketContent_Routing](config.Transferer, r.recvRouting)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.config.Ctx.Done():
				return

			case <-ticker.C:
				r.subRoutine()
			}
		}
	}()

	return r
}

func (r *Routing) GetNextStep1D(packet *shared.Packet) *shared.NodeID {
	return r.r1d.getNextStep(packet)
}

func (r *Routing) GetNextStep2D(dst *geometry.Coordinate) *shared.NodeID {
	if r.r2d == nil {
		panic("routing 2D is not enabled")
	}

	return r.r2d.getNextStep(dst)
}

func (r *Routing) GetNextStepToSeed() *shared.NodeID {
	return r.rs.getNextStep()
}

func (r *Routing) UpdateLocalPosition(pos *geometry.Coordinate) error {
	if r.r2d == nil {
		return fmt.Errorf("position based network is not enabled")
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.requireUpdateRoute = r.r2d.updateLocalPosition(pos) || r.requireUpdateRoute
	return nil
}

func (r *Routing) UpdateSeedState(online bool) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.requireUpdateRoute = r.rs.updateSeedState(online) || r.requireUpdateRoute
}

func (r *Routing) UpdateNodeConnections(connections map[shared.NodeID]struct{}) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.requireUpdateRoute = r.rs.updateNodeConnections(connections) || r.requireUpdateRoute
	r.requireUpdateConnection = r.r1d.updateNodeConnections(connections) || r.requireUpdateConnection
	if r.r2d != nil {
		r.requireUpdateConnection = r.r2d.updateNodeConnections(connections) || r.requireUpdateConnection
	}
}

func (r *Routing) CountRecvPacket(from *shared.NodeID, packet *shared.Packet) {
	r.r1d.countRecvPacket(from)
}

func (r *Routing) subRoutine() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	// TODO: should be optimized
	if r.r2d != nil {
		r.config.Handler.RoutingUpdateNextNodePositions(r.r2d.getNextNodePositions())
	}

	r.requireUpdateRoute = r.rs.subRoutine() || r.requireUpdateRoute || r.requireUpdateConnection

	if r.requireUpdateConnection || time.Now().After(r.lastConnectionUpdate.Add(connectionUpdateInterval)) {
		required, keep := r.r1d.getConnections()
		r.config.Observation.UpdateRequiredNodeIDs1D(shared.ConvertNodeIDSetToStringMap(required))
		if r.r2d != nil {
			required2d := r.r2d.getConnections()
			for nodeID := range required2d {
				required[nodeID] = struct{}{}
			}
			r.config.Observation.UpdateRequiredNodeIDs2D(shared.ConvertNodeIDSetToStringMap(required2d))
		}
		r.config.Handler.RoutingUpdateConnection(required, keep)
		r.requireUpdateConnection = false
		r.lastConnectionUpdate = time.Now()
	}

	if r.requireUpdateRoute || time.Now().After(r.lastRouteUpdate.Add(r.config.RoutingExchangeInterval)) {
		r.sendRouting()
		r.requireUpdateRoute = false
		r.lastRouteUpdate = time.Now()
	}
}

func (r *Routing) sendRouting() {
	content := &proto.Routing{
		NodeRecords: make(map[string]*proto.RoutingNodeRecord),
	}
	r.rs.setupRoutingPacket(content)
	r.r1d.setupRoutingPacket(content)
	if r.r2d != nil {
		r.r2d.setupRoutingPacket(content)
	}

	r.config.Transferer.RequestOneWay(&shared.NodeIDNext, shared.PacketModeNoRetry, &proto.PacketContent{
		Content: &proto.PacketContent_Routing{
			Routing: content,
		},
	})
}

func (r *Routing) recvRouting(p *shared.Packet) {
	src := p.SrcNodeID
	content := p.Content.GetRouting()

	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.requireUpdateRoute = r.rs.recvRoutingPacket(src, content) || r.requireUpdateRoute
	r.requireUpdateConnection = r.r1d.recvRoutingPacket(src, content) || r.requireUpdateConnection
	if r.r2d != nil {
		r.requireUpdateConnection = r.r2d.recvRoutingPacket(src, content) || r.requireUpdateConnection
	}
}

// implement handlers
func (r *Routing) seedSetEnabled(enabled bool) {
	r.config.Handler.RoutingSetSeedEnabled(enabled)
}
