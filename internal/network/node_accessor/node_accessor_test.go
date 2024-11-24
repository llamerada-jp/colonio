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
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/proto"
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
}

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

type transfererHandlerHelper struct {
	sendPacket  func(*shared.Packet)
	relayPacket func(*shared.NodeID, *shared.Packet)
}

func (h *transfererHandlerHelper) TransfererSendPacket(p *shared.Packet) {
	if h.sendPacket != nil {
		h.sendPacket(p)
	}
}

func (h *transfererHandlerHelper) TransfererRelayPacket(nid *shared.NodeID, p *shared.Packet) {
	if h.relayPacket != nil {
		h.relayPacket(nid, p)
	}
}

func TestNodeAccessorStandalone(t *testing.T) {
	context, cancel := context.WithCancel(context.Background())
	defer cancel()
	localNodeID := shared.NewRandomNodeID()

	mtx := sync.Mutex{}
	packets := make([]*shared.Packet, 0)
	connections := make(map[shared.NodeID]struct{})

	tra := transferer.NewTransferer(&transferer.Config{
		Ctx:         context,
		Logger:      slog.Default(),
		LocalNodeID: localNodeID,
		Handler: &transfererHandlerHelper{
			sendPacket: func(p *shared.Packet) {
				mtx.Lock()
				defer mtx.Unlock()
				packets = append(packets, p)
			},
			relayPacket: func(nid *shared.NodeID, p *shared.Packet) {
				assert.Fail(t, "unexpected call")
			},
		},
	})

	na := NewNodeAccessor(&Config{
		Ctx:         context,
		Logger:      slog.Default(),
		LocalNodeID: localNodeID,
		Handler: &nodeAccessorHandlerHelper{
			recvPacket: func(_ *shared.NodeID, _ *shared.Packet) {
				assert.Fail(t, "unexpected call")
			},
			changeConnections: func(c map[shared.NodeID]struct{}) {
				mtx.Lock()
				defer mtx.Unlock()
				connections = c
			},
		},
		Transferer: tra,
	})

	na.SetConfig(iceServers, nlConfig)

	// keeping turn off (there is only one node in the cluster)
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		mtx.Lock()
		require.Len(t, packets, 0)
		require.Len(t, connections, 0)
		mtx.Unlock()
	}

	// turn on (is not only one node in the cluster)
	na.SetEnabled(true)
	require.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(packets) == 1 && len(connections) == 0
	}, 60*time.Second, 100*time.Millisecond)

	mtx.Lock()
	packet := packets[0]
	mtx.Unlock()
	assert.True(t, localNodeID.Equal(packet.DstNodeID))
	assert.True(t, localNodeID.Equal(packet.SrcNodeID))
	assert.Equal(t, uint32(0), packet.HopCount)
	assert.Equal(t, shared.PacketModeRelaySeed|shared.PacketModeNoRetry, packet.Mode)
	content := packet.Content.GetSignalingOffer()
	require.NotNil(t, content)
	assert.True(t, localNodeID.Equal(shared.NewNodeIDFromProto(content.PrimeNodeId)))
	assert.True(t, localNodeID.Equal(shared.NewNodeIDFromProto(content.SecondNodeId)))
	assert.Equal(t, uint32(offerTypeFirst), content.Type)

	assert.NotNil(t, na.firstLink)
	assert.Nil(t, na.randomLink)
	assert.Len(t, na.nodeID2link, 0)
	assert.Len(t, na.link2nodeID, 1)
	assert.Len(t, na.connectingStates, 1)

	// turn off
	na.SetEnabled(false)

	na.mtx.Lock()
	assert.Nil(t, na.firstLink)
	assert.Len(t, na.nodeID2link, 0)
	assert.Len(t, na.link2nodeID, 0)
	assert.Len(t, na.connectingStates, 0)
	na.mtx.Unlock()
}

