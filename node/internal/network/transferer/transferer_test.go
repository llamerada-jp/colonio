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
package transferer

import (
	"sync"
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/constants"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
	"github.com/stretchr/testify/assert"
)

var _ Handler = &transfererHandlerHelper{}

type transfererHandlerHelper struct {
	sendPacket  func(*networkTypes.Packet)
	relayPacket func(*types.NodeID, *networkTypes.Packet)
}

func (h *transfererHandlerHelper) TransfererSendPacket(p *networkTypes.Packet) {
	if h.sendPacket != nil {
		h.sendPacket(p)
	}
}

func (h *transfererHandlerHelper) TransfererRelayPacket(nid *types.NodeID, p *networkTypes.Packet) {
	if h.relayPacket != nil {
		h.relayPacket(nid, p)
	}
}

var _ ResponseHandler = &responseHandlerHelper{}

type responseHandlerHelper struct {
	onResponse func(*networkTypes.Packet)
	onError    func(constants.PacketErrorCode, string)
}

func (h *responseHandlerHelper) OnResponse(p *networkTypes.Packet) {
	if h.onResponse != nil {
		h.onResponse(p)
	}
}

func (h *responseHandlerHelper) OnError(code constants.PacketErrorCode, message string) {
	if h.onError != nil {
		h.onError(code, message)
	}
}

func TestRequestHandler(t *testing.T) {
	mtx := sync.Mutex{}
	handlerCount := 0

	transferer := NewTransferer(&Config{
		Logger:  testUtil.Logger(t),
		Handler: nil,
	})
	transferer.Start(t.Context(), nil)
	defer transferer.Stop()

	packet := &networkTypes.Packet{
		DstNodeID: nil,
		SrcNodeID: nil,
		ID:        1,
		HopCount:  0,
		Mode:      2,
		Content: &proto.PacketContent{
			Content: &proto.PacketContent_SpreadKnock{
				SpreadKnock: &proto.SpreadKnock{},
			},
		},
	}

	SetRequestHandler[proto.PacketContent_SpreadKnock](transferer, func(p *networkTypes.Packet) {
		mtx.Lock()
		defer mtx.Unlock()
		handlerCount++
		assert.Equal(t, packet, p)
	})
	SetRequestHandler[proto.PacketContent_SpreadRelay](transferer, func(p *networkTypes.Packet) {
		assert.Fail(t, "unexpected call")
	})

	transferer.Receive(packet)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Equal(c, 1, handlerCount)
	}, 10*time.Second, 100*time.Millisecond)
}

func TestRelay(t *testing.T) {
	replyCount := 0
	dstNodeID := types.NewRandomNodeID()
	packet := &networkTypes.Packet{}

	transferer := NewTransferer(&Config{
		Logger: testUtil.Logger(t),
		Handler: &transfererHandlerHelper{
			sendPacket: func(p *networkTypes.Packet) {
				assert.Fail(t, "unexpected call")
			},
			relayPacket: func(nid *types.NodeID, p *networkTypes.Packet) {
				replyCount++
				assert.Equal(t, dstNodeID, nid)
				assert.Equal(t, packet, p)
			},
		},
	})
	transferer.Start(t.Context(), nil)
	defer transferer.Stop()

	transferer.Relay(dstNodeID, packet)

	assert.Equal(t, 1, replyCount)
}

