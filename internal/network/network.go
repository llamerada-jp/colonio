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
package network

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/network/node_accessor"
	"github.com/llamerada-jp/colonio/internal/network/routing"
	"github.com/llamerada-jp/colonio/internal/network/seed_accessor"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Handler interface {
	NetworkRecvConfig(*config.Cluster)
}

type Config struct {
	Ctx     context.Context
	Logger  *slog.Logger
	Handler Handler
	// Allow insecure server connections. You must not use this option in production. Only for testing.
	Insecure bool
	// Interval between retries when a network error occurs.
	SeedTripInterval time.Duration
}

type Network struct {
	config       *Config
	ctx          context.Context
	cancel       context.CancelFunc
	mtx          sync.RWMutex
	enabled      bool
	localNID     *shared.NodeID
	routing      *routing.Routing
	seedAccessor *seed_accessor.SeedAccessor
	nodeAccessor *node_accessor.NodeAccessor
	transferer   *transferer.Transferer
}

func NewNetwork(config *Config) *Network {
	ctx, cancel := context.WithCancel(config.Ctx)

	return &Network{
		config:  config,
		ctx:     ctx,
		cancel:  cancel,
		mtx:     sync.RWMutex{},
		enabled: false,
	}
}

func (n *Network) Connect(url string, token []byte) error {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if n.localNID != nil {
		return errors.New("network module can be connected only once")
	}

	n.localNID = shared.NewRandomNodeID()
	n.transferer = transferer.NewTransferer(&transferer.Config{
		Ctx:      n.ctx,
		Logger:   n.config.Logger,
		LocalNID: n.localNID,
		Handler:  n,
	})
	n.seedAccessor = seed_accessor.NewSeedAccessor(&seed_accessor.Config{
		Ctx:    n.ctx,
		Logger: n.config.Logger,
		Transporter: seed_accessor.DefaultSeedTransporterFactory(&seed_accessor.SeedTransporterOption{
			Verification: !n.config.Insecure,
		}),
		Handler:      n,
		URL:          url,
		LocalNID:     n.localNID,
		Token:        token,
		TripInterval: n.config.SeedTripInterval,
	})
	n.nodeAccessor = node_accessor.NewNodeAccessor(&node_accessor.Config{
		Ctx:      n.ctx,
		Logger:   n.config.Logger,
		LocalNID: n.localNID,
		Handler:  n,
	})
	n.enabled = true

	return nil
}

func (n *Network) Disconnect() error {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.enabled = false

	n.routing = nil

	if n.nodeAccessor != nil {
		n.nodeAccessor.SetEnabled(false)
		n.nodeAccessor = nil
	}

	if n.seedAccessor != nil {
		n.seedAccessor.SetEnabled(false)
		n.seedAccessor.Destruct()
		n.seedAccessor = nil
	}

	n.cancel()

	return nil
}

func (n *Network) IsOnline() bool {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if n.nodeAccessor != nil && n.nodeAccessor.IsOnline() {
		return true
	}

	if n.seedAccessor.IsAuthenticated() && n.seedAccessor.IsOnlyOne() {
		return true
	}

	return false
}

func (n *Network) UpdateLocalPosition(pos *geometry.Coordinate) {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if n.routing != nil {
		n.routing.UpdateLocalPosition(pos)
	}
}

// implement handlers

func (n *Network) NodeAccessorRecvPacket(*shared.Packet) {
	panic("not implemented")
}

func (n *Network) RoutingSetSeedEnabled(enabled bool) {
	if n.seedAccessor != nil {
		n.seedAccessor.SetEnabled(enabled)
	}
}

func (n *Network) RoutingUpdateConnection(required, keep map[shared.NodeID]struct{}) {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if n.nodeAccessor != nil {
		n.nodeAccessor.ConnectLinks(required, keep)
	}
}

func (n *Network) SeedAuthorizeFailed() {
	panic("not implemented")
}

func (n *Network) SeedChangeState(online bool) {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if n.routing != nil {
		n.routing.UpdateSeedState(online)
	}
}

func (n *Network) SeedRecvConfig(clusterConfig *config.Cluster) {
	// TODO: receive config twice and those are different, suggest to rerun the network.
	if n.routing != nil {
		panic("config already received")
	}

	var cs geometry.CoordinateSystem
	if clusterConfig.Geometry != nil {
		if clusterConfig.Geometry.Plane != nil {
			cs = geometry.NewPlaneCoordinateSystem(clusterConfig.Geometry.Plane)

		} else if clusterConfig.Geometry.Sphere != nil {
			cs = geometry.NewSphereCoordinateSystem(clusterConfig.Geometry.Sphere)
		}
	}

	n.routing = routing.NewRouting(&routing.Config{
		Ctx:         n.ctx,
		Logger:      n.config.Logger,
		LocalNodeID: n.localNID,
		Handler:     n,
		Transferer:  n.transferer,
		Geometry:    cs,
	})

	n.nodeAccessor.SetConfig(clusterConfig.IceServers, &node_accessor.NodeLinkConfig{
		SessionTimeout:    clusterConfig.SessionTimeout,
		KeepaliveInterval: clusterConfig.KeepaliveInterval,
		BufferInterval:    clusterConfig.BufferInterval,
		PacketBaseBytes:   clusterConfig.WebRTCPacketBaseBytes,
	})
	n.nodeAccessor.SetEnabled(true)
}

func (n *Network) SeedRecvPacket(*shared.Packet) {
	panic("not implemented")
}

func (n *Network) SeedRequireRandomConnect() {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if n.nodeAccessor != nil {
		n.nodeAccessor.ConnectRandomLink()
	}
}

func (n *Network) TransfererSendPacket(*shared.Packet) {
	panic("not implemented")
}

func (n *Network) TransfererRelayPacket(*shared.NodeID, *shared.Packet) {
	panic("not implemented")
}
