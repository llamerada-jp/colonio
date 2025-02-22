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
package spread

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/internal/wait_any"
	"github.com/llamerada-jp/colonio/proto"
)

const (
	optSomeoneMustExists = uint32(0x1)
)

var (
	ErrSomeoneMustReceiveViolation = fmt.Errorf("receiving node not found")
)

type Options struct {
	SomeoneMustExists bool
}

type Request struct {
	SourceNodeID *shared.NodeID
	Message      []byte
	Options      *Options
}

type Handler interface {
	SpreadGetRelayNodeID(position *geometry.Coordinate) *shared.NodeID
}

type cache struct {
	srcNodeID *shared.NodeID
	uid       uint64
	center    *geometry.Coordinate
	r         float64
	name      string
	message   []byte
	opt       *Options
	timestamp time.Time
}

type Spread struct {
	ctx      context.Context
	logger   *slog.Logger
	handler  Handler
	mtx      sync.RWMutex
	handlers map[string]func(*Request)

	transferer *transferer.Transferer
	config     *config.Spread
	cache      map[uint64]*cache

	localNodeID       *shared.NodeID
	coordinateSystem  geometry.CoordinateSystem
	localPosition     *geometry.Coordinate
	nextNodePositions map[shared.NodeID]*geometry.Coordinate
}

func convertOptToProto(opt *Options) uint32 {
	r := uint32(0)

	if opt.SomeoneMustExists {
		r |= optSomeoneMustExists
	}

	return r
}

func convertOptFromProto(opt uint32) *Options {
	r := &Options{}

	if (optSomeoneMustExists & opt) != 0 {
		r.SomeoneMustExists = true
	}

	return r
}

func NewSpread(ctx context.Context, logger *slog.Logger, handler Handler) *Spread {
	return &Spread{
		ctx:      ctx,
		logger:   logger,
		handler:  handler,
		handlers: make(map[string]func(*Request)),
		cache:    make(map[uint64]*cache),
	}
}

func (s *Spread) ApplyConfig(tr *transferer.Transferer, config *config.Spread, localNodeID *shared.NodeID, cs geometry.CoordinateSystem) {
	s.mtx.Lock()
	s.transferer = tr
	s.config = config
	s.localNodeID = localNodeID
	s.coordinateSystem = cs
	s.mtx.Unlock()

	transferer.SetRequestHandler[proto.PacketContent_SpreadKnock](s.transferer, s.recvKnock)
	transferer.SetRequestHandler[proto.PacketContent_SpreadRelay](s.transferer, s.recvRelay)
	transferer.SetRequestHandler[proto.PacketContent_Spread](s.transferer, s.recvSpread)

	go func() {
		ticker := time.NewTicker(config.CacheLifetime / 2)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return

			case <-ticker.C:
				s.cleanupCache()
			}
		}
	}()
}

func (s *Spread) UpdateLocalPosition(pos *geometry.Coordinate) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.localPosition = pos
}

func (s *Spread) UpdateNextNodePosition(positions map[shared.NodeID]*geometry.Coordinate) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.nextNodePositions = positions
}

func (s *Spread) Post(center *geometry.Coordinate, r float64, name string, message []byte, opt *Options) error {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	tr := s.transferer

	if tr == nil {
		return fmt.Errorf("this function can be used when online and enabled Spread feature")
	}

	cache := s.makeCache(center, r, name, message, opt)

	if opt.SomeoneMustExists {
		wa := wait_any.NewWaitAny()
		if s.coordinateSystem.GetDistance(center, s.localPosition) <= r {
			for nodeID, position := range s.nextNodePositions {
				if s.coordinateSystem.GetDistance(center, position) <= r {
					wa.Add(1)
					s.sendRelay(wa, &nodeID, cache)
				}
			}

		} else {
			nextNodeID := s.handler.SpreadGetRelayNodeID(center)
			if *nextNodeID == shared.NodeIDThis {
				return ErrSomeoneMustReceiveViolation
			}
			wa.Add(1)
			s.sendRelay(wa, nextNodeID, cache)
		}

		if !wa.Wait() {
			return ErrSomeoneMustReceiveViolation
		}
		return nil
	}

	if s.coordinateSystem.GetDistance(center, s.localPosition) > r {
		nextNodeID := s.handler.SpreadGetRelayNodeID(center)
		s.sendRelay(nil, nextNodeID, cache)
		return nil
	}

	if len(message) > int(s.config.SizeToUseKnock) {
		s.sendKnock(nil, cache)

	} else {
		for nodeID, position := range s.nextNodePositions {
			if s.coordinateSystem.GetDistance(cache.center, position) < r {
				s.sendSpread(&nodeID, cache)
			}
		}
	}
	return nil
}

