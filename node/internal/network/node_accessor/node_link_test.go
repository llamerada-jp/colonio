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
	"math/rand"
	"sync"
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	proto3 "google.golang.org/protobuf/proto"
)

type nodeLinkHandlerHelper struct {
	changeState func(nl *nodeLink, nls nodeLinkState)
	updateICE   func(nl *nodeLink, s string)
	recvPacket  func(nl *nodeLink, p *networkTypes.Packet)
}

func (h *nodeLinkHandlerHelper) nodeLinkChangeState(nl *nodeLink, nls nodeLinkState) {
	if h.changeState != nil {
		h.changeState(nl, nls)
	}
}

func (h *nodeLinkHandlerHelper) nodeLinkUpdateICE(nl *nodeLink, s string) {
	if h.updateICE != nil {
		h.updateICE(nl, s)
	}
}

func (h *nodeLinkHandlerHelper) nodeLinkRecvPacket(nl *nodeLink, p *networkTypes.Packet) {
	if h.recvPacket != nil {
		h.recvPacket(nl, p)
	}
}

func TestNodeLinkNormal(t *testing.T) {
	testLabel := "test"
	config := &NodeLinkConfig{
		ctx:               t.Context(),
		logger:            testUtil.Logger(t),
		label:             testLabel,
		ICEServers:        testUtil.TestingICEServers,
		SessionTimeout:    30 * time.Second,
		KeepaliveInterval: 10 * time.Second,
		BufferInterval:    10 * time.Millisecond,
		PacketBaseBytes:   512 * 1024,
	}

	mtx := sync.Mutex{}
	var received *networkTypes.Packet
	state1 := ""
	state2 := ""

	var link1 *nodeLink
	var link2 *nodeLink
	var err error

	// new
	link1, err = newNodeLink(config, &nodeLinkHandlerHelper{
		changeState: func(nl *nodeLink, nls nodeLinkState) {
			assert.Equal(t, nl, link1)
			mtx.Lock()
			defer mtx.Unlock()
			switch nls {
			case nodeLinkStateConnecting:
				state1 = state1 + "C"
			case nodeLinkStateOnline:
				state1 = state1 + "O"
			case nodeLinkStateDisabled:
				state1 = state1 + "D"
			}
		},
		updateICE: func(nl *nodeLink, s string) {
			assert.Equal(t, nl, link1)
			err := link2.updateICE(s)
			require.NoError(t, err)
		},
		recvPacket: func(nl *nodeLink, p *networkTypes.Packet) {
			mtx.Lock()
			defer mtx.Unlock()
			assert.Equal(t, nl, link1)
			assert.Nil(t, received)
			received = p
		},
	}, true)
	require.NoError(t, err)
	defer link1.disconnect()

	link2, err = newNodeLink(config, &nodeLinkHandlerHelper{
		changeState: func(nl *nodeLink, nls nodeLinkState) {
			assert.Equal(t, nl, link2)
			mtx.Lock()
			defer mtx.Unlock()
			switch nls {
			case nodeLinkStateConnecting:
				state2 = state2 + "C"
			case nodeLinkStateOnline:
				state2 = state2 + "O"
			case nodeLinkStateDisabled:
				state2 = state2 + "D"
			}
		},
		updateICE: func(nl *nodeLink, s string) {
			assert.Equal(t, nl, link2)
			err := link1.updateICE(s)
			require.NoError(t, err)
		},
		recvPacket: func(nl *nodeLink, p *networkTypes.Packet) {
			assert.FailNow(t, "unexpected packet")
		},
	}, false)
	require.NoError(t, err)
	defer link2.disconnect()

	// connect
	go func() {
		sdp1, err := link1.getLocalSDP()
		require.NoError(t, err)
		err = link2.setRemoteSDP(sdp1)
		require.NoError(t, err)

		sdp2, err := link2.getLocalSDP()
		require.NoError(t, err)
		err = link1.setRemoteSDP(sdp2)
		require.NoError(t, err)
	}()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, nodeLinkStateOnline, link1.getLinkState())
		assert.Equal(c, nodeLinkStateOnline, link2.getLinkState())
	}, 10*time.Second, 100*time.Millisecond)

	// check link label
	assert.Equal(t, link1.getLabel(), testLabel)
	assert.Equal(t, link2.getLabel(), testLabel)

	// send packet
	packet := &networkTypes.Packet{
		DstNodeID: types.NewRandomNodeID(),
		SrcNodeID: types.NewRandomNodeID(),
		ID:        99,
		HopCount:  5,
		Mode:      networkTypes.PacketModeExplicit,
		Content: &proto.PacketContent{
			Content: &proto.PacketContent_Error{
				Error: &proto.Error{
					Code:    3,
					Message: "test",
				},
			},
		},
	}
	err = link2.sendPacket(packet)
	require.NoError(t, err)

	// receive packet
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.NotNil(c, received)
	}, 10*time.Second, 100*time.Millisecond)

	// check packet
	mtx.Lock()
	receivedSnapshot := received
	mtx.Unlock()
	assert.True(t, receivedSnapshot.DstNodeID.Equal(packet.DstNodeID))
	assert.True(t, receivedSnapshot.SrcNodeID.Equal(packet.SrcNodeID))
	assert.Equal(t, receivedSnapshot.ID, packet.ID)
	assert.Equal(t, receivedSnapshot.HopCount, packet.HopCount)
	assert.Equal(t, receivedSnapshot.Mode, packet.Mode)
	assert.Equal(t, receivedSnapshot.Content.GetError().Code, packet.Content.GetError().Code)
	assert.Equal(t, receivedSnapshot.Content.GetError().Message, packet.Content.GetError().Message)

	// disconnect
	err = link1.disconnect()
	require.NoError(t, err)
	assert.Equal(t, link1.getLinkState(), nodeLinkStateDisabled)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, nodeLinkStateDisabled, link2.getLinkState())
	}, 10*time.Second, 100*time.Millisecond)

	mtx.Lock()
	state1Snapshot := state1
	state2Snapshot := state2
	mtx.Unlock()
	assert.Equal(t, state1Snapshot, "OD")
	assert.Equal(t, state2Snapshot, "OD")
}