func TestRequestAndResponse(t *testing.T) {
	mtx := sync.Mutex{}
	packets := make([]*networkTypes.Packet, 0)
	var responsePacket *networkTypes.Packet
	localNodeID := types.NewRandomNodeID()
	dstNodeID := types.NewRandomNodeID()

	transferer := NewTransferer(&Config{
		Logger: testUtil.Logger(t),
		Handler: &transfererHandlerHelper{
			sendPacket: func(p *networkTypes.Packet) {
				mtx.Lock()
				defer mtx.Unlock()
				packets = append(packets, p)
			},
			relayPacket: func(nid *types.NodeID, p *networkTypes.Packet) {
				assert.Fail(t, "unexpected call")
			},
		},
		retryCountMax: 3,
		retryInterval: 1 * time.Second,
	})
	transferer.Start(t.Context(), localNodeID)
	defer transferer.Stop()

	transferer.Request(dstNodeID, networkTypes.PacketModeExplicit,
		&proto.PacketContent{
			Content: &proto.PacketContent_SpreadKnock{},
		},
		&responseHandlerHelper{
			onResponse: func(p *networkTypes.Packet) {
				mtx.Lock()
				defer mtx.Unlock()
				assert.Nil(t, responsePacket)
				responsePacket = p
			},
			onError: func(code constants.PacketErrorCode, message string) {
				assert.Fail(t, "unexpected call")
			},
		},
	)

	// Wait for the packet to be sent.
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Len(c, packets, 1)
	}, 10*time.Second, 100*time.Millisecond)

	assert.True(t, dstNodeID.Equal(packets[0].DstNodeID))
	assert.True(t, localNodeID.Equal(packets[0].SrcNodeID))
	assert.NotEqual(t, 0, packets[0].ID)
	assert.Equal(t, uint32(0), packets[0].HopCount)
	assert.Equal(t, networkTypes.PacketModeExplicit, packets[0].Mode)
	assert.IsType(t, &proto.PacketContent_SpreadKnock{}, packets[0].Content.Content)

	// Wait for trying to send packet.
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Len(c, packets, 2)
	}, 10*time.Second, 100*time.Millisecond)

	assert.True(t, dstNodeID.Equal(packets[1].DstNodeID))
	assert.True(t, localNodeID.Equal(packets[1].SrcNodeID))
	assert.Equal(t, packets[0].ID, packets[1].ID)
	assert.Equal(t, uint32(0), packets[1].HopCount)
	assert.Equal(t, networkTypes.PacketModeExplicit, packets[1].Mode)
	assert.IsType(t, &proto.PacketContent_SpreadKnock{}, packets[1].Content.Content)

	// send response
	transferer.Response(packets[0], &proto.PacketContent{
		Content: &proto.PacketContent_SpreadRelay{},
	})

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Len(c, packets, 3)
	}, 10*time.Second, 100*time.Millisecond)
	transferer.Receive(packets[2])

	// Wait for the response packet to be sent.
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.NotNil(c, responsePacket)
	}, 10*time.Second, 100*time.Millisecond)

	assert.True(t, localNodeID.Equal(responsePacket.DstNodeID))
	assert.True(t, localNodeID.Equal(responsePacket.SrcNodeID))
	assert.Equal(t, packets[0].ID, responsePacket.ID)
	assert.Equal(t, uint32(0), responsePacket.HopCount)
	assert.Equal(t, networkTypes.PacketModeResponse|networkTypes.PacketModeExplicit|networkTypes.PacketModeOneWay, responsePacket.Mode)
	assert.IsType(t, &proto.PacketContent_SpreadRelay{}, responsePacket.Content.Content)

	// request record is removed
	assert.Len(t, transferer.requestRecord, 0)
}

func TestRequestOneWay(t *testing.T) {
	mtx := sync.Mutex{}
	var packet *networkTypes.Packet
	localNodeID := types.NewRandomNodeID()
	dstNodeID := types.NewRandomNodeID()

	transferer := NewTransferer(&Config{
		Logger: testUtil.Logger(t),
		Handler: &transfererHandlerHelper{
			sendPacket: func(p *networkTypes.Packet) {
				mtx.Lock()
				defer mtx.Unlock()
				assert.Nil(t, packet)
				packet = p
			},
			relayPacket: func(nid *types.NodeID, p *networkTypes.Packet) {
				assert.Fail(t, "unexpected call")
			},
		},
	})
	transferer.Start(t.Context(), localNodeID)
	defer transferer.Stop()

	transferer.RequestOneWay(dstNodeID, networkTypes.PacketModeExplicit, &proto.PacketContent{
		Content: &proto.PacketContent_SpreadKnock{},
	})

	// Wait for the packet to be sent.
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.NotNil(c, packet)
	}, 10*time.Second, 100*time.Millisecond)

	assert.True(t, dstNodeID.Equal(packet.DstNodeID))
	assert.True(t, localNodeID.Equal(packet.SrcNodeID))
	assert.NotEqual(t, 0, packet.ID)
	assert.Equal(t, uint32(0), packet.HopCount)
	assert.Equal(t, networkTypes.PacketModeExplicit|networkTypes.PacketModeOneWay, packet.Mode)
	assert.IsType(t, &proto.PacketContent_SpreadKnock{}, packet.Content.Content)

	// request record is not created
	assert.Len(t, transferer.requestRecord, 0)
}

func TestTimeout(t *testing.T) {
	mtx := sync.Mutex{}
	packetCount := 0
	hasError := false
	localNodeID := types.NewRandomNodeID()
	dstNodeID := types.NewRandomNodeID()

	transferer := NewTransferer(&Config{
		Logger: testUtil.Logger(t),
		Handler: &transfererHandlerHelper{
			sendPacket: func(p *networkTypes.Packet) {
				mtx.Lock()
				defer mtx.Unlock()
				packetCount++
			},
			relayPacket: func(nid *types.NodeID, p *networkTypes.Packet) {
				assert.Fail(t, "unexpected call")
			},
		},
		retryCountMax: 3,
		retryInterval: 1 * time.Second,
	})
	transferer.Start(t.Context(), localNodeID)
	defer transferer.Stop()

	transferer.Request(dstNodeID, networkTypes.PacketModeNone,
		&proto.PacketContent{Content: &proto.PacketContent_SpreadKnock{}},
		&responseHandlerHelper{
			onResponse: func(p *networkTypes.Packet) {
				assert.Fail(t, "unexpected call")
			},
			onError: func(code constants.PacketErrorCode, message string) {
				mtx.Lock()
				defer mtx.Unlock()
				hasError = true
				assert.Equal(t, constants.PacketErrorCodeNetworkTimeout, code)
			},
		})

	// Wait for timeout.
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()
		assert.True(c, hasError)
	}, 10*time.Second, 100*time.Millisecond)

	// the first packet + retry 3 times
	assert.Equal(t, 4, packetCount)
	// request record is removed after timeout
	assert.Len(t, transferer.requestRecord, 0)
}