func TestNodeAccessorPair(t *testing.T) {
	context, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodeIDs := testUtil.UniqueNodeIDs(2)

	transferrers := make([]*transferer.Transferer, 0)
	for i := 0; i < 2; i++ {
		i := i
		transferrers = append(transferrers, transferer.NewTransferer(&transferer.Config{
			Ctx:         context,
			Logger:      slog.Default(),
			LocalNodeID: nodeIDs[i],
			Handler: &transfererHandlerHelper{
				sendPacket: func(p *shared.Packet) {
					assert.True(t, nodeIDs[i].Equal(p.SrcNodeID))

					// pass the packet to the other node
					tr := transferrers[1-i]
					tr.Receive(p)
				},
				relayPacket: func(nid *shared.NodeID, p *shared.Packet) {
					assert.Fail(t, "unexpected call")
				},
			},
		}))
	}

	nodeAccessors := make([]*NodeAccessor, 0)
	mtx := sync.Mutex{}
	packets := make([][]*shared.Packet, 2)
	connections := make([]map[shared.NodeID]struct{}, 2)

	for i := 0; i < 2; i++ {
		i := i
		packets[i] = make([]*shared.Packet, 0)
		nodeAccessors = append(nodeAccessors, NewNodeAccessor(&Config{
			Ctx:         context,
			Logger:      slog.Default(),
			LocalNodeID: nodeIDs[i],
			Handler: &nodeAccessorHandlerHelper{
				recvPacket: func(n *shared.NodeID, p *shared.Packet) {
					mtx.Lock()
					defer mtx.Unlock()
					packets[i] = append(packets[i], p)
					assert.True(t, n.Equal(nodeIDs[1-i]))
				},
				changeConnections: func(c map[shared.NodeID]struct{}) {
					mtx.Lock()
					defer mtx.Unlock()
					connections[i] = c
				},
			},
			Transferer: transferrers[i],
		}))
	}

	// make online
	for i := 0; i < 2; i++ {
		nodeAccessors[i].SetConfig(iceServers, nlConfig)
	}
	nodeAccessors[0].SetEnabled(true)

	// wait for detecting each other
	require.Eventually(t, func() bool {
		return nodeAccessors[0].IsOnline() && nodeAccessors[1].IsOnline()
	}, 60*time.Second, 100*time.Millisecond)

	for i := 0; i < 2; i++ {
		nodeAccessors[i].mtx.Lock()
		assert.Nil(t, nodeAccessors[i].firstLink)
		assert.Nil(t, nodeAccessors[i].randomLink)
		assert.Len(t, nodeAccessors[i].nodeID2link, 1)
		assert.Contains(t, nodeAccessors[i].nodeID2link, *nodeIDs[1-i])
		assert.Len(t, nodeAccessors[i].link2nodeID, 1)
		assert.Len(t, nodeAccessors[i].connectingStates, 0)
		nodeAccessors[i].mtx.Unlock()

		mtx.Lock()
		assert.Len(t, connections[i], 1)
		assert.Contains(t, connections[i], *nodeIDs[1-i])
		mtx.Unlock()
	}

	// try random link and it is not effective
	nodeAccessors[0].ConnectRandomLink()
	require.Eventually(t, func() bool {
		nodeAccessors[0].mtx.Lock()
		defer nodeAccessors[0].mtx.Unlock()
		return nodeAccessors[0].randomLink == nil
	}, 10*time.Second, 100*time.Millisecond)

	for i := 0; i < 2; i++ {
		nodeAccessors[i].mtx.Lock()
		assert.Nil(t, nodeAccessors[i].firstLink)
		assert.Nil(t, nodeAccessors[i].randomLink)
		assert.Len(t, nodeAccessors[i].nodeID2link, 1)
		assert.Len(t, nodeAccessors[i].link2nodeID, 1)
		assert.Len(t, nodeAccessors[i].connectingStates, 0)
		nodeAccessors[i].mtx.Unlock()

		mtx.Lock()
		assert.Len(t, connections[i], 1)
		assert.Contains(t, connections[i], *nodeIDs[1-i])
		mtx.Unlock()
	}

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

	// turn off and wait for detecting offline
	nodeAccessors[0].SetEnabled(false)
	require.Eventually(t, func() bool {
		return !nodeAccessors[0].IsOnline() && !nodeAccessors[1].IsOnline()
	}, 10*time.Second, 100*time.Millisecond)

	for i := 0; i < 2; i++ {
		nodeAccessors[i].mtx.Lock()
		assert.Nil(t, nodeAccessors[i].firstLink)
		assert.Nil(t, nodeAccessors[i].randomLink)
		assert.Len(t, nodeAccessors[i].nodeID2link, 0)
		assert.Len(t, nodeAccessors[i].link2nodeID, 0)
		assert.Len(t, nodeAccessors[i].connectingStates, 0)
		nodeAccessors[i].mtx.Unlock()
	}

	require.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(connections[0]) == 0 && len(connections[1]) == 0
	}, 10*time.Second, 100*time.Millisecond)
}