func (s *Spread) SetHandler(name string, handler func(*Request)) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, ok := s.handlers[name]; ok {
		s.logger.Warn("handler already exists", slog.String("name", name))
	}

	s.handlers[name] = handler
}

func (s *Spread) UnsetHandler(name string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, ok := s.handlers[name]; !ok {
		s.logger.Warn("handler not found", slog.String("name", name))
	}

	delete(s.handlers, name)
}

func (s *Spread) makeCache(center *geometry.Coordinate, r float64, name string, message []byte, opt *Options) *cache {
	uid := rand.Uint64()
	for uid == 0 || s.cache[uid] != nil {
		uid = rand.Uint64()
	}

	cache := &cache{
		srcNodeID: s.localNodeID,
		uid:       uid,
		center:    center,
		r:         r,
		name:      name,
		message:   message,
		opt:       opt,
		timestamp: time.Now(),
	}

	s.cache[uid] = cache
	return cache
}

func (s *Spread) makeCacheWithUID(srcNodeID *shared.NodeID, uid uint64, center *geometry.Coordinate, r float64, name string, message []byte, opt *Options) *cache {
	cache := &cache{
		srcNodeID: srcNodeID,
		uid:       uid,
		center:    center,
		r:         r,
		name:      name,
		message:   message,
		opt:       opt,
		timestamp: time.Now(),
	}

	s.cache[uid] = cache
	return cache
}

func (s *Spread) recvKnock(packet *shared.Packet) {
	content := packet.Content.GetSpreadKnock()
	center := geometry.NewCoordinateFromProto(content.Center)
	r := content.R
	uid := content.Uid

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	accept := false
	if _, ok := s.cache[uid]; !ok && s.coordinateSystem.GetDistance(center, s.localPosition) <= r {
		accept = true
	}

	response := &proto.PacketContent{
		Content: &proto.PacketContent_SpreadKnockResponse{
			SpreadKnockResponse: &proto.SpreadKnockResponse{
				Accept: accept,
			},
		},
	}
	s.transferer.Response(packet, response)
}

func (s *Spread) recvRelay(packet *shared.Packet) {
	content := packet.Content.GetSpreadRelay()
	uid := content.Uid
	oneWay := (packet.Mode & shared.PacketModeOneWay) != 0
	center := geometry.NewCoordinateFromProto(content.Center)
	r := content.R
	name := content.Name

	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, ok := s.cache[uid]; ok {
		if !oneWay {
			s.responseForRelay(packet, false)
		}
		return
	}

	cache := s.makeCacheWithUID(
		shared.NewNodeIDFromProto(content.Source),
		uid, center, r, name,
		content.Message,
		convertOptFromProto(content.Opt))

	if s.coordinateSystem.GetDistance(center, s.localPosition) > r {
		nextNodeID := s.handler.SpreadGetRelayNodeID(center)
		if *nextNodeID == shared.NodeIDThis {
			if !oneWay {
				s.responseForRelay(packet, false)
			}
			return
		}
		s.transferer.Relay(nextNodeID, packet)
		return
	}

	handler := s.handlers[name]
	if handler != nil {
		go handler(&Request{
			SourceNodeID: cache.srcNodeID,
			Message:      cache.message,
			Options:      cache.opt,
		})
	}

	if !oneWay {
		s.responseForRelay(packet, true)
	}

	if len(cache.message) > int(s.config.SizeToUseKnock) {
		s.sendKnock(packet.SrcNodeID, cache)

	} else {
		for nodeID, position := range s.nextNodePositions {
			if s.coordinateSystem.GetDistance(cache.center, position) < r {
				s.sendSpread(&nodeID, cache)
			}
		}
	}
}

func (s *Spread) responseForRelay(packet *shared.Packet, success bool) {
	response := &proto.PacketContent{
		Content: &proto.PacketContent_SpreadRelayResponse{
			SpreadRelayResponse: &proto.SpreadRelayResponse{
				Success: success,
			},
		},
	}
	s.transferer.Response(packet, response)
}

