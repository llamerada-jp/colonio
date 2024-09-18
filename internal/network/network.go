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
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/network/node_accessor"
	"github.com/llamerada-jp/colonio/internal/network/routing"
	"github.com/llamerada-jp/colonio/internal/network/seed_accessor"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type NetworkConfig struct {
	Ctx    context.Context
	Logger *slog.Logger
	// Allow insecure server connections. You must not use this option in production. Only for testing.
	Insecure bool
	// Interval between retries when a network error occurs.
	SeedTripInterval time.Duration
}

type Network struct {
	config       *NetworkConfig
	localNID     *shared.NodeID
	routing      *routing.Routing
	seedAccessor *seed_accessor.SeedAccessor
	nodeAccessor *node_accessor.NodeAccessor
	transferer   *transferer.Transferer
}

func NewNetwork(config *NetworkConfig) *Network {
	return &Network{
		config: config,
	}
}

func (n *Network) Connect(url string, token []byte) error {
	if n.localNID != nil {
		return errors.New("network already connected")
	}

	n.localNID = shared.NewRandomNodeID()
	n.transferer = transferer.NewTransferer(&transferer.Config{
		Ctx:      n.config.Ctx,
		Logger:   n.config.Logger,
		LocalNID: n.localNID,
	})
	n.routing = routing.NewRouting(&routing.Config{})
	n.seedAccessor = seed_accessor.NewSeedAccessor(&seed_accessor.Config{
		Ctx:    n.config.Ctx,
		Logger: n.config.Logger,
		Transporter: seed_accessor.DefaultSeedTransporterFactory(&seed_accessor.SeedTransporterOption{
			Verification: !n.config.Insecure,
		}),
		EventHandler: n,
		URL:          url,
		LocalNID:     n.localNID,
		Token:        token,
		TripInterval: n.config.SeedTripInterval,
		// TODO: add seedAccessor event handler
	})
	n.nodeAccessor = node_accessor.NewNodeAccessor(&node_accessor.Config{
		Ctx:      n.config.Ctx,
		Logger:   n.config.Logger,
		LocalNID: n.localNID,
	})

	return nil
}

func (n *Network) Disconnect() error {
	panic("not implemented")
}

func (n *Network) IsOnline() bool {
	panic("not implemented")
}

func (n *Network) SeedAuthorizeFailed() {
	panic("not implemented")
}

func (n *Network) SeedChangeState() {
	panic("not implemented")
}

func (n *Network) SeedRecvConfig(clusterConfig *config.Cluster) {
	// TODO: receive config twice and those are different, suggest to rerun the network.
	n.nodeAccessor.SetConfig(clusterConfig.IceServers, &node_accessor.NodeLinkConfig{
		SessionTimeout:    clusterConfig.SessionTimeout,
		KeepaliveInterval: clusterConfig.KeepaliveInterval,
		BufferInterval:    clusterConfig.BufferInterval,
		PacketBaseBytes:   clusterConfig.WebRTCPacketBaseBytes,
	})
}

func (n *Network) SeedRecvPacket(*shared.Packet) {
	panic("not implemented")
}

func (n *Network) SeedRequireRandomConnect() {
	panic("not implemented")
}
