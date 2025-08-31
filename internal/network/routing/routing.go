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

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

const (
	connectionUpdateInterval = 10 * time.Second
	requireUpdateConnections = 0x1
	requireExchangeRouting   = 0x2
)

type Handler interface {
	RoutingReconcileNextNodes(nextNodeIDs, disconnectedNodeIDs []*shared.NodeID) (bool, error)
	RoutingUpdateConnection(required, keep map[shared.NodeID]struct{})
	RoutingUpdateNextNodePositions(map[shared.NodeID]*geometry.Coordinate)
}

type Config struct {
	Logger           *slog.Logger
	Handler          Handler
	Observation      config.ObservationCaller
	Transferer       *transferer.Transferer
	CoordinateSystem geometry.CoordinateSystem
}

type Routing struct {
	logger           *slog.Logger
	handler          Handler
	observation      config.ObservationCaller
	transferer       *transferer.Transferer
	coordinateSystem geometry.CoordinateSystem

	localNodeID          *shared.NodeID
	r1d                  *routing1D
	r2d                  *routing2D
	mtx                  sync.Mutex
	lastRouteUpdate      time.Time
	triggeredAction      int
	lastConnectionUpdate time.Time
}

func NewRouting(config *Config) *Routing {
	r := &Routing{
		logger:               config.Logger,
		handler:              config.Handler,
		observation:          config.Observation,
		transferer:           config.Transferer,
		coordinateSystem:     config.CoordinateSystem,
		lastRouteUpdate:      time.Now(),
		lastConnectionUpdate: time.Now(),
	}

	transferer.SetRequestHandler[proto.PacketContent_Routing](config.Transferer, r.recvRouting)

	return r
}

func (r *Routing) Start(ctx context.Context, localNodeID *shared.NodeID) {
	r.localNodeID = localNodeID

	r.r1d = newRouting1D(&routing1DConfig{
		logger:             r.logger,
		localNodeID:        localNodeID,
		reconcileNextNodes: r.handler.RoutingReconcileNextNodes,
	})

	if r.coordinateSystem != nil {
		r.r2d = newRouting2D(&routing2DConfig{
			logger:      r.logger,
			localNodeID: localNodeID,
			geometry:    r.coordinateSystem,
		})
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				r.subRoutine()
			}
		}
	}()
}

func (r *Routing) GetStability() (bool, []*shared.NodeID) {
	return r.r1d.getStability()
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

func (r *Routing) UpdateLocalPosition(pos *geometry.Coordinate) error {
	if r.r2d == nil {
		return fmt.Errorf("position based network is not enabled")
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.triggeredAction = r.triggeredAction | r.r2d.updateLocalPosition(pos)
	return nil
}

func (r *Routing) UpdateNodeConnections(connections map[shared.NodeID]struct{}) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.triggeredAction = r.triggeredAction | r.r1d.updateNodeConnections(connections)
	if r.r2d != nil {
		r.triggeredAction = r.triggeredAction | r.r2d.updateNodeConnections(connections)
	}
}

func (r *Routing) CountRecvPacket(from *shared.NodeID, packet *shared.Packet) {
	r.r1d.countRecvPacket(from)
}

func (r *Routing) subRoutine() {
	r.r1d.subRoutine()

	r.mtx.Lock()
	defer r.mtx.Unlock()

	// TODO: should be optimized
	if r.r2d != nil {
		r.handler.RoutingUpdateNextNodePositions(r.r2d.getNextNodePositions())
	}

	if (r.triggeredAction&requireUpdateConnections) != 0 ||
		time.Now().After(r.lastConnectionUpdate.Add(connectionUpdateInterval)) {
		required, keep := r.r1d.getConnections()
		if r.observation != nil {
			r.observation.UpdateRequiredNodeIDs1D(shared.ConvertNodeIDSetToStringMap(required))
		}
		if r.r2d != nil {
			required2d := r.r2d.getConnections()
			for nodeID := range required2d {
				required[nodeID] = struct{}{}
			}
			if r.observation != nil {
				r.observation.UpdateRequiredNodeIDs2D(shared.ConvertNodeIDSetToStringMap(required2d))
			}
		}
		r.handler.RoutingUpdateConnection(required, keep)
		r.triggeredAction = r.triggeredAction &^ requireUpdateConnections
		r.lastConnectionUpdate = time.Now()
	}

	if (r.triggeredAction & requireExchangeRouting) != 0 {
		r.sendRouting()
		r.triggeredAction = r.triggeredAction &^ requireExchangeRouting
		r.lastRouteUpdate = time.Now()
	}
}

func (r *Routing) sendRouting() {
	content := &proto.Routing{
		NodeRecords: make(map[string]*proto.RoutingNodeRecord),
	}
	r.r1d.setupRoutingPacket(content)
	if r.r2d != nil {
		r.r2d.setupRoutingPacket(content)
	}

	r.transferer.RequestOneWay(&shared.NodeNeighborhoods, shared.PacketModeNoRetry, &proto.PacketContent{
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

	action, err := r.r1d.recvRoutingPacket(src, content)
	if err != nil {
		r.logger.Warn("error on processing routing packet", slog.String("error", err.Error()))
		return
	}
	r.triggeredAction = r.triggeredAction | action
	if r.r2d != nil {
		action, err = r.r2d.recvRoutingPacket(src, content)
		if err != nil {
			r.logger.Warn("error on processing routing packet", slog.String("error", err.Error()))
			return
		}
		r.triggeredAction = r.triggeredAction | action
	}
}
