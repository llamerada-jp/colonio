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
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/network/signal"
	"github.com/llamerada-jp/colonio/internal/shared"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var iceServers = []config.ICEServer{
	{
		URLs: []string{"stun:stun.l.google.com:19302"},
	},
}

var nlConfig = &NodeLinkConfig{
	SessionTimeout:    5 * time.Second,
	KeepaliveInterval: 1 * time.Second,
	BufferInterval:    10 * time.Millisecond,
	PacketBaseBytes:   1024,
}

type nodeAccessorHandlerHelper struct {
	recvPacket        func(*shared.NodeID, *shared.Packet)
	changeConnections func(map[shared.NodeID]struct{})
	sendSignalOffer   func(*shared.NodeID, *signal.Offer) error
	sendSignalAnswer  func(*shared.NodeID, *signal.Answer) error
	sendSignalICE     func(*shared.NodeID, *signal.ICE) error
}

var _ Handler = &nodeAccessorHandlerHelper{}

func (h *nodeAccessorHandlerHelper) NodeAccessorRecvPacket(n *shared.NodeID, p *shared.Packet) {
	if h.recvPacket != nil {
		h.recvPacket(n, p)
	}
}

func (h *nodeAccessorHandlerHelper) NodeAccessorChangeConnections(c map[shared.NodeID]struct{}) {
	if h.changeConnections != nil {
		h.changeConnections(c)
	}
}

func (h *nodeAccessorHandlerHelper) NodeAccessorSendSignalOffer(n *shared.NodeID, o *signal.Offer) error {
	if h.sendSignalOffer != nil {
		return h.sendSignalOffer(n, o)
	}
	return nil
}

func (h *nodeAccessorHandlerHelper) NodeAccessorSendSignalAnswer(n *shared.NodeID, a *signal.Answer) error {
	if h.sendSignalAnswer != nil {
		return h.sendSignalAnswer(n, a)
	}
	return nil
}

func (h *nodeAccessorHandlerHelper) NodeAccessorSendSignalICE(n *shared.NodeID, i *signal.ICE) error {
	if h.sendSignalICE != nil {
		return h.sendSignalICE(n, i)
	}
	return nil
}

type envelope struct {
	dstNodeID *shared.NodeID
	offer     *signal.Offer
	answer    *signal.Answer
	ice       *signal.ICE
}

func relay(envelope *envelope, srcNodeID *shared.NodeID, nodeIDs []*shared.NodeID, accessors []*NodeAccessor) error {
	dstNodeID := envelope.dstNodeID
	if envelope.offer != nil && envelope.offer.OfferType == signal.OfferTypeNext {
		minNodeID := dstNodeID
		for _, nodeID := range nodeIDs {
			if nodeID.Compare(minNodeID) < 0 {
				minNodeID = nodeID
			}
			if nodeID.Compare(envelope.dstNodeID) > 0 {
				if dstNodeID.Equal(envelope.dstNodeID) {
					dstNodeID = nodeID
				} else if nodeID.Compare(dstNodeID) < 0 {
					dstNodeID = nodeID
				}
			}
		}
		if dstNodeID.Equal(envelope.dstNodeID) {
			dstNodeID = minNodeID
		}
	}
	for i, nodeID := range nodeIDs {
		if nodeID.Equal(dstNodeID) {
			ac := accessors[i]
			if envelope.offer != nil {
				go ac.SignalingOffer(srcNodeID, envelope.offer)
			} else if envelope.answer != nil {
				go ac.SignalingAnswer(srcNodeID, envelope.answer)
			} else if envelope.ice != nil {
				go ac.SignalingICE(srcNodeID, envelope.ice)
			}
			return nil
		}
	}
	return nil
}

