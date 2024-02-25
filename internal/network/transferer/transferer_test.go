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
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/stretchr/testify/assert"
)

var _ Handler = &transfererHandlerHelper{}

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

var _ ResponseHandler = &responseHandlerHelper{}

type responseHandlerHelper struct {
	onResponse func(*shared.Packet)
	onError    func(constants.PacketErrorCode, string)
}

func (h *responseHandlerHelper) OnResponse(p *shared.Packet) {
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := &Config{
		Ctx:         ctx,
		Logger:      slog.Default(),
		LocalNodeID: nil,
		Handler:     nil,
	}

	mtx := sync.Mutex{}
	handlerCount := 0

	transferer := NewTransferer(config)

	packet := &shared.Packet{
		DstNodeID: nil,
		SrcNodeID: nil,
		ID:        1,
		HopCount:  0,
		Mode:      2,
		Content: &proto.PacketContent{
			Content: &proto.PacketContent_SignalingIce{
				SignalingIce: &proto.SignalingICE{},
			},
		},
	}

	SetRequestHandler[proto.PacketContent_SignalingIce](transferer, func(p *shared.Packet) {
		mtx.Lock()
		defer mtx.Unlock()
		handlerCount++
		assert.Equal(t, packet, p)
	})
	SetRequestHandler[proto.PacketContent_SignalingOffer](transferer, func(p *shared.Packet) {
		assert.Fail(t, "unexpected call")
	})

	transferer.Receive(packet)

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return handlerCount == 1
	}, 1*time.Second, 10*time.Millisecond)
}

func TestRelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	replyCount := 0
	dstNodeID := shared.NewRandomNodeID()
	packet := &shared.Packet{}

	config := &Config{
		Ctx:         ctx,
		Logger:      slog.Default(),
		LocalNodeID: nil,
		Handler: &transfererHandlerHelper{
			sendPacket: func(p *shared.Packet) {
				assert.Fail(t, "unexpected call")
			},
			relayPacket: func(nid *shared.NodeID, p *shared.Packet) {
				replyCount++
				assert.Equal(t, dstNodeID, nid)
				assert.Equal(t, packet, p)
			},
		},
	}

	transferer := NewTransferer(config)
	transferer.Relay(dstNodeID, packet)

	assert.Equal(t, 1, replyCount)
}

func TestRequestAndResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	packets := make([]*shared.Packet, 0)
	var responsePacket *shared.Packet
	localNodeID := shared.NewRandomNodeID()
	dstNodeID := shared.NewRandomNodeID()

	config := &Config{
		Ctx:         ctx,
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
		retryCountMax: 3,
		retryInterval: 1 * time.Second,
	}

	transferer := NewTransferer(config)
	transferer.Request(dstNodeID, shared.PacketModeExplicit,
		&proto.PacketContent{
			Content: &proto.PacketContent_SignalingIce{},
		},
		&responseHandlerHelper{
			onResponse: func(p *shared.Packet) {
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
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(packets) == 1
	}, 1*time.Second, 10*time.Millisecond)

	assert.True(t, dstNodeID.Equal(packets[0].DstNodeID))
	assert.True(t, localNodeID.Equal(packets[0].SrcNodeID))
	assert.NotEqual(t, 0, packets[0].ID)
	assert.Equal(t, uint32(0), packets[0].HopCount)
	assert.Equal(t, shared.PacketModeExplicit, packets[0].Mode)
	assert.IsType(t, &proto.PacketContent_SignalingIce{}, packets[0].Content.Content)

	// Wait for trying to send packet.
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(packets) == 2
	}, 5*time.Second, 10*time.Millisecond)

	assert.True(t, dstNodeID.Equal(packets[1].DstNodeID))
	assert.True(t, localNodeID.Equal(packets[1].SrcNodeID))
	assert.Equal(t, packets[0].ID, packets[1].ID)
	assert.Equal(t, uint32(0), packets[1].HopCount)
	assert.Equal(t, shared.PacketModeExplicit, packets[1].Mode)
	assert.IsType(t, &proto.PacketContent_SignalingIce{}, packets[1].Content.Content)

	// send response
	transferer.Response(packets[0], &proto.PacketContent{
		Content: &proto.PacketContent_SignalingOffer{},
	})

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(packets) == 3
	}, 1*time.Second, 10*time.Millisecond)
	transferer.Receive(packets[2])

	// Wait for the response packet to be sent.
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return responsePacket != nil
	}, 1*time.Second, 10*time.Millisecond)

	assert.True(t, localNodeID.Equal(responsePacket.DstNodeID))
	assert.True(t, localNodeID.Equal(responsePacket.SrcNodeID))
	assert.Equal(t, packets[0].ID, responsePacket.ID)
	assert.Equal(t, uint32(0), responsePacket.HopCount)
	assert.Equal(t, shared.PacketModeResponse|shared.PacketModeExplicit|shared.PacketModeOneWay, responsePacket.Mode)
	assert.IsType(t, &proto.PacketContent_SignalingOffer{}, responsePacket.Content.Content)

	// request record is removed
	assert.Len(t, transferer.requestRecord, 0)
}