func (s *Spread) recvSpread(packet *shared.Packet) {
	content := packet.Content.GetSpread()
	center := geometry.NewCoordinateFromProto(content.Center)
	r := content.R
	uid := content.Uid

	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, ok := s.cache[uid]; ok || s.coordinateSystem.GetDistance(center, s.localPosition) > r {
		return
	}

	cache := s.makeCacheWithUID(
		shared.NewNodeIDFromProto(content.Source),
		uid, center, r,
		content.Name,
		content.Message,
		convertOptFromProto(content.Opt))

	handler := s.handlers[content.Name]
	if handler != nil {
		go handler(&Request{
			SourceNodeID: cache.srcNodeID,
			Message:      cache.message,
			Options:      cache.opt,
		})
	}

	if len(cache.message) > int(s.config.SizeToUseKnock) {
		s.sendKnock(packet.SrcNodeID, cache)

	} else {
		for nodeID, position := range s.nextNodePositions {
			if *packet.SrcNodeID != nodeID && s.coordinateSystem.GetDistance(cache.center, position) < r {
				s.sendSpread(&nodeID, cache)
			}
		}
	}
}

func (s *Spread) sendKnock(exclude *shared.NodeID, cache *cache) {
	content := &proto.PacketContent{
		Content: &proto.PacketContent_SpreadKnock{
			SpreadKnock: &proto.SpreadKnock{
				Center: cache.center.Proto(),
				R:      cache.r,
				Uid:    cache.uid,
			},
		},
	}

	for nodeID, position := range s.nextNodePositions {
		if exclude != nil && *exclude == nodeID {
			continue
		}

		handler := &knockHandler{
			logger:    s.logger,
			cache:     cache,
			dstNodeID: &nodeID,
			sender:    s.sendSpread,
		}

		if s.coordinateSystem.GetDistance(cache.center, position) < cache.r {
			s.transferer.Request(&nodeID, shared.PacketModeNone, content, handler)
		}
	}
}

type knockHandler struct {
	logger    *slog.Logger
	cache     *cache
	dstNodeID *shared.NodeID
	sender    func(dstNodeID *shared.NodeID, cache *cache)
}

func (h *knockHandler) OnResponse(p *shared.Packet) {
	response := p.Content.GetSpreadKnockResponse()
	if response == nil {
		h.logger.Warn("invalid packet received")
		return
	}

	if !response.Accept {
		return
	}

	h.sender(h.dstNodeID, h.cache)
}

func (h *knockHandler) OnError(code constants.PacketErrorCode, message string) {
	h.logger.Warn(message, slog.Int("code", int(code)))
}

func (s *Spread) sendRelay(wa *wait_any.WaitAny, dstNodeID *shared.NodeID, cache *cache) {
	content := &proto.PacketContent{
		Content: &proto.PacketContent_SpreadRelay{
			SpreadRelay: &proto.SpreadRelay{
				Source:  cache.srcNodeID.Proto(),
				Center:  cache.center.Proto(),
				R:       cache.r,
				Uid:     cache.uid,
				Name:    cache.name,
				Message: cache.message,
				Opt:     convertOptToProto(cache.opt),
			},
		},
	}

	if cache.opt.SomeoneMustExists {
		handler := &relayHandler{
			logger: s.logger,
			wa:     wa,
		}
		s.transferer.Request(dstNodeID, shared.PacketModeNone, content, handler)

	} else {
		s.transferer.RequestOneWay(dstNodeID, shared.PacketModeNone, content)
	}
}

func (s *Spread) sendSpread(dstNodeID *shared.NodeID, cache *cache) {
	content := &proto.PacketContent{
		Content: &proto.PacketContent_Spread{
			Spread: &proto.Spread{
				Source:  cache.srcNodeID.Proto(),
				Center:  cache.center.Proto(),
				R:       cache.r,
				Uid:     cache.uid,
				Name:    cache.name,
				Message: cache.message,
				Opt:     convertOptToProto(cache.opt),
			},
		},
	}

	s.transferer.RequestOneWay(dstNodeID, shared.PacketModeNone, content)
}

type relayHandler struct {
	logger *slog.Logger
	wa     *wait_any.WaitAny
}

func (h *relayHandler) OnResponse(p *shared.Packet) {
	response := p.Content.GetSpreadRelayResponse()
	if response == nil {
		h.logger.Warn("invalid packet received")
		h.wa.Done(false)
		return
	}

	h.wa.Done(response.Success)
}

func (h *relayHandler) OnError(code constants.PacketErrorCode, message string) {
	h.logger.Warn(message, slog.Int("code", int(code)))
	h.wa.Done(false)
}

func (s *Spread) cleanupCache() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	now := time.Now()
	for uid, cache := range s.cache {
		if now.After(cache.timestamp.Add(s.config.CacheLifetime)) {
			delete(s.cache, uid)
		}
	}
}
