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
	"github.com/llamerada-jp/colonio/internal/network/signal"
	"github.com/llamerada-jp/colonio/internal/shared"
)

const (
	defaultConnectionTimeout      = 10 * time.Second
	defaultNextConnectionInterval = 180 * time.Second
)

type Handler interface {
	NodeAccessorRecvPacket(*shared.NodeID, *shared.Packet)
	NodeAccessorChangeConnections(map[shared.NodeID]struct{})
	NodeAccessorSendSignalOffer(*shared.NodeID, *signal.Offer) error
	NodeAccessorSendSignalAnswer(*shared.NodeID, *signal.Answer) error
	NodeAccessorSendSignalICE(*shared.NodeID, *signal.ICE) error
}

type Config struct {
	Logger         *slog.Logger
	Handler        Handler
	ICEServers     []config.ICEServer
	NodeLinkConfig *NodeLinkConfig

	// config parameters for testing
	ConnectionTimeout      time.Duration
	NextConnectionInterval time.Duration
}

type NodeAccessor struct {
	logger                  *slog.Logger
	handler                 Handler
	nodeLinkConfig          *NodeLinkConfig
	localNodeID             *shared.NodeID
	connectionTimeout       time.Duration
	nextConnectionInterval  time.Duration
	nextConnectionTimestamp time.Time

	mtx sync.RWMutex
	// isAlone is from seedAccessor
	isAlone bool

	// a set of connected links
	nodeID2link map[shared.NodeID]*nodeLink
	link2nodeID map[*nodeLink]*shared.NodeID
	// a set of connecting link states
	offerID2state map[uint32]*connectingState
	link2offerID  map[*nodeLink]uint32
}

type connectingState struct {
	isPrime           bool
	icesMtx           sync.Mutex
	ices              []string
	link              *nodeLink
	nodeID            shared.NodeID
	creationTimestamp time.Time
}

func NewNodeAccessor(config *Config) (*NodeAccessor, error) {
	ct := defaultConnectionTimeout
	if config.ConnectionTimeout != 0 {
		ct = config.ConnectionTimeout
	}
	nci := defaultNextConnectionInterval
	if config.NextConnectionInterval != 0 {
		nci = config.NextConnectionInterval
	}

	webrtcConfig, err := defaultWebRTCConfigFactory(config.ICEServers)
	if err != nil {
		return nil, fmt.Errorf("failed to create webRTCConfig: %w", err)
	}

	nlc := *config.NodeLinkConfig
	na := &NodeAccessor{
		logger:                 config.Logger,
		handler:                config.Handler,
		nodeLinkConfig:         &nlc,
		connectionTimeout:      ct,
		nextConnectionInterval: nci,
		isAlone:                true,
		nodeID2link:            make(map[shared.NodeID]*nodeLink),
		link2nodeID:            make(map[*nodeLink]*shared.NodeID),
		offerID2state:          make(map[uint32]*connectingState),
		link2offerID:           make(map[*nodeLink]uint32),
	}

	na.nodeLinkConfig.webrtcConfig = webrtcConfig
	na.nodeLinkConfig.logger = config.Logger

	return na, nil
}

func (na *NodeAccessor) Start(ctx context.Context, localNodeID *shared.NodeID) {
	na.localNodeID = localNodeID
	na.nodeLinkConfig.ctx = ctx

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				na.mtx.Lock()
				defer na.mtx.Unlock()
				for link := range na.link2nodeID {
					na.disconnectLink(link, false)
				}

				if na.nodeLinkConfig.webrtcConfig != nil {
					na.nodeLinkConfig.webrtcConfig.destruct()
					na.nodeLinkConfig.webrtcConfig = nil
				}
				return

			case <-ticker.C:
				na.subRoutine()
			}
		}
	}()
}

func (na *NodeAccessor) SetBeAlone(isAlone bool) {
	na.mtx.Lock()
	defer na.mtx.Unlock()
	na.isAlone = isAlone
}

func (na *NodeAccessor) subRoutine() {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	if err := na.houseKeeping(); err != nil {
		na.logger.Warn("failed to house keeping", slog.String("error", err.Error()))
	}

	// do not connect if any other node does not exist
	if na.isAlone {
		return
	}

	// If both nodes are connected to each other at the same time,
	// the connection will be broken and started over. So a random number is inserted to change the timing.
	if len(na.link2nodeID) == 0 && len(na.offerID2state) == 0 && rand.Int()%10 == 0 {
		if err := na.connect(signal.OfferTypeNext, na.localNodeID); err != nil {
			na.logger.Warn("failed to connect first link", slog.String("error", err.Error()))
		}
	}

	// try to connect to next node for redundancy
	if len(na.link2nodeID) != 0 && time.Now().After(na.nextConnectionTimestamp.Add(time.Duration(na.nextConnectionInterval))) {
		na.nextConnectionTimestamp = time.Now()
		if err := na.connect(signal.OfferTypeNext, na.localNodeID); err != nil {
			na.logger.Warn("failed to connect next link", slog.String("error", err.Error()))
		}
	}
}

