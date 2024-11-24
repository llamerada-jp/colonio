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
	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/network/node_accessor"
	"github.com/llamerada-jp/colonio/internal/network/routing"
	"github.com/llamerada-jp/colonio/internal/network/seed_accessor"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/observation"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Handler interface {
	NetworkRecvConfig(*config.Cluster)
	NetworkUpdateNextNodePosition(map[shared.NodeID]*geometry.Coordinate)
}

type Config struct {
	Ctx         context.Context
	Logger      *slog.Logger
	LocalNodeID *shared.NodeID
	Handler     Handler
	Observation observation.Caller
	// Allow insecure server connections. You must not use this option in production. Only for testing.
	Insecure bool
	// Interval between retries when a network error occurs.
	SeedTripInterval time.Duration
}

type Network struct {
	config           *Config
	ctx              context.Context
	cancel           context.CancelFunc
	mtx              sync.RWMutex
	enabled          bool
	routing          *routing.Routing
	seedAccessor     *seed_accessor.SeedAccessor
	nodeAccessor     *node_accessor.NodeAccessor
	transferer       *transferer.Transferer
	coordinateSystem geometry.CoordinateSystem

	// maximum number of hops that a packet can be relayed.
	hopLimit uint
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

	if n.transferer != nil {
		return errors.New("network module can be connected only once")
	}

	n.transferer = transferer.NewTransferer(&transferer.Config{
		Ctx:         n.ctx,
		Logger:      n.config.Logger,
		LocalNodeID: n.config.LocalNodeID,
		Handler:     n,
	})
	n.seedAccessor = seed_accessor.NewSeedAccessor(&seed_accessor.Config{
		Ctx:    n.ctx,
		Logger: n.config.Logger,
		Transporter: seed_accessor.DefaultSeedTransporterFactory(&seed_accessor.SeedTransporterOption{
			Verification: !n.config.Insecure,
		}),
		Handler:      n,
		URL:          url,
		LocalNodeID:  n.config.LocalNodeID,
		Token:        token,
		TripInterval: n.config.SeedTripInterval,
	})
	n.nodeAccessor = node_accessor.NewNodeAccessor(&node_accessor.Config{
		Ctx:         n.ctx,
		Logger:      n.config.Logger,
		LocalNodeID: n.config.LocalNodeID,
		Handler:     n,
		Transferer:  n.transferer,
	})
	n.enabled = true

	return nil
}

func (n *Network) Disconnect() {
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
}

func (n *Network) IsOnline() bool {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if n.nodeAccessor != nil && n.nodeAccessor.IsOnline() {
		return true
	}

	if n.seedAccessor != nil && n.seedAccessor.IsAuthenticated() && n.seedAccessor.IsOnlyOne() {
		return true
	}

	return false
}

func (n *Network) GetTransferer() *transferer.Transferer {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	return n.transferer
}

func (n *Network) GetCoordinateSystem() geometry.CoordinateSystem {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	return n.coordinateSystem
}

func (n *Network) UpdateLocalPosition(pos *geometry.Coordinate) error {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if n.routing == nil {
		return errors.New("network is not initialized")
	}

	return n.routing.UpdateLocalPosition(pos)
}

func (n *Network) GetNextStep2D(dst *geometry.Coordinate) *shared.NodeID {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if n.routing == nil {
		panic("routing 2D is not enabled")
	}

	return n.routing.GetNextStep2D(dst)
}

// implement handlers

func (n *Network) NodeAccessorRecvPacket(from *shared.NodeID, packet *shared.Packet) {
	if !n.checkHopCount(packet) {
		return
	}
	if n.routing != nil && from != nil {
		n.routing.CountRecvPacket(from, packet)
	}

	n.classifyPacket(packet, false)
}

