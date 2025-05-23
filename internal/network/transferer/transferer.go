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
	"fmt"
	"log/slog"
	"math/rand"
	"reflect"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/shared"
)

const (
	defaultRetryCountMax = 5
	defaultRetryInterval = 10 * time.Second
)

type Handler interface {
	TransfererSendPacket(*shared.Packet)
	TransfererRelayPacket(*shared.NodeID, *shared.Packet)
}

type Config struct {
	Logger  *slog.Logger
	Handler Handler
	// config parameters for testing
	retryCountMax uint
	retryInterval time.Duration
}

type ResponseHandler interface {
	OnResponse(packet *shared.Packet)
	OnError(constants.PacketErrorCode, string)
}

type Transferer struct {
	logger  *slog.Logger
	handler Handler
	// config parameters for testing
	retryCountMax uint
	retryInterval time.Duration

	ctx         context.Context
	cancel      context.CancelFunc
	localNodeID *shared.NodeID

	mtx             sync.RWMutex
	requestHandlers map[reflect.Type]func(*shared.Packet)
	requestRecord   map[uint32]*requestRecord
}

type requestRecord struct {
	dstNodeID *shared.NodeID
	id        uint32
	mode      shared.PacketMode
	content   *proto.PacketContent
	handler   ResponseHandler
	tryCount  uint
	lastSend  time.Time
}

func NewTransferer(config *Config) *Transferer {
	t := &Transferer{
		logger:          config.Logger,
		handler:         config.Handler,
		mtx:             sync.RWMutex{},
		requestHandlers: make(map[reflect.Type]func(*shared.Packet)),
		requestRecord:   make(map[uint32]*requestRecord),
	}

	if config.retryCountMax != 0 {
		t.retryCountMax = config.retryCountMax
		t.retryInterval = config.retryInterval
	} else {
		t.retryCountMax = defaultRetryCountMax
		t.retryInterval = defaultRetryInterval
	}

	return t
}

func SetRequestHandler[T any](t *Transferer, handler func(*shared.Packet)) {
	// TODO check T is proto.PacketContent.Content
	t.mtx.Lock()
	defer t.mtx.Unlock()

	contentType := reflect.TypeOf((*T)(nil)).Elem()
	if _, ok := t.requestHandlers[contentType]; ok {
		panic(fmt.Sprintf("the handler for %s already exists", contentType.Name()))
	}
	t.requestHandlers[contentType] = handler
}

func (t *Transferer) Start(ctx context.Context, localNodeID *shared.NodeID) {
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.localNodeID = localNodeID

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-t.ctx.Done():
				return
			case <-ticker.C:
				t.subRoutine()
			}
		}
	}()
}

func (t *Transferer) Stop() {
	t.cancel()
}

func (t *Transferer) subRoutine() {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	for id, record := range t.requestRecord {
		if time.Now().Before(record.lastSend.Add(t.retryInterval)) {
			continue
		}

		// timeout error
		if record.tryCount > t.retryCountMax {
			delete(t.requestRecord, id)
			go record.handler.OnError(constants.PacketErrorCodeNetworkTimeout, "request timeout")
			continue
		}

		// retry
		if record.mode&shared.PacketModeNoRetry == 0 {
			packet := &shared.Packet{
				DstNodeID: record.dstNodeID,
				SrcNodeID: t.localNodeID,
				ID:        id,
				HopCount:  0,
				Mode:      record.mode,
				Content:   record.content,
			}
			t.handler.TransfererSendPacket(packet)
		}

		record.tryCount++
		record.lastSend = time.Now()
	}
}

func (t *Transferer) Relay(dstNodeID *shared.NodeID, packet *shared.Packet) {
	t.handler.TransfererRelayPacket(dstNodeID, packet)
}

func (t *Transferer) Request(dstNodeID *shared.NodeID, mode shared.PacketMode, content *proto.PacketContent, handler ResponseHandler) {
	if dstNodeID.Equal(&shared.NodeNeighborhoods) && (mode&shared.PacketModeOneWay) == 0 {
		panic("packet mode should be one way if destination is for next nodes")
	}

	t.mtx.Lock()
	id := rand.Uint32()
	for id == 0 || t.requestRecord[id] != nil {
		id = rand.Uint32()
	}

	t.requestRecord[id] = &requestRecord{
		dstNodeID: dstNodeID,
		id:        id,
		mode:      mode,
		content:   content,
		handler:   handler,
		tryCount:  1,
		lastSend:  time.Now(),
	}
	t.mtx.Unlock()

	packet := &shared.Packet{
		DstNodeID: dstNodeID,
		SrcNodeID: t.localNodeID,
		ID:        id,
		HopCount:  0,
		Mode:      mode,
		Content:   content,
	}
	t.handler.TransfererSendPacket(packet)
}

func (t *Transferer) RequestOneWay(dstNodeID *shared.NodeID, mode shared.PacketMode, content *proto.PacketContent) {
	packet := &shared.Packet{
		DstNodeID: dstNodeID,
		SrcNodeID: t.localNodeID,
		ID:        0,
		HopCount:  0,
		Mode:      mode | shared.PacketModeOneWay,
		Content:   content,
	}
	t.handler.TransfererSendPacket(packet)
}

func (t *Transferer) Response(packetFor *shared.Packet, content *proto.PacketContent) {
	if packetFor.Mode&shared.PacketModeOneWay != 0 {
		panic("one way packet should not be responded")
	}

	mode := shared.PacketModeResponse | shared.PacketModeExplicit | shared.PacketModeOneWay

	packet := &shared.Packet{
		DstNodeID: packetFor.SrcNodeID,
		SrcNodeID: t.localNodeID,
		ID:        packetFor.ID,
		HopCount:  0,
		Mode:      mode,
		Content:   content,
	}
	t.handler.TransfererSendPacket(packet)
}

func (t *Transferer) Error(packetFor *shared.Packet, code constants.PacketErrorCode, message string) {
	content := &proto.PacketContent{
		Content: &proto.PacketContent_Error{
			Error: &proto.Error{
				Code:    uint32(code),
				Message: message,
			},
		},
	}
	t.Response(packetFor, content)
}

func (t *Transferer) Cancel(packet *shared.Packet) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	delete(t.requestRecord, packet.ID)
}

func (t *Transferer) Receive(packet *shared.Packet) {
	if packet.ID != 0 && packet.Mode&shared.PacketModeResponse != 0 {
		t.mtx.Lock()
		defer t.mtx.Unlock()

		record := t.requestRecord[packet.ID]
		if record == nil {
			t.logger.Warn("response for expired or unknown request")
			return
		}
		delete(t.requestRecord, packet.ID)

		go func(p *shared.Packet, handler ResponseHandler) {
			errPacket := p.Content.GetError()
			if errPacket != nil {
				handler.OnError(constants.PacketErrorCode(errPacket.Code), errPacket.Message)
			} else {
				handler.OnResponse(p)
			}
		}(packet, record.handler)
		return
	}

	t.mtx.RLock()
	defer t.mtx.RUnlock()

	contentType := reflect.TypeOf(packet.Content.Content).Elem()
	handler, ok := t.requestHandlers[contentType]
	if !ok {
		panic(fmt.Sprintf("the handler for %s not found", contentType.Name()))
	}

	go handler(packet)
}
