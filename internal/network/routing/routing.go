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
	"log/slog"
	"time"

	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Handler interface {
	routingSetSeedActivated(activated bool)
}

type Config struct {
	Ctx         context.Context
	Logger      *slog.Logger
	LocalNodeID *shared.NodeID
	Handler     Handler
	Transferer  *transferer.Transferer
	Geometry    geometry.CoordinateSystem

	// should be copied from config.RoutingExchangeInterval
	RoutingExchangeInterval time.Duration
	// should be copied from config.SeedConnectRate
	SeedConnectRate uint
	// should be copied from config.SeedReconnectDuration
	SeedReconnectDuration time.Duration
}

type Routing struct {
	config *Config
	r1d    *routing1D
	r2d    *routing2D
	rs     *routingSeed
}

func NewRouting(config *Config) *Routing {
	r := &Routing{
		config: config,
	}

	r.r1d = newRouting1D(&routing1DConfig{
		localNodeID: config.LocalNodeID,
	})

	if config.Geometry != nil {
		r.r2d = newRouting2D(&routing2DConfig{
			geometry: config.Geometry,
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

func (r *Routing) UpdateLocalPosition(pos *geometry.Coordinate) {
	if r.r2d == nil {
		panic("routing 2D is not enabled")
	}

	r.r2d.updateLocalPosition(pos)
}

func (r *Routing) UpdateSeedState(online bool) {
	updated := r.rs.updateSeedState(online)

	if updated {
		r.sendRouting()
	}
}

func (r *Routing) UpdateNodeConnections(connections []*shared.NodeID) {
	updated := r.rs.updateNodeConnections(connections)

	if updated {
		r.sendRouting()
	}

	panic("not implemented")
}

func (r *Routing) CountRecvPacket(from *shared.NodeID, packet *shared.Packet) {
	r.r1d.countRecvPacket(from)
}

func (r *Routing) subRoutine() {
	panic("not implemented")
}

func (r *Routing) sendRouting() {
	panic("not implemented")
}

func (r *Routing) recvRouting(p *shared.Packet) {
	panic("not implemented")
}

func (r *Routing) seedGetActivated() bool {
	panic("not implemented")
}

func (r *Routing) seedSetActivated(activated bool) {
	r.config.Handler.routingSetSeedActivated(activated)
}

func (r *Routing) seedRequestRoutingExchange() {
	panic("not implemented")
}
