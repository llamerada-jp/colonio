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
package node_accessor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Handler interface {
}

type Config struct {
	Ctx      context.Context
	Logger   *slog.Logger
	LocalNID *shared.NodeID
	Handler  Handler
}

type NodeAccessor struct {
	config   *Config
	mtx      sync.RWMutex
	nlConfig *nodeLinkConfig

	// if enabled is false, node accessor does not connect any links.
	enabled          bool
	firstLink        *nodeLink
	randomLink       *nodeLink
	links            map[shared.NodeID]*nodeLink
	connectingStates map[*nodeLink]*connectingState
}

type offerType int

const (
	offerTypeFirst offerType = iota
	offerTypeRandom
	offerTypeNormal
)

type connectingState struct {
	isBySeed bool
	isPrime  bool
}

func NewNodeAccessor(config *Config) *NodeAccessor {
	return &NodeAccessor{
		config:           config,
		links:            make(map[shared.NodeID]*nodeLink),
		connectingStates: make(map[*nodeLink]*connectingState),
	}
}

func (na *NodeAccessor) SetConfig(clusterConfig *config.Cluster) error {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	if na.nlConfig != nil {
		return errors.New("node accessor already configured")
	}

	webrtcConfig, err := defaultWebRTCConfigFactory(clusterConfig.IceServers)
	if err != nil {
		return fmt.Errorf("failed to create webRTCConfig: %w", err)
	}

	na.nlConfig = &nodeLinkConfig{
		ctx:               na.config.Ctx,
		logger:            na.config.Logger,
		webrtcConfig:      webrtcConfig,
		sessionTimeout:    clusterConfig.SessionTimeout,
		keepaliveInterval: clusterConfig.KeepaliveInterval,
		bufferInterval:    clusterConfig.BufferInterval,
		packetBaseBytes:   clusterConfig.WebRTCPacketBaseBytes,
	}

	go na.subRoutine()

	return nil
}

func (na *NodeAccessor) SetEnabled(enabled bool) {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	na.enabled = enabled
}

func (na *NodeAccessor) subRoutine() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-na.config.Ctx.Done():
			return

		case <-ticker.C:
			na.mtx.Lock()
			if na.enabled && len(na.links) == 0 && na.firstLink == nil {
				if err := na.connectFirstLink(); err != nil {
					na.config.Logger.Warn("failed to connect first link", slog.String("error", err.Error()))
				}
			}

			if err := na.houseKeeping(); err != nil {
				na.config.Logger.Warn("failed to house keeping", slog.String("error", err.Error()))
			}
			na.mtx.Unlock()
		}
	}
}

func (na *NodeAccessor) ConnectLinks(nodeIDs []*shared.NodeID) error {
	if !na.IsOnline() {
		return errors.New("node accessor should be online before connecting link")
	}

	na.mtx.Lock()
	defer na.mtx.Unlock()

	if !na.enabled {
		return errors.New("node accessor should be enabled before connecting link")
	}

	for _, nodeID := range nodeIDs {
		if link, ok := na.links[*nodeID]; ok {
			if link.getLinkState() == nodeLinkStateDisabled {
				na.disconnectLinkByID(nodeID)
			} else {
				continue
			}
		}

		link, err := na.createLink(nodeID, true, false, true)
		if err != nil {
			return err
		}

		if err = na.sendOffer(link, na.config.LocalNID, nodeID, offerTypeNormal); err != nil {
			return err
		}
	}

	for nodeID := range na.links {
		idToKeep := false
		for _, targetNodeID := range nodeIDs {
			if nodeID == *targetNodeID {
				idToKeep = true
				break
			}
		}
		if !idToKeep {
			if err := na.disconnectLinkByID(&nodeID); err != nil {
				return err
			}
		}
	}

	return nil
}

func (na *NodeAccessor) ConnectRandomLink() error {
	if !na.IsOnline() {
		return errors.New("node accessor should be online before connecting link")
	}

	na.mtx.Lock()
	defer na.mtx.Unlock()

	if !na.enabled {
		return errors.New("node accessor should be enabled before connecting link")
	}

	if na.randomLink != nil {
		return nil
	}

	var err error
	na.randomLink, err = na.createLink(nil, true, true, true)
	if err != nil {
		return err
	}

	return na.sendOffer(na.randomLink, na.config.LocalNID, na.config.LocalNID, offerTypeRandom)
}