func (n *Network) NodeAccessorChangeConnections(connections map[shared.NodeID]struct{}) {
	// use go routine to avoid deadlock
	go func() {
		n.mtx.RLock()
		defer n.mtx.RUnlock()

		if n.routing != nil {
			n.routing.UpdateNodeConnections(connections)
		}
	}()

	n.config.Observation.ChangeConnectedNodes(shared.ConvertNodeIDSetToStringMap(connections))
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

func (n *Network) RoutingUpdateNextNodePositions(positions map[shared.NodeID]*geometry.Coordinate) {
	n.config.Handler.NetworkUpdateNextNodePosition(positions)
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

	n.mtx.Lock()
	defer n.mtx.Unlock()

	if n.routing != nil {
		panic("config already received")
	}

	n.hopLimit = clusterConfig.HopLimit

	if clusterConfig.Geometry != nil {
		if clusterConfig.Geometry.Plane != nil {
			n.coordinateSystem = geometry.NewPlaneCoordinateSystem(clusterConfig.Geometry.Plane)

		} else if clusterConfig.Geometry.Sphere != nil {
			n.coordinateSystem = geometry.NewSphereCoordinateSystem(clusterConfig.Geometry.Sphere)
		}
	}

	n.routing = routing.NewRouting(&routing.Config{
		Ctx:                     n.ctx,
		Logger:                  n.config.Logger,
		LocalNodeID:             n.config.LocalNodeID,
		Handler:                 n,
		Observation:             n.config.Observation,
		Transferer:              n.transferer,
		CoordinateSystem:        n.coordinateSystem,
		RoutingExchangeInterval: clusterConfig.RoutingExchangeInterval,
		SeedConnectRate:         clusterConfig.SeedConnectRate,
		SeedReconnectDuration:   clusterConfig.SeedReconnectDuration,
	})

	n.nodeAccessor.SetConfig(clusterConfig.IceServers, &node_accessor.NodeLinkConfig{
		SessionTimeout:    clusterConfig.SessionTimeout,
		KeepaliveInterval: clusterConfig.KeepaliveInterval,
		BufferInterval:    clusterConfig.BufferInterval,
		PacketBaseBytes:   clusterConfig.WebRTCPacketBaseBytes,
	})
	n.nodeAccessor.SetEnabled(true)

	n.config.Handler.NetworkRecvConfig(clusterConfig)
}

func (n *Network) SeedRecvPacket(packet *shared.Packet) {
	if !n.checkHopCount(packet) {
		return
	}

	n.classifyPacket(packet, true)
}

func (n *Network) SeedRequireRandomConnect() {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if n.nodeAccessor != nil {
		n.nodeAccessor.ConnectRandomLink()
	}
}

func (n *Network) TransfererSendPacket(packet *shared.Packet) {
	n.classifyPacket(packet, false)
}

func (n *Network) TransfererRelayPacket(dstNodeID *shared.NodeID, packet *shared.Packet) {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if !n.enabled {
		return
	}

	if !n.checkHopCount(packet) {
		return
	}

	err := n.nodeAccessor.RelayPacket(dstNodeID, packet)
	if err != nil {
		n.config.Logger.Debug("failed to relay packet", slog.String("error", err.Error()))
	}
}

func (n *Network) checkHopCount(packet *shared.Packet) bool {
	if packet.HopCount > uint32(n.hopLimit) {
		return false
	}

	packet.HopCount++

	return true
}

func (n *Network) classifyPacket(packet *shared.Packet, from_seed bool) {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if !n.enabled {
		return
	}

	if (packet.Mode & shared.PacketModeRelaySeed) != 0x0 {
		if (packet.Mode&shared.PacketModeResponse) != 0x0 && !from_seed {
			nextStepToSeed := n.routing.GetNextStepToSeed()
			if *nextStepToSeed == shared.NodeIDThis {
				n.seedAccessor.RelayPacket(packet)
				return
			}

			err := n.nodeAccessor.RelayPacket(nextStepToSeed, packet)
			if err != nil {
				n.config.Logger.Debug("failed to relay packet", slog.String("error", err.Error()))
			}
			return
		}

		if *packet.SrcNodeID == *n.config.LocalNodeID {
			if *n.routing.GetNextStepToSeed() == shared.NodeIDThis {
				n.seedAccessor.RelayPacket(packet)
			} else {
				n.config.Logger.Debug("drop a packet addressed to the seed")
			}
			return
		}
	}

	nextNodeID := n.routing.GetNextStep1D(packet)
	if *nextNodeID == shared.NodeIDThis || *nextNodeID == *n.config.LocalNodeID {
		n.transferer.Receive(packet)
		return

	} else if nextNodeID.IsNormal() || *nextNodeID == shared.NodeIDNext {
		n.nodeAccessor.RelayPacket(nextNodeID, packet)
		return

	} else if *nextNodeID == shared.NodeIDNone && (packet.Mode&shared.PacketModeOneWay) == 0x0 {
		n.transferer.Error(packet, constants.PacketErrorCodeNoOneReceive, "no one receive the packet")
		return
	}

	n.config.Logger.Debug("drop packet")
}
