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
	"math/rand"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/proto"
)

const (
	defaultConnectionTimeout = 10 * time.Second

	offerStatusSuccessAccept = iota
	offerStatusSuccessAlready

	offerStatusFailureConflict = iota
)

type Handler interface {
	NodeAccessorRecvPacket(*shared.NodeID, *shared.Packet)
	NodeAccessorChangeConnections(map[shared.NodeID]struct{})
}

type Config struct {
	Ctx         context.Context
	Logger      *slog.Logger
	LocalNodeID *shared.NodeID
	Handler     Handler
	Transferer  *transferer.Transferer

	// config parameters for testing
	connectionTimeout time.Duration
}

type NodeAccessor struct {
	config   *Config
	mtx      sync.RWMutex
	nlConfig *NodeLinkConfig

	// if enabled is false, node accessor does not connect any links.
	enabled          bool
	firstLink        *nodeLink
	randomLink       *nodeLink
	nodeID2link      map[shared.NodeID]*nodeLink
	link2nodeID      map[*nodeLink]*shared.NodeID
	connectingStates map[*nodeLink]*connectingState
}

type offerType int

const (
	offerTypeFirst offerType = iota
	offerTypeRandom
	offerTypeNormal
)

type connectingState struct {
	isBySeed          bool
	isPrime           bool
	ices              []string
	creationTimestamp time.Time
}

func NewNodeAccessor(config *Config) *NodeAccessor {
	if config.connectionTimeout == 0 {
		config.connectionTimeout = defaultConnectionTimeout
	}

	na := &NodeAccessor{
		config:           config,
		nodeID2link:      make(map[shared.NodeID]*nodeLink),
		link2nodeID:      make(map[*nodeLink]*shared.NodeID),
		connectingStates: make(map[*nodeLink]*connectingState),
	}

	transferer.SetRequestHandler[proto.PacketContent_SignalingOffer](config.Transferer, na.recvOffer)
	transferer.SetRequestHandler[proto.PacketContent_SignalingIce](config.Transferer, na.recvICE)

	return na
}

func (na *NodeAccessor) SetConfig(iceServers []config.ICEServer, nlConfig *NodeLinkConfig) error {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	if na.nlConfig != nil {
		return errors.New("node accessor already configured")
	}

	webrtcConfig, err := defaultWebRTCConfigFactory(iceServers)
	if err != nil {
		return fmt.Errorf("failed to create webRTCConfig: %w", err)
	}

	copyNlConfig := *nlConfig
	copyNlConfig.ctx = na.config.Ctx
	copyNlConfig.logger = na.config.Logger
	copyNlConfig.webrtcConfig = webrtcConfig

	na.nlConfig = &copyNlConfig

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-na.config.Ctx.Done():
				na.mtx.Lock()
				defer na.mtx.Unlock()
				for link := range na.link2nodeID {
					na.disconnectLink(link)
				}

				webrtcConfig.destruct()
				return

			case <-ticker.C:
				na.subRoutine()
			}
		}

	}()

	return nil
}

func (na *NodeAccessor) SetEnabled(enabled bool) {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	na.enabled = enabled
	if !enabled {
		for link := range na.link2nodeID {
			na.disconnectLink(link)
		}
	}
}

func (na *NodeAccessor) subRoutine() {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	// If both nodes are connected to each other at the same time,
	// the connection will be broken and started over. So a random number is inserted to shift the timing.
	if na.enabled && len(na.link2nodeID) == 0 && rand.Int()%10 == 0 {
		if err := na.connectFirstLink(); err != nil {
			na.config.Logger.Warn("failed to connect first link", slog.String("error", err.Error()))
		}
	}

	if err := na.houseKeeping(); err != nil {
		na.config.Logger.Warn("failed to house keeping", slog.String("error", err.Error()))
	}
}