func (na *NodeAccessor) ConnectLinks(requiredNodeIDs, keepNodeIDs map[shared.NodeID]struct{}) error {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	for nodeID := range requiredNodeIDs {
		if link, ok := na.nodeID2link[nodeID]; ok {
			if link.getLinkState() == nodeLinkStateDisabled {
				na.disconnectLink(link, false)
			} else {
				continue
			}
		}

		err := na.connect(signal.OfferTypeExplicit, &nodeID)
		if err != nil {
			return err
		}
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
			na.disconnectLink(link, false)
		}
	}

	return nil
}

func (na *NodeAccessor) SignalingOffer(srcNodeID *shared.NodeID, offer *signal.Offer) {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	offerID := offer.OfferID

	if state, ok := na.offerID2state[offerID]; ok && !state.nodeID.Equal(srcNodeID) {
		na.sendAnswer(srcNodeID, offerID, signal.AnswerStatusReject, "")
		return
	}

	if na.assignedNodeID(srcNodeID) {
		na.sendAnswer(srcNodeID, offerID, signal.AnswerStatusReject, "")
		return
	}

	if _, ok := na.nodeID2link[*srcNodeID]; ok {
		na.sendAnswer(srcNodeID, offerID, signal.AnswerStatusReject, "")
		return
	}

	if _, ok := na.offerID2state[offerID]; ok {
		na.sendAnswer(srcNodeID, offerID, signal.AnswerStatusReject, "")
		return
	}

	link, err := newNodeLink(na.nodeLinkConfig, na, false)
	if err != nil {
		na.logger.Error("failed to create link", slog.String("error", err.Error()))
		return
	}
	na.nodeID2link[*srcNodeID] = link
	na.link2nodeID[link] = srcNodeID
	na.offerID2state[offerID] = &connectingState{
		isPrime:           false,
		ices:              make([]string, 0),
		link:              link,
		nodeID:            *srcNodeID,
		creationTimestamp: time.Now(),
	}
	na.link2offerID[link] = offerID

	go func() {
		if err := link.setRemoteSDP(offer.Sdp); err != nil {
			na.logger.Error("failed to set remote SDP", slog.String("error", err.Error()))
			na.disconnectLink(link, true)
			return
		}

		sdp, err := link.getLocalSDP()
		if err != nil {
			na.logger.Error("failed to get local SDP", slog.String("error", err.Error()))
			na.disconnectLink(link, true)
			return
		}

		if err = na.sendAnswer(srcNodeID, offerID, signal.AnswerStatusAccept, sdp); err != nil {
			na.logger.Error("failed to send answer", slog.String("error", err.Error()))
			na.disconnectLink(link, true)
		}
	}()
}

func (na *NodeAccessor) SignalingAnswer(srcNodeID *shared.NodeID, answer *signal.Answer) {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	offerID := answer.OfferID
	state, ok := na.offerID2state[offerID]
	if !ok {
		return
	}

	if state.nodeID.Equal(&shared.NodeIDNone) {
		state.nodeID = *srcNodeID
	} else if !state.nodeID.Equal(srcNodeID) {
		return
	}

	if answer.Status == signal.AnswerStatusReject {
		na.disconnectLink(state.link, false)
		return
	}

	if _, ok := na.nodeID2link[*srcNodeID]; ok {
		na.disconnectLink(state.link, false)
		return
	}

	na.nodeID2link[*srcNodeID] = state.link
	na.link2nodeID[state.link] = srcNodeID

	go func() {
		if err := state.link.setRemoteSDP(answer.Sdp); err != nil {
			na.logger.Error("failed to set remote SDP", slog.String("error", err.Error()))
			return
		}

		state.icesMtx.Lock()
		defer state.icesMtx.Unlock()
		if len(state.ices) != 0 {
			na.sendICE(srcNodeID, offerID, state.ices)
		}
		state.ices = nil
	}()
}

func (na *NodeAccessor) SignalingICE(srcNodeID *shared.NodeID, ice *signal.ICE) {
	na.mtx.Lock()
	defer na.mtx.Unlock()

	offerID := ice.OfferID
	state, ok := na.offerID2state[offerID]
	if !ok {
		return
	}

	go func() {
		for _, i := range ice.Ices {
			if err := state.link.updateICE(i); err != nil {
				na.logger.Error("failed to update ICE", slog.String("error", err.Error()))
				break
			}
		}

		state.icesMtx.Lock()
		defer state.icesMtx.Unlock()
		if len(state.ices) != 0 {
			na.sendICE(srcNodeID, offerID, state.ices)
		}
		state.ices = nil
	}()
}