func (na *NodeAccessor) connectFirstLink() error {
	var err error
	na.firstLink, err = na.createLink(nil, true, true, true)
	if err != nil {
		return err
	}

	return na.sendOffer(na.firstLink, na.config.LocalNID, na.config.LocalNID, offerTypeFirst)
}

func (na *NodeAccessor) createLink(nodeID *shared.NodeID, createDC, bySeed, prime bool) (*nodeLink, error) {
	if _, ok := na.links[*nodeID]; ok {
		return nil, errors.New("link already exists when creating link")
	}

	// TODO: Implement nodeLinkEventHandler
	link, err := newNodeLink(na.nlConfig, na, createDC)
	if err != nil {
		return nil, fmt.Errorf("failed to create link: %w", err)
	}
	if nodeID != nil {
		na.links[*nodeID] = link
	}

	state := &connectingState{
		isBySeed: bySeed,
		isPrime:  prime,
	}
	na.connectingStates[link] = state

	return link, nil
}

func (na *NodeAccessor) DisconnectAll() error {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	if na.firstLink != nil {
		if err := na.disconnectLink(na.firstLink); err != nil {
			return err
		}
		na.firstLink = nil
	}

	if na.randomLink != nil {
		if err := na.disconnectLink(na.randomLink); err != nil {
			return err
		}
		na.randomLink = nil
	}

	for nodeID := range na.links {
		if err := na.disconnectLinkByID(&nodeID); err != nil {
			return err
		}
	}
	return nil
}

func (na *NodeAccessor) disconnectLinkByID(nodeID *shared.NodeID) error {
	link, ok := na.links[*nodeID]
	if !ok {
		return nil
	}

	if err := na.disconnectLink(link); err != nil {
		return err
	}

	delete(na.links, *nodeID)

	return nil
}

func (na *NodeAccessor) disconnectLink(link *nodeLink) error {
	if err := link.disconnect(); err != nil {
		return fmt.Errorf("failed to disconnect link: %w", err)
	}

	delete(na.connectingStates, link)

	return nil
}

func (na *NodeAccessor) houseKeeping() error {
	panic("not implemented")
}

func (na *NodeAccessor) IsOnline() bool {
	na.mtx.RLock()
	defer na.mtx.RUnlock()
	for _, link := range na.links {
		if link.getLinkState() == nodeLinkStateOnline {
			return true
		}
	}

	return false
}

func (na *NodeAccessor) RelayPacket(dstNodeID *shared.NodeID, packet *shared.Packet) error {
	if !na.IsOnline() {
		return errors.New("node accessor should be online before relaying packet")
	}

	if dstNodeID.Equal(&shared.NodeIDNext) {
		if (packet.Mode & shared.PacketModeOneWay) == shared.PacketModeNone {
			return errors.New("packet mode should be one way when multicast")
		}
		for _, link := range na.links {
			if link.getLinkState() == nodeLinkStateOnline {
				if err := link.sendPacket(packet); err != nil {
					return errors.New("failed to send packet")
				}
			}
		}

	} else {
		link, ok := na.links[*dstNodeID]
		if !ok || link.getLinkState() != nodeLinkStateOnline {
			return errors.New("link is not online")
		}
		if err := link.sendPacket(packet); err != nil {
			return errors.New("failed to send packet")
		}
	}

	return nil
}

func (na *NodeAccessor) sendOffer(link *nodeLink, primaryNID, secondaryNID *shared.NodeID, t offerType) error {
	panic("not implemented")
}

func (na *NodeAccessor) nodeLinkChangeState(*nodeLink, nodeLinkState) {
	panic("not implemented")
}

func (na *NodeAccessor) nodeLinkUpdateICE(string) {
	panic("not implemented")
}

func (na *NodeAccessor) nodeLinkRectPacket(*shared.Packet) {
	panic("not implemented")
}