func TestRequestOneWay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	var packet *shared.Packet
	localNodeID := shared.NewRandomNodeID()
	dstNodeID := shared.NewRandomNodeID()

	config := &Config{
		Ctx:         ctx,
		Logger:      slog.Default(),
		LocalNodeID: localNodeID,
		Handler: &transfererHandlerHelper{
			sendPacket: func(p *shared.Packet) {
				mtx.Lock()
				defer mtx.Unlock()
				assert.Nil(t, packet)
				packet = p
			},
			relayPacket: func(nid *shared.NodeID, p *shared.Packet) {
				assert.Fail(t, "unexpected call")
			},
		},
	}

	transferer := NewTransferer(config)
	transferer.RequestOneWay(dstNodeID, shared.PacketModeExplicit, &proto.PacketContent{
		Content: &proto.PacketContent_SignalingIce{},
	})

	// Wait for the packet to be sent.
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return packet != nil
	}, 1*time.Second, 10*time.Millisecond)

	assert.True(t, dstNodeID.Equal(packet.DstNodeID))
	assert.True(t, localNodeID.Equal(packet.SrcNodeID))
	assert.NotEqual(t, 0, packet.ID)
	assert.Equal(t, uint32(0), packet.HopCount)
	assert.Equal(t, shared.PacketModeExplicit|shared.PacketModeOneWay, packet.Mode)
	assert.IsType(t, &proto.PacketContent_SignalingIce{}, packet.Content.Content)

	// request record is not created
	assert.Len(t, transferer.requestRecord, 0)
}

func TestTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	packetCount := 0
	hasError := false
	localNodeID := shared.NewRandomNodeID()
	dstNodeID := shared.NewRandomNodeID()

	config := &Config{
		Ctx:         ctx,
		Logger:      slog.Default(),
		LocalNodeID: localNodeID,
		Handler: &transfererHandlerHelper{
			sendPacket: func(p *shared.Packet) {
				mtx.Lock()
				defer mtx.Unlock()
				packetCount++
			},
			relayPacket: func(nid *shared.NodeID, p *shared.Packet) {
				assert.Fail(t, "unexpected call")
			},
		},
		retryCountMax: 3,
		retryInterval: 1 * time.Second,
	}

	transferer := NewTransferer(config)
	transferer.Request(dstNodeID, shared.PacketModeNone,
		&proto.PacketContent{Content: &proto.PacketContent_SignalingIce{}},
		&responseHandlerHelper{
			onResponse: func(p *shared.Packet) {
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
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return hasError
	}, 10*time.Second, 10*time.Millisecond)

	// the first packet + retry 3 times
	assert.Equal(t, 4, packetCount)
	// request record is removed after timeout
	assert.Len(t, transferer.requestRecord, 0)
}