func TestNodeLinkTimeout(t *testing.T) {
	config := &NodeLinkConfig{
		ctx:               t.Context(),
		logger:            testUtil.Logger(t),
		label:             "test",
		ICEServers:        testUtil.TestingICEServers,
		SessionTimeout:    5 * time.Second,
		KeepaliveInterval: 1 * time.Second,
		BufferInterval:    10 * time.Millisecond,
		PacketBaseBytes:   1024,
	}

	var link *nodeLink
	var webrtcLink webRTCLink
	var err error

	// turn offline after session timeout passed with do nothing
	mtx := sync.Mutex{}
	keepalivePackets := 0
	disableErrorCheck := false

	link, err = newNodeLink(config, &nodeLinkHandlerHelper{}, true)
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, nodeLinkStateDisabled, link.getLinkState())
	}, 10*time.Second, 100*time.Millisecond)

	// connect
	link, err = newNodeLink(config, &nodeLinkHandlerHelper{
		updateICE: func(_ *nodeLink, s string) {
			err := webrtcLink.updateICE(s)
			require.NoError(t, err)
		},
	}, false)
	require.NoError(t, err)
	defer link.disconnect()

	webrtcLink, err = defaultWebRTCLinkFactory(&webRTCLinkConfig{
		iceServers: testUtil.TestingICEServers,
		isOffer:    true,
		label:      "test",
	}, &webRTCLinkEventHandler{
		changeLinkState: func(active bool, online bool) {},
		updateICE: func(ice string) {
			err := link.updateICE(ice)
			require.NoError(t, err)
		},
		recvData: func(data []byte) {
			p := &proto.NodePackets{}
			err := proto3.Unmarshal(data, p)
			require.NoError(t, err)
			assert.Equal(t, len(p.Packets), 0)

			mtx.Lock()
			defer mtx.Unlock()
			keepalivePackets++
		},
		raiseError: func(err string) {
			mtx.Lock()
			defer mtx.Unlock()
			if disableErrorCheck {
				return
			}
			assert.FailNow(t, "raise error: "+err)
		},
	})
	require.NoError(t, err)
	defer webrtcLink.disconnect()

	go func() {
		sdp1, err := webrtcLink.getLocalSDP()
		require.NoError(t, err)
		err = link.setRemoteSDP(sdp1)
		require.NoError(t, err)

		sdp2, err := link.getLocalSDP()
		require.NoError(t, err)
		err = webrtcLink.setRemoteSDP(sdp2)
		require.NoError(t, err)
	}()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, nodeLinkStateOnline, link.getLinkState())
		assert.True(c, webrtcLink.isActive())
		assert.True(c, webrtcLink.isOnline())
	}, 10*time.Second, 100*time.Millisecond)

	// wait for keepalive packets
	mtx.Lock()
	keepalivePackets = 0
	mtx.Unlock()
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Greater(c, keepalivePackets, 0)
	}, 10*time.Second, 100*time.Millisecond)

	p := &proto.NodePackets{}
	data, err := proto3.Marshal(p)
	require.NoError(t, err)
	webrtcLink.send(data)

	time.Sleep(3 * time.Second)

	require.Equal(t, nodeLinkStateOnline, link.getLinkState())
	require.True(t, webrtcLink.isActive())
	require.True(t, webrtcLink.isOnline())

	mtx.Lock()
	// ignore error check since the error may be raised when the link is closed by the peer
	disableErrorCheck = true
	mtx.Unlock()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, nodeLinkStateDisabled, link.getLinkState())
		assert.False(c, webrtcLink.isActive())
		assert.False(c, webrtcLink.isOnline())
	}, 10*time.Second, 100*time.Millisecond)
}

