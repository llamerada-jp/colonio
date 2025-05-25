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
	"log/slog"
	"net/http"
	"time"

	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/network/node_accessor"
	"github.com/llamerada-jp/colonio/internal/network/routing"
	"github.com/llamerada-jp/colonio/internal/network/seed_accessor"
	"github.com/llamerada-jp/colonio/internal/network/signal"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/observation"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Handler interface {
	NetworkUpdateNextNodePosition(map[shared.NodeID]*geometry.Coordinate)
}

type Config struct {
	Logger           *slog.Logger
	Handler          Handler
	Observation      observation.Caller
	CoordinateSystem geometry.CoordinateSystem

	// config parameters for seed
	HttpClient *http.Client // optional
	SeedURL    string
	// config parameters for webrtc node
	NLC *node_accessor.NodeLinkConfig

	// maximum number of hops that a packet can be relayed.
	PacketHopLimit uint
}

type Network struct {
	logger      *slog.Logger
	handler     Handler
	observation observation.Caller

	localNodeID *shared.NodeID

	seedAccessor *seed_accessor.SeedAccessor
	nodeAccessor *node_accessor.NodeAccessor
	transferer   *transferer.Transferer
	routing      *routing.Routing

	packetHopLimit uint
}

func NewNetwork(config *Config) (*Network, error) {
	n := &Network{
		logger:         config.Logger,
		handler:        config.Handler,
		observation:    config.Observation,
		packetHopLimit: config.PacketHopLimit,
	}

	n.seedAccessor = seed_accessor.NewSeedAccessor(&seed_accessor.Config{
		Logger:     config.Logger,
		Handler:    n,
		URL:        config.SeedURL,
		HttpClient: config.HttpClient,
	})

	na, err := node_accessor.NewNodeAccessor(&node_accessor.Config{
		Logger:         config.Logger,
		Handler:        n,
		NodeLinkConfig: config.NLC,
	})
	if err != nil {
		return nil, err
	}
	n.nodeAccessor = na

	n.transferer = transferer.NewTransferer(&transferer.Config{
		Logger:  config.Logger,
		Handler: n,
	})

	n.routing = routing.NewRouting(&routing.Config{
		Logger:           config.Logger,
		Handler:          n,
		Observation:      config.Observation,
		Transferer:       n.transferer,
		CoordinateSystem: config.CoordinateSystem,
	})

	return n, nil
}

func (n *Network) Start(ctx context.Context) (*shared.NodeID, error) {
	var err error
	n.localNodeID, err = n.seedAccessor.Start(ctx)
	if err != nil {
		return nil, err
	}

	n.nodeAccessor.Start(ctx, n.localNodeID)
	n.transferer.Start(ctx, n.localNodeID)
	n.routing.Start(ctx, n.localNodeID)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n.nodeAccessor.SetBeAlone(n.seedAccessor.IsAlone())
			}
		}
	}()

	return n.localNodeID, nil
}

func (n *Network) IsOnline() bool {
	return n.seedAccessor.IsAlone() || n.nodeAccessor.IsOnline()
}

func (n *Network) GetTransferer() *transferer.Transferer {
	return n.transferer
}

func (n *Network) UpdateLocalPosition(pos *geometry.Coordinate) error {
	return n.routing.UpdateLocalPosition(pos)
}

func (n *Network) GetNextStep2D(dst *geometry.Coordinate) *shared.NodeID {
	return n.routing.GetNextStep2D(dst)
}

// implements for seed_accessor.Handler
func (n *Network) SeedRecvSignalOffer(srcNodeID *shared.NodeID, offer *signal.Offer) {
	n.nodeAccessor.SignalingOffer(srcNodeID, offer)
}

func (n *Network) SeedRecvSignalAnswer(srcNodeID *shared.NodeID, answer *signal.Answer) {
	n.nodeAccessor.SignalingAnswer(srcNodeID, answer)
}

func (n *Network) SeedRecvSignalICE(srcNodeID *shared.NodeID, ice *signal.ICE) {
	n.nodeAccessor.SignalingICE(srcNodeID, ice)
}

// implements for node_accessor.Handler
func (n *Network) NodeAccessorRecvPacket(from *shared.NodeID, packet *shared.Packet) {
	if !n.checkHopCount(packet) {
		return
	}
	if from != nil {
		n.routing.CountRecvPacket(from, packet)
	}

	n.classifyPacket(packet)
}

func (n *Network) NodeAccessorChangeConnections(connections map[shared.NodeID]struct{}) {
	// use go routine to avoid deadlock
	go n.routing.UpdateNodeConnections(connections)

	n.observation.ChangeConnectedNodes(shared.ConvertNodeIDSetToStringMap(connections))
}

func (n *Network) NodeAccessorSendSignalOffer(dstNodeID *shared.NodeID, offer *signal.Offer) error {
	return n.seedAccessor.SendSignalOffer(dstNodeID, offer)
}

func (n *Network) NodeAccessorSendSignalAnswer(dstNodeID *shared.NodeID, answer *signal.Answer) error {
	return n.seedAccessor.SendSignalAnswer(dstNodeID, answer)
}

func (n *Network) NodeAccessorSendSignalICE(dstNodeID *shared.NodeID, ice *signal.ICE) error {
	return n.seedAccessor.SendSignalICE(dstNodeID, ice)
}

// implements for transferer.Handler
func (n *Network) TransfererSendPacket(packet *shared.Packet) {
	n.classifyPacket(packet)
}

func (n *Network) TransfererRelayPacket(dstNodeID *shared.NodeID, packet *shared.Packet) {
	if !n.checkHopCount(packet) {
		return
	}

	err := n.nodeAccessor.RelayPacket(dstNodeID, packet)
	if err != nil {
		n.logger.Debug("failed to relay packet", slog.String("error", err.Error()))
	}
}

// implements for routing.Handler
func (n *Network) RoutingUpdateConnection(required, keep map[shared.NodeID]struct{}) {
	n.nodeAccessor.ConnectLinks(required, keep)
}

func (n *Network) RoutingUpdateNextNodePositions(positions map[shared.NodeID]*geometry.Coordinate) {
	n.handler.NetworkUpdateNextNodePosition(positions)
}

func (n *Network) checkHopCount(packet *shared.Packet) bool {
	if packet.HopCount > uint32(n.packetHopLimit) {
		return false
	}

	packet.HopCount++

	return true
}

func (n *Network) classifyPacket(packet *shared.Packet) {
	nextNodeID := n.routing.GetNextStep1D(packet)
	if nextNodeID == nil {
		if (packet.Mode & shared.PacketModeOneWay) == 0x0 {
			n.transferer.Error(packet, constants.PacketErrorCodeNoOneReceive, "no one receive the packet")
			return
		}
	} else if nextNodeID.Equal(&shared.NodeLocal) || nextNodeID.Equal(n.localNodeID) {
		n.transferer.Receive(packet)
		return

	} else if nextNodeID.IsNormal() || nextNodeID.Equal(&shared.NodeNeighborhoods) {
		if err := n.nodeAccessor.RelayPacket(nextNodeID, packet); err != nil {
			n.logger.Debug("failed to relay packet", slog.String("error", err.Error()))
			n.nodeAccessor.RelayPacket(nextNodeID, packet)
			return
		}
		return
	}

	n.logger.Debug("drop packet")
}