func (na *NodeAccessor) ConnectLinks(requiredNodeIDs, keepNodeIDs map[shared.NodeID]struct{}) error {
	if !na.IsOnline() {
		return errors.New("node accessor should be online before connecting link")
	}

	na.mtx.Lock()
	defer na.mtx.Unlock()

	if !na.enabled {
		return errors.New("node accessor should be enabled before connecting link")
	}

	for nodeID := range requiredNodeIDs {
		if link, ok := na.nodeID2link[nodeID]; ok {
			if link.getLinkState() == nodeLinkStateDisabled {
				na.disconnectLink(link)
			} else {
				continue
			}
		}

		link, err := na.createLink(&nodeID, true, false, true)
		if err != nil {
			return err
		}

		go func(nodeID *shared.NodeID) {
			if err = na.sendOffer(link, na.config.LocalNodeID, nodeID, offerTypeNormal); err != nil {
				na.config.Logger.Error("failed to send WebRTC offer request", slog.String("error", err.Error()))

				na.mtx.Lock()
				defer na.mtx.Unlock()
				na.disconnectLink(link)
			}
		}(&nodeID)
	}

	for nodeID, link := range na.nodeID2link {
		idToKeep := false

		if _, ok := requiredNodeIDs[nodeID]; ok {
			idToKeep = true
		} else {
			if _, ok := keepNodeIDs[nodeID]; ok {
				idToKeep = true
			}
		}

		if !idToKeep {
			na.disconnectLink(link)
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

	go func(link *nodeLink) {
		err := na.sendOffer(link, na.config.LocalNodeID, na.config.LocalNodeID, offerTypeRandom)
		if err != nil {
			na.config.Logger.Error("failed to send WebRTC offer request to connect random link", slog.String("error", err.Error()))

			na.mtx.Lock()
			defer na.mtx.Unlock()
			na.disconnectLink(link)
		}
	}(na.randomLink)

	return nil
}

func (na *NodeAccessor) bindNodeID(link *nodeLink, nodeID *shared.NodeID) {
	if n := na.link2nodeID[link]; n != nil {
		panic("link is already bound to different nodeID")
	}

	na.link2nodeID[link] = nodeID

	if nodeID == nil {
		return
	}

	if _, ok := na.nodeID2link[*nodeID]; ok {
		panic("nodeID is already bound to different link")
	}
	na.nodeID2link[*nodeID] = link
}

func (na *NodeAccessor) connectFirstLink() error {
	var err error
	na.firstLink, err = na.createLink(nil, true, true, true)
	if err != nil {
		return err
	}

	go func(link *nodeLink) {
		err := na.sendOffer(link, na.config.LocalNodeID, na.config.LocalNodeID, offerTypeFirst)
		if err != nil {
			na.config.Logger.Error("failed to send WebRTC offer request to connect first link", slog.String("error", err.Error()))

			na.mtx.Lock()
			defer na.mtx.Unlock()
			na.disconnectLink(link)
		}
	}(na.firstLink)

	return nil
}

func (na *NodeAccessor) createLink(nodeID *shared.NodeID, createDC, bySeed, prime bool) (*nodeLink, error) {
	if nodeID != nil {
		if _, ok := na.nodeID2link[*nodeID]; ok {
			return nil, errors.New("link already exists when creating link")
		}
	}

	link, err := newNodeLink(na.nlConfig, na, createDC)
	if err != nil {
		return nil, fmt.Errorf("failed to create link: %w", err)
	}
	na.bindNodeID(link, nodeID)

	state := &connectingState{
		isBySeed:          bySeed,
		isPrime:           prime,
		ices:              make([]string, 0),
		creationTimestamp: time.Now(),
	}
	na.connectingStates[link] = state

	return link, nil
}

func (na *NodeAccessor) disconnectLink(link *nodeLink) {
	if err := link.disconnect(); err != nil {
		na.config.Logger.Error("failed to disconnect link", slog.String("error", err.Error()))
	}

	if link == na.firstLink {
		na.firstLink = nil
	}

	if link == na.randomLink {
		na.randomLink = nil
	}

	if nodeID := na.link2nodeID[link]; nodeID != nil {
		delete(na.nodeID2link, *nodeID)
		defer na.callChangeConnections()
	}

	delete(na.connectingStates, link)
	delete(na.link2nodeID, link)
}

func (na *NodeAccessor) houseKeeping() error {
	for link := range na.link2nodeID {
		if link.getLinkState() == nodeLinkStateDisabled {
			na.disconnectLink(link)
		}
	}

	for link, state := range na.connectingStates {
		if time.Now().After(state.creationTimestamp.Add(na.config.connectionTimeout)) {
			na.disconnectLink(link)
		}
	}

	return nil
}

func (na *NodeAccessor) IsOnline() bool {
	na.mtx.RLock()
	defer na.mtx.RUnlock()
	for _, link := range na.nodeID2link {
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

	na.mtx.RLock()
	defer na.mtx.RUnlock()

	if dstNodeID.Equal(&shared.NodeIDNext) {
		if (packet.Mode & shared.PacketModeOneWay) == shared.PacketModeNone {
			return errors.New("packet mode should be one way when multicast")
		}
		for nodeID, link := range na.nodeID2link {
			copy := *packet
			copy.DstNodeID = &nodeID
			if link.getLinkState() == nodeLinkStateOnline {
				if err := link.sendPacket(&copy); err != nil {
					return errors.New("failed to send packet")
				}
			}
		}

	} else {
		link, ok := na.nodeID2link[*dstNodeID]
		if !ok || link.getLinkState() != nodeLinkStateOnline {
			return errors.New("link is not online")
		}
		if err := link.sendPacket(packet); err != nil {
			return errors.New("failed to send packet")
		}
	}

	return nil
}

func (na *NodeAccessor) recvOffer(packet *shared.Packet) {
	content := packet.Content.GetSignalingOffer()
	primeNodeID := shared.NewNodeIDFromProto(content.GetPrimeNodeId())
	sdp := content.GetSdp()
	offerType := offerType(content.GetType())
	isBySeed := false
	if offerType == offerTypeFirst || offerType == offerTypeRandom {
		isBySeed = true
	}

	if primeNodeID.Equal(na.config.LocalNodeID) {
		// cancel random connection since there is on the same cluster
		if offerType == offerTypeRandom {
			na.config.Transferer.Cancel(packet)

			na.mtx.Lock()
			defer na.mtx.Unlock()
			if na.randomLink != nil {
				na.disconnectLink(na.randomLink)
			}

		} else {
			// send response to tell conflicting node id
			na.config.Transferer.Response(packet, &proto.PacketContent{
				Content: &proto.PacketContent_SignalingOfferFailure{
					SignalingOfferFailure: &proto.SignalingOfferFailure{
						Status:      offerStatusFailureConflict,
						PrimeNodeId: content.PrimeNodeId,
					},
				},
			})
		}
		return
	}

	na.mtx.Lock()
	defer na.mtx.Unlock()
	if link, ok := na.nodeID2link[*primeNodeID]; ok {
		if link.getLinkState() == nodeLinkStateOnline {
			// Already having a online connection with prime node yet.
			na.config.Transferer.Response(packet, &proto.PacketContent{
				Content: &proto.PacketContent_SignalingOfferSuccess{
					SignalingOfferSuccess: &proto.SignalingOfferSuccess{
						Status:       uint32(offerStatusSuccessAlready),
						SecondNodeId: na.config.LocalNodeID.Proto(),
					},
				},
			})

		} else {
			// Disconnect existing connection as it is unrelated.
			na.disconnectLink(link)

			// Recreate connection and response for the offer when prime node id is earlier then local.
			if primeNodeID.Smaller(na.config.LocalNodeID) {
				if err := na.acceptOffer(packet, primeNodeID, sdp, isBySeed); err != nil {
					na.config.Logger.Error("failed to accept offer", slog.String("error", err.Error()))
				}
			}
		}
	} else {
		if err := na.acceptOffer(packet, primeNodeID, sdp, isBySeed); err != nil {
			na.config.Logger.Error("failed to accept offer", slog.String("error", err.Error()))
		}
	}
}

func (na *NodeAccessor) acceptOffer(packet *shared.Packet, primeNodeID *shared.NodeID, remoteSDP string, isBySeed bool) error {
	link, err := na.createLink(primeNodeID, false, isBySeed, false)
	if err != nil {
		na.disconnectLink(link)
		return fmt.Errorf("failed to create link for the offer: %w", err)
	}

	if err := link.setRemoteSDP(remoteSDP); err != nil {
		na.disconnectLink(link)
		return fmt.Errorf("failed to set remove spd for the offer: %w", err)
	}

	localSDP, err := link.getLocalSDP()
	if err != nil {
		na.disconnectLink(link)
		return fmt.Errorf("failed to get local sdp for the offer: %w", err)
	}

	na.config.Transferer.Response(packet, &proto.PacketContent{
		Content: &proto.PacketContent_SignalingOfferSuccess{
			SignalingOfferSuccess: &proto.SignalingOfferSuccess{
				Status:       offerStatusSuccessAccept,
				SecondNodeId: na.config.LocalNodeID.Proto(),
				Sdp:          localSDP,
			},
		},
	})

	return nil
}

func (na *NodeAccessor) recvICE(packet *shared.Packet) {
	content := packet.Content.GetSignalingIce()
	localNodeID := shared.NewNodeIDFromProto(content.LocalNodeId)
	remoteNodeID := shared.NewNodeIDFromProto(content.RemoteNodeId)
	ices := content.Ices

	na.mtx.Lock()
	defer na.mtx.Unlock()

	link, ok := na.nodeID2link[*localNodeID]
	if !ok || !remoteNodeID.Equal(na.config.LocalNodeID) {
		return
	}

	for _, ice := range ices {
		if err := link.updateICE(ice); err != nil {
			na.config.Logger.Error("failed to update ICE", slog.String("error", err.Error()))
		}
	}

	state, ok := na.connectingStates[link]
	if !ok || len(state.ices) == 0 {
		return
	}

	if len(state.ices) != 0 {
		na.sendICE(link, state.ices)
	}
	state.ices = nil
}

type offerHandler struct {
	logger    *slog.Logger
	na        *NodeAccessor
	link      *nodeLink
	nodeID    *shared.NodeID
	offerType offerType
}

func (oh *offerHandler) OnResponse(packet *shared.Packet) {
	oh.na.mtx.Lock()
	defer oh.na.mtx.Unlock()

	if content := packet.Content.GetSignalingOfferSuccess(); content != nil {
		oh.onSuccess(content)
		return
	}

	if content := packet.Content.GetSignalingOfferFailure(); content != nil {
		oh.onFailure(content)
		return
	}

	// unexpected response
	oh.logger.Debug("unexpected response", slog.String("from", oh.nodeID.String()))
	oh.na.disconnectLink(oh.link)
}

func (oh *offerHandler) onSuccess(content *proto.SignalingOfferSuccess) {
	// if the link is already connected, do nothing.
	if content.Status == offerStatusSuccessAlready {
		return
	}

	// unexpected code detected.
	if content.Status != offerStatusSuccessAccept {
		oh.na.disconnectLink(oh.link)
	}

	secondNodeID := shared.NewNodeIDFromProto(content.SecondNodeId)
	remoteSDP := content.Sdp

	switch oh.offerType {
	case offerTypeFirst:
		if oh.na.firstLink != oh.link {
			oh.na.disconnectLink(oh.link)
			return
		}

		// avoid duplicate link when creating the first link
		if _, ok := oh.na.nodeID2link[*secondNodeID]; ok {
			oh.na.disconnectLink(oh.link)
			return
		}

		oh.na.bindNodeID(oh.link, secondNodeID)
		oh.na.firstLink = nil

	case offerTypeRandom:
		if oh.na.randomLink != oh.link {
			oh.na.disconnectLink(oh.link)
			return
		}

		// check duplicate
		if _, ok := oh.na.nodeID2link[*secondNodeID]; ok {
			oh.na.disconnectLink(oh.link)
			return
		}

		oh.na.bindNodeID(oh.link, secondNodeID)
		oh.na.randomLink = nil

	case offerTypeNormal:
		if _, ok := oh.na.nodeID2link[*secondNodeID]; !ok {
			oh.na.disconnectLink(oh.link)
			return
		}
	}

	oh.link.setRemoteSDP(remoteSDP)

	if state, ok := oh.na.connectingStates[oh.link]; ok {
		if len(state.ices) != 0 {
			oh.na.sendICE(oh.link, state.ices)
		}
		state.ices = nil
	}
}

func (oh *offerHandler) onFailure(content *proto.SignalingOfferFailure) {
	oh.na.disconnectLink(oh.link)

	switch content.Status {
	case offerStatusFailureConflict:
		oh.logger.Debug("conflict on offer", slog.String("from", oh.nodeID.String()))
		panic("implement method for dealing with conflict")
	}
}

func (oh *offerHandler) OnError(_ constants.PacketErrorCode, message string) {
	oh.na.mtx.Lock()
	defer oh.na.mtx.Unlock()

	oh.logger.Debug("error on offer", slog.String("message", message))
	oh.na.disconnectLink(oh.link)
}

func (na *NodeAccessor) sendOffer(link *nodeLink, primaryNID, secondaryNID *shared.NodeID, t offerType) error {
	sdp, err := link.getLocalSDP()
	if err != nil {
		return fmt.Errorf("failed to get SDP: %w", err)
	}

	oh := &offerHandler{
		logger:    na.config.Logger,
		na:        na,
		link:      link,
		nodeID:    secondaryNID,
		offerType: t,
	}

	mode := shared.PacketModeExplicit
	if t == offerTypeFirst || t == offerTypeRandom {
		mode = shared.PacketModeRelaySeed | shared.PacketModeNoRetry
	}

	na.config.Transferer.Request(secondaryNID, mode, &proto.PacketContent{
		Content: &proto.PacketContent_SignalingOffer{
			SignalingOffer: &proto.SignalingOffer{
				PrimeNodeId:  primaryNID.Proto(),
				SecondNodeId: secondaryNID.Proto(),
				Sdp:          sdp,
				Type:         uint32(t),
			},
		},
	}, oh)

	return nil
}

func (na *NodeAccessor) sendICE(link *nodeLink, ices []string) {
	nodeID, ok := na.link2nodeID[link]
	if !ok {
		panic("link is not bound to nodeID")
	}

	state, ok := na.connectingStates[link]
	if !ok {
		return
	}

	mode := shared.PacketModeExplicit
	if state.isBySeed {
		mode |= shared.PacketModeRelaySeed
		if !state.isPrime {
			mode |= shared.PacketModeResponse
		}
	}

	na.config.Transferer.RequestOneWay(nodeID, mode, &proto.PacketContent{
		Content: &proto.PacketContent_SignalingIce{
			SignalingIce: &proto.SignalingICE{
				LocalNodeId:  na.config.LocalNodeID.Proto(),
				RemoteNodeId: nodeID.Proto(),
				Ices:         ices,
			},
		},
	})
}

func (na *NodeAccessor) nodeLinkChangeState(link *nodeLink, state nodeLinkState) {
	go func() {
		na.mtx.Lock()
		defer na.mtx.Unlock()

		if state != nodeLinkStateConnecting {
			if _, ok := na.connectingStates[link]; ok {
				delete(na.connectingStates, link)
				na.callChangeConnections()
			}
		}

		if state == nodeLinkStateDisabled {
			na.disconnectLink(link)
		}
	}()
}

func (na *NodeAccessor) nodeLinkUpdateICE(link *nodeLink, ice string) {
	if link.getLinkState() != nodeLinkStateConnecting {
		return
	}

	go func() {
		na.mtx.Lock()
		defer na.mtx.Unlock()

		state, ok := na.connectingStates[link]
		if !ok {
			return
		}

		if state.ices != nil {
			state.ices = append(state.ices, ice)
			return
		}

		na.sendICE(link, []string{ice})
	}()
}

func (na *NodeAccessor) nodeLinkRecvPacket(link *nodeLink, packet *shared.Packet) {
	nodeID := na.link2nodeID[link]
	na.config.Handler.NodeAccessorRecvPacket(nodeID, packet)
}

func (na *NodeAccessor) callChangeConnections() {
	connections := make(map[shared.NodeID]struct{})
	for nodeID, link := range na.nodeID2link {
		if _, ok := na.connectingStates[link]; ok {
			continue
		}
		connections[nodeID] = struct{}{}
	}
	na.config.Handler.NodeAccessorChangeConnections(connections)
}