func TestNodeAccessorAlone(t *testing.T) {
	localNodeID := shared.NewRandomNodeID()

	mtx := sync.Mutex{}
	connections := make(map[shared.NodeID]struct{})
	envelopes := make([]*envelope, 0)

	na, err := NewNodeAccessor(&Config{
		Logger: slog.Default(),
		Handler: &nodeAccessorHandlerHelper{
			recvPacket: func(_ *shared.NodeID, _ *shared.Packet) {
				assert.Fail(t, "unexpected call")
			},
			changeConnections: func(c map[shared.NodeID]struct{}) {
				mtx.Lock()
				defer mtx.Unlock()
				connections = c
			},
			sendSignalOffer: func(dstNodeID *shared.NodeID, offer *signal.Offer) error {
				mtx.Lock()
				defer mtx.Unlock()
				envelopes = append(envelopes, &envelope{
					dstNodeID: dstNodeID,
					offer:     offer,
				})
				return nil
			},
			sendSignalAnswer: func(dstNodeID *shared.NodeID, answer *signal.Answer) error {
				mtx.Lock()
				defer mtx.Unlock()
				envelopes = append(envelopes, &envelope{
					dstNodeID: dstNodeID,
					answer:    answer,
				})
				return nil
			},
			sendSignalICE: func(dstNodeID *shared.NodeID, ice *signal.ICE) error {
				mtx.Lock()
				defer mtx.Unlock()
				envelopes = append(envelopes, &envelope{
					dstNodeID: dstNodeID,
					ice:       ice,
				})
				return nil
			},
		},
		ICEServers:        iceServers,
		NodeLinkConfig:    nlConfig,
		ConnectionTimeout: 1 * time.Second,
	})
	require.NoError(t, err)
	na.Start(t.Context(), localNodeID)

	// keeping turn off (there is only one node in the cluster)
	na.SetBeAlone(true)
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		mtx.Lock()
		require.Len(t, connections, 0)
		require.Len(t, envelopes, 0)
		mtx.Unlock()
	}

	// turn on (is not only one node in the cluster)
	na.SetBeAlone(false)
	require.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(envelopes) >= 1 && len(connections) == 0
	}, 60*time.Second, 100*time.Millisecond)

	mtx.Lock()
	received := envelopes[0]
	mtx.Unlock()
	assert.True(t, localNodeID.Equal(received.dstNodeID))
	assert.NotNil(t, received.offer)
	assert.Equal(t, received.offer.OfferType, signal.OfferTypeNext)

	na.mtx.Lock()
	assert.Len(t, na.nodeID2link, 0)
	assert.Len(t, na.link2nodeID, 0)
	assert.Len(t, na.offerID2state, 1)
	assert.Len(t, na.link2offerID, 1)
	na.mtx.Unlock()
	assert.Contains(t, na.offerID2state, received.offer.OfferID)

	// wait for cutting off the connection
	na.SetBeAlone(true)
	require.Eventually(t, func() bool {
		na.mtx.Lock()
		defer na.mtx.Unlock()
		return len(na.offerID2state) == 0 && len(na.link2offerID) == 0
	}, 60*time.Second, 100*time.Millisecond)
}

func TestNodeAccessorPair(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)

	nodeAccessors := make([]*NodeAccessor, 0)
	mtx := sync.Mutex{}
	packets := make([][]*shared.Packet, 2)
	connections := make([]map[shared.NodeID]struct{}, 2)

	for i, srcNodeID := range nodeIDs {
		i := i
		packets[i] = make([]*shared.Packet, 0)

		na, err := NewNodeAccessor(&Config{
			Logger: slog.Default(),
			Handler: &nodeAccessorHandlerHelper{
				recvPacket: func(n *shared.NodeID, p *shared.Packet) {
					mtx.Lock()
					defer mtx.Unlock()
					packets[i] = append(packets[i], p)
				},
				changeConnections: func(c map[shared.NodeID]struct{}) {
					mtx.Lock()
					defer mtx.Unlock()
					connections[i] = c
				},
				sendSignalOffer: func(dstNodeID *shared.NodeID, offer *signal.Offer) error {
					return relay(&envelope{
						dstNodeID: dstNodeID,
						offer:     offer,
					}, srcNodeID, nodeIDs, nodeAccessors)
				},
				sendSignalAnswer: func(dstNodeID *shared.NodeID, answer *signal.Answer) error {
					return relay(&envelope{
						dstNodeID: dstNodeID,
						answer:    answer,
					}, srcNodeID, nodeIDs, nodeAccessors)
				},
				sendSignalICE: func(dstNodeID *shared.NodeID, ice *signal.ICE) error {
					return relay(&envelope{
						dstNodeID: dstNodeID,
						ice:       ice,
					}, srcNodeID, nodeIDs, nodeAccessors)
				},
			},
			ICEServers:     iceServers,
			NodeLinkConfig: nlConfig,
		})
		require.NoError(t, err)
		nodeAccessors = append(nodeAccessors, na)
	}

	// make online
	for i, na := range nodeAccessors {
		na.Start(t.Context(), nodeIDs[i])
		na.SetBeAlone(false)
	}

	// wait for detecting each other
	require.Eventually(t, func() bool {
		for _, na := range nodeAccessors {
			if !na.IsOnline() {
				return false
			}
		}
		return true
	}, 60*time.Second, 100*time.Millisecond)

	for _, na := range nodeAccessors {
		na.mtx.Lock()
		assert.Len(t, na.nodeID2link, 1)
		assert.Len(t, na.link2nodeID, 1)
		assert.Len(t, na.offerID2state, 0)
		assert.Len(t, na.link2offerID, 0)
		na.mtx.Unlock()
	}

	mtx.Lock()
	for _, c := range connections {
		assert.Len(t, c, 1)
	}
	mtx.Unlock()

	// send origPacket
	origPacket := &shared.Packet{
		DstNodeID: shared.NewRandomNodeID(),
		SrcNodeID: shared.NewRandomNodeID(),
		ID:        999,
		HopCount:  0,
		Mode:      shared.PacketModeNoRetry,
		Content: &proto.PacketContent{
			Content: &proto.PacketContent_Error{
				Error: &proto.Error{
					Code:    10,
					Message: "test",
				},
			},
		},
	}
	nodeAccessors[0].RelayPacket(nodeIDs[1], origPacket)

	// wait for receiving the packet
	require.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(packets[1]) == 1
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	receivedPacket := packets[1][0]
	mtx.Unlock()
	assert.True(t, origPacket.DstNodeID.Equal(receivedPacket.DstNodeID))
	assert.True(t, origPacket.SrcNodeID.Equal(receivedPacket.SrcNodeID))
	assert.Equal(t, origPacket.ID, receivedPacket.ID)
	assert.Equal(t, origPacket.HopCount, receivedPacket.HopCount)
	assert.Equal(t, origPacket.Mode, receivedPacket.Mode)
	assert.Equal(t, origPacket.Content.GetError().Code, receivedPacket.Content.GetError().Code)
	assert.Equal(t, origPacket.Content.GetError().Message, receivedPacket.Content.GetError().Message)
}