func TestNodeLinkBufferInterval(t *testing.T) {
	config1 := &NodeLinkConfig{
		ctx:               t.Context(),
		logger:            testUtil.Logger(t),
		label:             "test",
		ICEServers:        testUtil.TestingICEServers,
		SessionTimeout:    30 * time.Second,
		KeepaliveInterval: 10 * time.Second,
		BufferInterval:    0, // disable buffer
		PacketBaseBytes:   1024 * 1024,
	}

	config2 := *config1
	config2.BufferInterval = 3 * time.Second

	mtx := sync.Mutex{}
	received1 := 0
	received2 := 0

	var link1 *nodeLink
	var link2 *nodeLink
	var err error

	// new
	link1, err = newNodeLink(config1, &nodeLinkHandlerHelper{
		updateICE: func(_ *nodeLink, s string) {
			err := link2.updateICE(s)
			require.NoError(t, err)
		},
		recvPacket: func(_ *nodeLink, p *networkTypes.Packet) {
			mtx.Lock()
			defer mtx.Unlock()
			received1++
		},
	}, true)
	require.NoError(t, err)
	defer link1.disconnect()

	link2, err = newNodeLink(&config2, &nodeLinkHandlerHelper{
		updateICE: func(_ *nodeLink, s string) {
			err := link1.updateICE(s)
			require.NoError(t, err)
		},
		recvPacket: func(_ *nodeLink, p *networkTypes.Packet) {
			mtx.Lock()
			defer mtx.Unlock()
			received2++
		},
	}, false)
	require.NoError(t, err)
	defer link2.disconnect()

	// connect
	go func() {
		sdp1, err := link1.getLocalSDP()
		require.NoError(t, err)
		err = link2.setRemoteSDP(sdp1)
		require.NoError(t, err)

		sdp2, err := link2.getLocalSDP()
		require.NoError(t, err)
		err = link1.setRemoteSDP(sdp2)
		require.NoError(t, err)
	}()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, nodeLinkStateOnline, link1.getLinkState())
		assert.Equal(c, nodeLinkStateOnline, link2.getLinkState())
	}, 10*time.Second, 100*time.Millisecond)

	// send packet
	for i := 0; i < 10; i++ {
		packet := &networkTypes.Packet{
			DstNodeID: types.NewRandomNodeID(),
			SrcNodeID: types.NewRandomNodeID(),
			ID:        uint32(i),
			HopCount:  5,
			Mode:      networkTypes.PacketModeExplicit,
			Content: &proto.PacketContent{
				Content: &proto.PacketContent_Error{
					Error: &proto.Error{
						Code:    3,
						Message: "test",
					},
				},
			},
		}

		err = link1.sendPacket(packet)
		require.NoError(t, err)
		err = link2.sendPacket(packet)
		require.NoError(t, err)
	}

	// receive packet
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Equal(c, 0, received1)
		assert.Equal(c, 10, received2)
	}, 10*time.Second, 100*time.Millisecond)

	time.Sleep(1 * time.Second)
	mtx.Lock()
	assert.Equal(t, received1, 0)
	mtx.Unlock()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Equal(c, 10, received1)
	}, 10*time.Second, 100*time.Millisecond)
}

