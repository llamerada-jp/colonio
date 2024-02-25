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
package messaging

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync"

	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Options struct {
	AcceptNearby   bool
	IgnoreResponse bool
}

type Request struct {
	SourceNodeID *shared.NodeID
	Message      []byte
	Options      *Options
}

type ResponseWriter interface {
	Write([]byte) error
}

type Messaging struct {
	logger     *slog.Logger
	transferer *transferer.Transferer
	mtx        sync.RWMutex
	handlers   map[string]func(*Request, ResponseWriter)
}

type responseWriterImpl struct {
	logger     *slog.Logger
	mtx        sync.Mutex
	send       bool
	transferer *transferer.Transferer
	packet     *shared.Packet
}

func newResponseWriter(logger *slog.Logger, transferer *transferer.Transferer, packet *shared.Packet) ResponseWriter {
	r := &responseWriterImpl{
		logger:     logger,
		transferer: transferer,
		packet:     packet,
	}

	// Set finalizer to check if the response is written. (unstable)
	runtime.SetFinalizer(r, func(r *responseWriterImpl) {
		if !r.send {
			logger.Warn("no response was written by the handler, and responded null value automatically.")
			r.Write(nil)
		}
	})

	return r
}

func (r *responseWriterImpl) Write(message []byte) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if r.send {
		return fmt.Errorf("multiple responses are written")
	}
	r.send = true

	r.transferer.Response(r.packet, &proto.PacketContent{
		Content: &proto.PacketContent_MessagingResponse{
			MessagingResponse: &proto.MessagingResponse{
				Response: message,
			},
		},
	})

	return nil
}

type messagingResponse struct {
	ok           bool
	response     []byte
	errorCode    constants.PacketErrorCode
	errorMessage string
}

type messagingResponseHandler struct {
	c chan *messagingResponse
}

func newMessagingResponseHandler() *messagingResponseHandler {
	return &messagingResponseHandler{
		c: make(chan *messagingResponse, 1),
	}
}

func (r *messagingResponseHandler) OnResponse(packet *shared.Packet) {
	r.c <- &messagingResponse{
		ok:       true,
		response: packet.Content.GetMessagingResponse().Response,
	}
	close(r.c)
}

func (r *messagingResponseHandler) OnError(errorCode constants.PacketErrorCode, errorMessage string) {
	r.c <- &messagingResponse{
		ok:           false,
		errorCode:    errorCode,
		errorMessage: errorMessage,
	}
	close(r.c)
}

func NewMessaging(logger *slog.Logger) *Messaging {
	return &Messaging{
		logger:   logger,
		mtx:      sync.RWMutex{},
		handlers: make(map[string]func(*Request, ResponseWriter)),
	}
}

func (m *Messaging) ApplyConfig(tr *transferer.Transferer) {
	m.mtx.Lock()
	m.transferer = tr
	m.mtx.Unlock()

	transferer.SetRequestHandler[proto.PacketContent_Messaging](m.transferer, m.recvMessage)
}

func (m *Messaging) Post(dstNodeID *shared.NodeID, name string, val []byte, opt *Options) ([]byte, error) {
	m.mtx.RLock()
	tr := m.transferer
	m.mtx.RUnlock()

	if tr == nil {
		return nil, fmt.Errorf("post method should be called after online")
	}

	mode := shared.PacketModeNone
	if !opt.AcceptNearby {
		mode = shared.PacketModeExplicit
	}

	content := &proto.PacketContent{
		Content: &proto.PacketContent_Messaging{
			Messaging: &proto.Messaging{
				Name:    name,
				Message: val,
			},
		},
	}

	if opt.IgnoreResponse {
		tr.RequestOneWay(dstNodeID, mode, content)
		return nil, nil
	}

	rh := newMessagingResponseHandler()
	tr.Request(dstNodeID, mode, content, rh)
	res := <-rh.c
	if !res.ok {
		return nil, fmt.Errorf("error response(%d): %s", res.errorCode, res.errorMessage)
	}
	return res.response, nil
}

func (m *Messaging) SetHandler(name string, handler func(*Request, ResponseWriter)) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if _, ok := m.handlers[name]; ok {
		m.logger.Warn("handler already exists", slog.String("name", name))
	}

	m.handlers[name] = handler
}

func (m *Messaging) UnsetHandler(name string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if _, ok := m.handlers[name]; !ok {
		m.logger.Warn("handler not found", slog.String("name", name))
	}

	delete(m.handlers, name)
}

func (m *Messaging) recvMessage(packet *shared.Packet) {
	content := packet.Content.GetMessaging()
	name := content.Name
	oneWay := (packet.Mode & shared.PacketModeOneWay) != 0

	m.mtx.RLock()
	defer m.mtx.RUnlock()
	handler, ok := m.handlers[name]
	if !ok {
		if !oneWay {
			m.transferer.Error(packet, constants.PacketErrorCodeNoOneReceive,
				fmt.Sprintf("no handler for the message(%s)", name))
		}
		m.logger.Debug("handler not found", slog.String("name", name))
		return
	}

	opt := &Options{
		AcceptNearby:   (packet.Mode & shared.PacketModeExplicit) == 0,
		IgnoreResponse: (packet.Mode & shared.PacketModeOneWay) != 0,
	}

	messagingRequest := &Request{
		SourceNodeID: packet.SrcNodeID,
		Message:      content.Message,
		Options:      opt,
	}

	if oneWay {
		go handler(messagingRequest, nil)
	} else {
		rw := newResponseWriter(m.logger, m.transferer, packet)
		go handler(messagingRequest, rw)
	}
}