func TestNodeAccessorMany(t *testing.T) {
	for n := 3; n <= 5; n++ {
		t.Run(fmt.Sprintf("nodes(%d)", n), func(t *testing.T) {
			testNodeAccessorMany(t, n)
		})
	}
}

func testNodeAccessorMany(t *testing.T, n int) {
	nodeIDs := testUtil.UniqueNodeIDs(n)
	slices.SortFunc(nodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})

	nodeAccessors := make([]*NodeAccessor, 0)
	mtx := sync.Mutex{}
	packets := make([][]*shared.Packet, n)
	connections := make([]map[shared.NodeID]struct{}, n)

	for i, srcNodeID := range nodeIDs {
		i := i
		packets[i] = make([]*shared.Packet, 0)

		na, err := NewNodeAccessor(&Config{
			Logger: slog.Default(),
			Handler: &nodeAccessorHandlerHelper{
				recvPacket: func(n *shared.NodeID, p *shared.Packet) {
					assert.False(t, srcNodeID.Equal(n))
					mtx.Lock()
					defer mtx.Unlock()
					packets[i] = append(packets[i], p)
				},
				changeConnections: func(c map[shared.NodeID]struct{}) {
					mtx.Lock()
					defer mtx.Unlock()
					connections[i] = c
				},
				sendSignalOffer: func(dstNodeID *shared.NodeID, offer *signal.Offer) error {
					return relay(&envelope{
						dstNodeID: dstNodeID,
						offer:     offer,
					}, srcNodeID, nodeIDs, nodeAccessors)
				},
				sendSignalAnswer: func(dstNodeID *shared.NodeID, answer *signal.Answer) error {
					return relay(&envelope{
						dstNodeID: dstNodeID,
						answer:    answer,
					}, srcNodeID, nodeIDs, nodeAccessors)
				},
				sendSignalICE: func(dstNodeID *shared.NodeID, ice *signal.ICE) error {
					return relay(&envelope{
						dstNodeID: dstNodeID,
						ice:       ice,
					}, srcNodeID, nodeIDs, nodeAccessors)
				},
			},
			ICEServers:             iceServers,
			NodeLinkConfig:         nlConfig,
			NextConnectionInterval: 5 * time.Second,
		})
		require.NoError(t, err)
		nodeAccessors = append(nodeAccessors, na)
	}

	// make online
	for i, na := range nodeAccessors {
		na.SetBeAlone(false)
		na.Start(t.Context(), nodeIDs[i])
	}

	// wait for detecting each other
	require.Eventually(t, func() bool {
		for _, na := range nodeAccessors {
			if !na.IsOnline() {
				return false
			}
		}
		return true
	}, 60*time.Second, 100*time.Millisecond)

	for _, na := range nodeAccessors {
		na.mtx.Lock()
		assert.Greater(t, len(na.nodeID2link), 0)
		assert.Greater(t, len(na.link2nodeID), 0)
		assert.Len(t, na.offerID2state, 0)
		assert.Len(t, na.link2offerID, 0)
		na.mtx.Unlock()
	}

	// each accessor connect both of the previous and the next node
	for i, na := range nodeAccessors {
		rNodeIDs := make(map[shared.NodeID]struct{})
		if i == 0 {
			rNodeIDs[*nodeIDs[len(nodeIDs)-1]] = struct{}{}
		} else {
			rNodeIDs[*nodeIDs[i-1]] = struct{}{}
		}
		if i == len(nodeAccessors)-1 {
			rNodeIDs[*nodeIDs[0]] = struct{}{}
		} else {
			rNodeIDs[*nodeIDs[i+1]] = struct{}{}
		}
		err := na.ConnectLinks(rNodeIDs, map[shared.NodeID]struct{}{})
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool {
		for _, na := range nodeAccessors {
			na.mtx.Lock()
			defer na.mtx.Unlock()
			if len(na.nodeID2link) != 2 || len(na.link2nodeID) != 2 ||
				len(na.offerID2state) != 0 || len(na.link2offerID) != 0 {
				return false
			}
		}
		return true
	}, 60*time.Second, 100*time.Millisecond)

	// send packet
	srcPacket := &shared.Packet{
		DstNodeID: shared.NewRandomNodeID(),
		SrcNodeID: shared.NewRandomNodeID(),
		// ID:        0,
		HopCount: 0,
		Mode:     shared.PacketModeNoRetry | shared.PacketModeOneWay,
	}

	srcPacket1 := *srcPacket
	srcPacket1.ID = 1

	srcPacket1.Content = &proto.PacketContent{
		Content: &proto.PacketContent_Error{
			Error: &proto.Error{
				Code:    10,
				Message: "test1",
			},
		},
	}

	err := nodeAccessors[0].RelayPacket(nodeIDs[1], &srcPacket1)
	require.NoError(t, err)

	srcPacket2 := *srcPacket
	srcPacket2.ID = 2

	srcPacket2.Content = &proto.PacketContent{
		Content: &proto.PacketContent_Error{
			Error: &proto.Error{
				Code:    20,
				Message: "test2",
			},
		},
	}

	err = nodeAccessors[0].RelayPacket(nodeIDs[len(nodeIDs)-1], &srcPacket2)
	require.NoError(t, err)

	// wait for receiving the packet
	require.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(packets[1]) == 1 && len(packets[len(nodeIDs)-1]) == 1
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	assert.Len(t, packets[0], 0)
	receivedPacket1 := packets[1][0]
	receivedPacket2 := packets[len(nodeIDs)-1][0]
	mtx.Unlock()
	assert.Equal(t, receivedPacket1.ID, srcPacket1.ID)
	assert.Equal(t, receivedPacket1.Content.GetError().Code, srcPacket1.Content.GetError().Code)
	assert.Equal(t, receivedPacket1.Content.GetError().Message, srcPacket1.Content.GetError().Message)
	assert.Equal(t, receivedPacket2.ID, srcPacket2.ID)
	assert.Equal(t, receivedPacket2.Content.GetError().Code, srcPacket2.Content.GetError().Code)
	assert.Equal(t, receivedPacket2.Content.GetError().Message, srcPacket2.Content.GetError().Message)
}