func TestNodeLinkPacketBaseBytes(t *testing.T) {
	config := &NodeLinkConfig{
		ctx:               t.Context(),
		logger:            testUtil.Logger(t),
		label:             "test",
		ICEServers:        testUtil.TestingICEServers,
		SessionTimeout:    30 * time.Second,
		KeepaliveInterval: 10 * time.Second,
		BufferInterval:    3 * time.Second,
		PacketBaseBytes:   1024,
	}

	mtx := sync.Mutex{}
	received := []*networkTypes.Packet{}
	var link1 *nodeLink
	var link2 *nodeLink
	var err error

	// new
	link1, err = newNodeLink(config, &nodeLinkHandlerHelper{
		updateICE: func(_ *nodeLink, s string) {
			err := link2.updateICE(s)
			require.NoError(t, err)
		},
		recvPacket: func(_ *nodeLink, p *networkTypes.Packet) {
			mtx.Lock()
			defer mtx.Unlock()
			received = append(received, p)
		},
	}, true)
	require.NoError(t, err)
	defer link1.disconnect()

	link2, err = newNodeLink(config, &nodeLinkHandlerHelper{
		updateICE: func(_ *nodeLink, s string) {
			err := link1.updateICE(s)
			require.NoError(t, err)
		},
		recvPacket: func(_ *nodeLink, p *networkTypes.Packet) {
			assert.FailNow(t, "unexpected packet")
		},
	}, false)
	require.NoError(t, err)
	defer link2.disconnect()

	// connect
	go func() {
		sdp1, err := link1.getLocalSDP()
		require.NoError(t, err)
		err = link2.setRemoteSDP(sdp1)
		require.NoError(t, err)

		sdp2, err := link2.getLocalSDP()
		require.NoError(t, err)
		err = link1.setRemoteSDP(sdp2)
		require.NoError(t, err)
	}()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, nodeLinkStateOnline, link1.getLinkState())
		assert.Equal(c, nodeLinkStateOnline, link2.getLinkState())
	}, 10*time.Second, 100*time.Millisecond)

	testCases := []struct {
		name    string
		packets []*networkTypes.Packet
	}{
		{
			name:    "send a medium packet",
			packets: []*networkTypes.Packet{genRandomPacket(1024 + 512)},
		},
		{
			name:    "send a large packet",
			packets: []*networkTypes.Packet{genRandomPacket(10 * 1024)},
		},
		{
			name: "send some small packets and a medium packet, sum of packet size is larger than packetBaseBytes",
			packets: []*networkTypes.Packet{
				genRandomPacket(3),
				genRandomPacket(3),
				genRandomPacket(3),
				genRandomPacket(1024),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mtx.Lock()
			received = []*networkTypes.Packet{}
			mtx.Unlock()
			for _, p := range tc.packets {
				err := link2.sendPacket(p)
				require.NoError(t, err)
			}

			// receive packet
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				mtx.Lock()
				defer mtx.Unlock()
				assert.Len(c, received, len(tc.packets))
			}, 10*time.Second, 100*time.Millisecond)
			mtx.Lock()
			receivedSnapshot := append([]*networkTypes.Packet(nil), received...)
			mtx.Unlock()
			assert.True(t, packetEqual(receivedSnapshot, tc.packets))
		})
	}

	// send a lot of small packets
	send := []*networkTypes.Packet{}
	mtx.Lock()
	received = []*networkTypes.Packet{}
	mtx.Unlock()
	for i := 0; i < 500; i++ {
		p := genRandomPacket(3)
		send = append(send, p)
		link2.sendPacket(p)
	}

	// receive some packets that over flow the buffer immediately
	time.Sleep(1 * time.Second)
	mtx.Lock()
	assert.GreaterOrEqual(t, len(received), 10)
	assert.Greater(t, 500, len(received))
	mtx.Unlock()

	// receive the rest packets after buffer interval
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Len(c, received, 500)
	}, 10*time.Second, 100*time.Millisecond)
	mtx.Lock()
	receivedSnapshot := append([]*networkTypes.Packet(nil), received...)
	mtx.Unlock()
	assert.True(t, packetEqual(receivedSnapshot, send))
}

func genRandomPacket(siz uint) *networkTypes.Packet {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	message := ""
	for i := uint(0); i < siz; i++ {
		message += string(charset[rand.Intn(len(charset))])
	}

	return &networkTypes.Packet{
		DstNodeID: types.NewRandomNodeID(),
		SrcNodeID: types.NewRandomNodeID(),
		ID:        rand.Uint32(),
		HopCount:  rand.Uint32(),
		Mode:      networkTypes.PacketModeExplicit,
		Content: &proto.PacketContent{
			Content: &proto.PacketContent_Error{
				Error: &proto.Error{
					Code:    rand.Uint32(),
					Message: message,
				},
			},
		},
	}
}

func packetEqual(p1s, p2s []*networkTypes.Packet) bool {
	if len(p1s) != len(p2s) {
		return false
	}

	for i := range p1s {
		p1 := p1s[i]
		p2 := p2s[i]

		if !p1.DstNodeID.Equal(p2.DstNodeID) {
			return false
		}
		if !p1.SrcNodeID.Equal(p2.SrcNodeID) {
			return false
		}
		if p1.ID != p2.ID {
			return false
		}
		if p1.HopCount != p2.HopCount {
			return false
		}
		if p1.Mode != p2.Mode {
			return false
		}
		if p1.Content.GetError().Code != p2.Content.GetError().Code {
			return false
		}
		if p1.Content.GetError().Message != p2.Content.GetError().Message {
			return false
		}
	}

	return true
}