func (na *NodeAccessor) connect(ot signal.OfferType, nodeID *shared.NodeID) error {
	if ot == signal.OfferTypeExplicit && na.assignedNodeID(nodeID) {
		return errors.New("node ID is already connected")
	}

	link, err := newNodeLink(na.nodeLinkConfig, na, true)
	if err != nil {
		return fmt.Errorf("failed to create link: %w", err)
	}

	state := &connectingState{
		isPrime:           true,
		ices:              make([]string, 0),
		link:              link,
		creationTimestamp: time.Now(),
	}
	if ot == signal.OfferTypeExplicit {
		state.nodeID = *nodeID
	} else {
		state.nodeID = shared.NodeIDNone
	}
	// assign unique offerID
	offerID := rand.Uint32()
	for _, ok := na.offerID2state[offerID]; ok; {
		offerID = rand.Uint32()
	}

	na.offerID2state[offerID] = state
	na.link2offerID[link] = offerID

	go func() {
		sdp, err := link.getLocalSDP()
		if err != nil {
			na.logger.Error("failed to get local SDP", slog.String("error", err.Error()))
			na.disconnectLink(link, true)
			return
		}

		err = na.handler.NodeAccessorSendSignalOffer(nodeID, &signal.Offer{
			OfferID:   offerID,
			OfferType: ot,
			Sdp:       sdp,
		})
		if err != nil {
			na.logger.Error("failed to send offer", slog.String("error", err.Error()))
			na.disconnectLink(link, true)
		}
	}()

	return nil
}

func (na *NodeAccessor) assignedNodeID(nodeID *shared.NodeID) bool {
	if _, ok := na.nodeID2link[*nodeID]; ok {
		return true
	}

	for _, state := range na.offerID2state {
		if state.nodeID.Equal(nodeID) {
			return true
		}
	}

	return false
}

func (na *NodeAccessor) disconnectLink(link *nodeLink, lock bool) {
	if err := link.disconnect(); err != nil {
		na.logger.Error("failed to disconnect link", slog.String("error", err.Error()))
	}

	if lock {
		na.mtx.Lock()
		defer na.mtx.Unlock()
	}

	if nodeID := na.link2nodeID[link]; nodeID != nil {
		delete(na.nodeID2link, *nodeID)
		delete(na.link2nodeID, link)
		na.callChangeConnections()
	}

	if offerID, ok := na.link2offerID[link]; ok {
		delete(na.offerID2state, offerID)
		delete(na.link2offerID, link)
	}
}

func (na *NodeAccessor) houseKeeping() error {
	for link := range na.link2nodeID {
		if link.getLinkState() == nodeLinkStateDisabled {
			na.disconnectLink(link, false)
		}
	}

	for _, state := range na.offerID2state {
		if time.Now().After(state.creationTimestamp.Add(na.connectionTimeout)) {
			na.disconnectLink(state.link, false)
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

func (na *NodeAccessor) sendAnswer(dstNodeID *shared.NodeID, offerID uint32, status signal.AnswerStatus, sdp string) error {
	return na.handler.NodeAccessorSendSignalAnswer(dstNodeID, &signal.Answer{
		OfferID: offerID,
		Status:  status,
		Sdp:     sdp,
	})
}

func (na *NodeAccessor) sendICE(dstNodeID *shared.NodeID, offerID uint32, ices []string) error {
	return na.handler.NodeAccessorSendSignalICE(dstNodeID, &signal.ICE{
		OfferID: offerID,
		Ices:    ices,
	})
}

func (na *NodeAccessor) nodeLinkChangeState(link *nodeLink, state nodeLinkState) {
	go func() {
		na.mtx.Lock()
		defer na.mtx.Unlock()

		if state == nodeLinkStateOnline {
			offerID, ok := na.link2offerID[link]
			if ok {
				delete(na.offerID2state, offerID)
				delete(na.link2offerID, link)
				na.callChangeConnections()
			}
		}

		if state == nodeLinkStateDisabled {
			na.disconnectLink(link, false)
		}
	}()
}

func (na *NodeAccessor) nodeLinkUpdateICE(link *nodeLink, ice string) {
	go func() {
		na.mtx.Lock()
		defer na.mtx.Unlock()

		offerID, ok := na.link2offerID[link]
		if !ok {
			return
		}

		state := na.offerID2state[offerID]
		state.icesMtx.Lock()
		defer state.icesMtx.Unlock()
		if state.ices == nil {
			na.sendICE(&state.nodeID, offerID, []string{ice})
		} else {
			state.ices = append(state.ices, ice)
		}
	}()
}

func (na *NodeAccessor) nodeLinkRecvPacket(link *nodeLink, packet *shared.Packet) {
	na.mtx.RLock()
	nodeID := na.link2nodeID[link]
	na.mtx.RUnlock()
	na.handler.NodeAccessorRecvPacket(nodeID, packet)
}

func (na *NodeAccessor) callChangeConnections() {
	connections := make(map[shared.NodeID]struct{})
	for nodeID := range na.nodeID2link {
		connections[nodeID] = struct{}{}
	}
	na.handler.NodeAccessorChangeConnections(connections)
}