func TestNodeAccessorConnectLinks(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(4)

	nodeAccessors := make([]*NodeAccessor, 0)
	mtx := sync.Mutex{}
	packets := make([][]*shared.Packet, 4)
	connections := make([]map[shared.NodeID]struct{}, 4)

	for i, srcNodeID := range nodeIDs {
		i := i
		packets[i] = make([]*shared.Packet, 0)

		na, err := NewNodeAccessor(&Config{
			Logger: slog.Default(),
			Handler: &nodeAccessorHandlerHelper{
				recvPacket: func(n *shared.NodeID, p *shared.Packet) {
					assert.False(t, srcNodeID.Equal(n))
					mtx.Lock()
					defer mtx.Unlock()
					packets[i] = append(packets[i], p)
				},
				changeConnections: func(c map[shared.NodeID]struct{}) {
					mtx.Lock()
					defer mtx.Unlock()
					connections[i] = c
				},
				sendSignalOffer: func(dstNodeID *shared.NodeID, offer *signal.Offer) error {
					return relay(&envelope{
						dstNodeID: dstNodeID,
						offer:     offer,
					}, srcNodeID, nodeIDs, nodeAccessors)
				},
				sendSignalAnswer: func(dstNodeID *shared.NodeID, answer *signal.Answer) error {
					return relay(&envelope{
						dstNodeID: dstNodeID,
						answer:    answer,
					}, srcNodeID, nodeIDs, nodeAccessors)
				},
				sendSignalICE: func(dstNodeID *shared.NodeID, ice *signal.ICE) error {
					return relay(&envelope{
						dstNodeID: dstNodeID,
						ice:       ice,
					}, srcNodeID, nodeIDs, nodeAccessors)
				},
			},
			ICEServers:     iceServers,
			NodeLinkConfig: nlConfig,
		})
		require.NoError(t, err)
		nodeAccessors = append(nodeAccessors, na)
	}

	// make online
	for i, na := range nodeAccessors {
		na.SetBeAlone(false)
		na.Start(t.Context(), nodeIDs[i])
	}

	// wait for detecting each other
	require.Eventually(t, func() bool {
		for _, na := range nodeAccessors {
			if !na.IsOnline() {
				return false
			}
		}
		return true
	}, 60*time.Second, 100*time.Millisecond)

	// node 1 ~ 3 connect to node 0
	for i := 1; i < len(nodeIDs); i++ {
		err := nodeAccessors[i].ConnectLinks(
			map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
			},
			map[shared.NodeID]struct{}{},
		)
		require.NoError(t, err)
	}

	assert.Eventually(t, func() bool {
		for _, na := range nodeAccessors {
			na.mtx.Lock()
			defer na.mtx.Unlock()
		}
		for i := 1; i < len(nodeIDs); i++ {
			if len(nodeAccessors[i].nodeID2link) != 1 || len(nodeAccessors[i].link2nodeID) != 1 ||
				len(nodeAccessors[i].offerID2state) != 0 || len(nodeAccessors[i].link2offerID) != 0 {
				return false
			}
			if nodeAccessors[i].nodeID2link[*nodeIDs[0]] == nil {
				return false
			}
			if nodeAccessors[0].nodeID2link[*nodeIDs[i]] == nil {
				return false
			}
		}
		return true
	}, 60*time.Second, 100*time.Millisecond)

	// node 2 ~ 3 connect to also node 1
	for i := 2; i < len(nodeIDs); i++ {
		err := nodeAccessors[i].ConnectLinks(
			map[shared.NodeID]struct{}{
				*nodeIDs[1]: {},
			},
			map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
			},
		)
		require.NoError(t, err)
	}

	assert.Eventually(t, func() bool {
		for _, na := range nodeAccessors {
			na.mtx.Lock()
			defer na.mtx.Unlock()
		}
		for i := 2; i < len(nodeIDs); i++ {
			// keep the connection to node 0
			assert.Contains(t, nodeAccessors[i].nodeID2link, *nodeIDs[0])
			assert.Contains(t, nodeAccessors[0].nodeID2link, *nodeIDs[i])

			if len(nodeAccessors[i].nodeID2link) != 2 || len(nodeAccessors[i].link2nodeID) != 2 ||
				len(nodeAccessors[i].offerID2state) != 0 || len(nodeAccessors[i].link2offerID) != 0 {
				return false
			}
			if nodeAccessors[i].nodeID2link[*nodeIDs[1]] == nil {
				return false
			}
			if nodeAccessors[1].nodeID2link[*nodeIDs[i]] == nil {
				return false
			}
		}
		return true
	}, 60*time.Second, 100*time.Millisecond)

	for i := 2; i < len(nodeIDs); i++ {
		err := nodeAccessors[i].ConnectLinks(
			map[shared.NodeID]struct{}{},
			map[shared.NodeID]struct{}{
				*nodeIDs[0]: {},
				*nodeIDs[1]: {},
			},
		)
		require.NoError(t, err)
	}
	// keep connections
	for n := 0; n < 30; n++ {
		time.Sleep(100 * time.Millisecond)
		func() {
			for _, na := range nodeAccessors {
				na.mtx.Lock()
				defer na.mtx.Unlock()
				// cancel the connection to the next node
				na.nextConnectionTimestamp = time.Now()
			}
			for i := 2; i < len(nodeIDs); i++ {
				assert.Len(t, nodeAccessors[i].offerID2state, 0)
				assert.Len(t, nodeAccessors[i].link2offerID, 0)
				assert.Contains(t, nodeAccessors[i].nodeID2link, *nodeIDs[0])
				assert.Contains(t, nodeAccessors[i].nodeID2link, *nodeIDs[1])
				assert.Contains(t, nodeAccessors[0].nodeID2link, *nodeIDs[i])
				assert.Contains(t, nodeAccessors[1].nodeID2link, *nodeIDs[i])
			}
		}()
	}
}