func TestNodeAccessorTrio(t *testing.T) {
	context, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodeIDs := testUtil.UniqueNodeIDs(3)

	transferrers := make([]*transferer.Transferer, 0)
	for i := 0; i < 3; i++ {
		i := i
		transferrers = append(transferrers, transferer.NewTransferer(&transferer.Config{
			Ctx:         context,
			Logger:      slog.Default(),
			LocalNodeID: nodeIDs[i],
			Handler: &transfererHandlerHelper{
				sendPacket: func(p *shared.Packet) {
					if !p.SrcNodeID.Equal(nodeIDs[0]) {
						transferrers[0].Receive(p)
						return
					}

					for j := 0; j < 3; j++ {
						if nodeIDs[j].Equal(p.DstNodeID) {
							transferrers[j].Receive(p)
							return
						}
					}
				},
				relayPacket: func(nid *shared.NodeID, p *shared.Packet) {
					assert.Fail(t, "unexpected call")
				},
			},
		}))
	}

	nodeAccessors := make([]*NodeAccessor, 0)
	mtx := sync.Mutex{}
	packets := make([][]*shared.Packet, 3)
	connections := make([]map[shared.NodeID]struct{}, 3)

	for i := 0; i < 3; i++ {
		packets[i] = make([]*shared.Packet, 0)
		nodeAccessors = append(nodeAccessors, NewNodeAccessor(&Config{
			Ctx:         context,
			Logger:      slog.Default(),
			LocalNodeID: nodeIDs[i],
			Handler: &nodeAccessorHandlerHelper{
				recvPacket: func(n *shared.NodeID, p *shared.Packet) {
					if i == 0 {
						assert.True(t, n.Equal(p.SrcNodeID))
					} else {
						assert.True(t, n.Equal(nodeIDs[0]))
					}
					mtx.Lock()
					defer mtx.Unlock()
					packets[i] = append(packets[i], p)
				},
				changeConnections: func(c map[shared.NodeID]struct{}) {
					mtx.Lock()
					defer mtx.Unlock()
					connections[i] = c
				},
			},
			Transferer: transferrers[i],
		}))
	}

	// make online
	for i := 0; i < 3; i++ {
		nodeAccessors[i].SetConfig(iceServers, nlConfig)
	}
	nodeAccessors[1].SetEnabled(true)
	nodeAccessors[2].SetEnabled(true)

	// wait for detecting each other
	require.Eventually(t, func() bool {
		return nodeAccessors[0].IsOnline() && nodeAccessors[1].IsOnline() && nodeAccessors[2].IsOnline()
	}, 60*time.Second, 100*time.Millisecond)

	nodeAccessors[1].SetEnabled(true)

	mtx.Lock()
	assert.Len(t, nodeAccessors[0].nodeID2link, 2)
	assert.Len(t, nodeAccessors[0].link2nodeID, 2)
	assert.Len(t, nodeAccessors[0].connectingStates, 0)
	assert.Len(t, connections[0], 2)
	assert.Contains(t, connections[0], *nodeIDs[1])
	assert.Contains(t, connections[0], *nodeIDs[2])
	for i := 1; i < 3; i++ {
		assert.Len(t, nodeAccessors[i].nodeID2link, 1)
		assert.Len(t, nodeAccessors[i].link2nodeID, 1)
		assert.Len(t, nodeAccessors[i].connectingStates, 0)
		assert.Len(t, connections[i], 1)
		assert.Contains(t, connections[i], *nodeIDs[0])
	}
	mtx.Unlock()

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
	err = nodeAccessors[0].RelayPacket(nodeIDs[2], &srcPacket2)
	require.NoError(t, err)

	// wait for receiving the packet
	require.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(packets[1]) == 1 && len(packets[2]) == 1
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	assert.Len(t, packets[0], 0)
	receivedPacket1 := packets[1][0]
	receivedPacket2 := packets[2][0]
	mtx.Unlock()
	assert.Equal(t, receivedPacket1.ID, srcPacket1.ID)
	assert.Equal(t, receivedPacket1.Content.GetError().Code, srcPacket1.Content.GetError().Code)
	assert.Equal(t, receivedPacket1.Content.GetError().Message, srcPacket1.Content.GetError().Message)
	assert.Equal(t, receivedPacket2.ID, srcPacket2.ID)
	assert.Equal(t, receivedPacket2.Content.GetError().Code, srcPacket2.Content.GetError().Code)
	assert.Equal(t, receivedPacket2.Content.GetError().Message, srcPacket2.Content.GetError().Message)
}
