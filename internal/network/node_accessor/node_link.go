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
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math/bits"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/proto"
	proto3 "google.golang.org/protobuf/proto"
)

type nodeLinkState int

const (
	nodeLinkStateConnecting nodeLinkState = iota
	nodeLinkStateOnline
	nodeLinkStateDisabled
)

type NodeLinkConfig struct {
	ctx          context.Context
	logger       *slog.Logger
	webrtcConfig webRTCConfig

	// should be copied from config.SessionTimeout
	SessionTimeout time.Duration
	// should be copied from config.KeepaliveInterval
	KeepaliveInterval time.Duration
	// should be copied from config.BufferInterval
	BufferInterval time.Duration
	// should be copied from config.WebRTCPacketBaseBytes
	PacketBaseBytes int
}

type nodeLinkHandler interface {
	nodeLinkChangeState(*nodeLink, nodeLinkState)
	nodeLinkUpdateICE(*nodeLink, string)
	nodeLinkRecvPacket(*nodeLink, *shared.Packet)
}

type waiting struct {
	packet  *shared.Packet
	content []byte
}

type nodeLink struct {
	config             *NodeLinkConfig
	handler            nodeLinkHandler
	ctx                context.Context
	cancel             context.CancelFunc
	webrtc             webRTCLink
	stateMtx           sync.RWMutex
	state              nodeLinkState
	keepaliveTimestamp time.Time
	queueMtx           sync.Mutex
	queue              []*waiting
	keepaliveTicker    *time.Ticker
	bufferTicker       *time.Ticker
	stackMtx           sync.Mutex
	stackID            uint32
	stack              []*proto.NodePacket
}

func newNodeLink(config *NodeLinkConfig, handler nodeLinkHandler, createDataChannel bool) (*nodeLink, error) {
	ctx, cancel := context.WithCancel(config.ctx)

	// The bufferTicker fires for configured intervals when some packets are in the buffer,
	// otherwise it disabled immediately.
	bufferTicker := time.NewTicker(1 * time.Second)
	if config.BufferInterval == 0 {
		bufferTicker.Stop()
	}

	var keepaliveTicker *time.Ticker
	if config.KeepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(config.KeepaliveInterval)
	}

	link := &nodeLink{
		config:             config,
		ctx:                ctx,
		cancel:             cancel,
		handler:            handler,
		stateMtx:           sync.RWMutex{},
		state:              nodeLinkStateConnecting,
		keepaliveTimestamp: time.Now(),
		queueMtx:           sync.Mutex{},
		queue:              make([]*waiting, 0),
		keepaliveTicker:    keepaliveTicker,
		bufferTicker:       bufferTicker,
		stackMtx:           sync.Mutex{},
		stackID:            0,
		stack:              nil,
	}

	var err error
	link.webrtc, err = defaultWebRTCLinkFactory(&webRTCLinkConfig{
		webrtcConfig:      config.webrtcConfig,
		createDataChannel: createDataChannel,
	}, &webRTCLinkEventHandler{
		raiseError:      link.webrtcRaiseError,
		changeLinkState: link.webrtcChangeLinkState,
		updateICE:       link.webrtcUpdateICE,
		recvData:        link.webrtcRecvData,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create WebRTCLink %w", err)
	}

	go link.routine()

	return link, nil
}

func (n *nodeLink) getLocalSDP() (string, error) {
	sdp, err := n.webrtc.getLocalSDP()
	if err != nil {
		n.webrtc.disconnect()
		return "", fmt.Errorf("failed to get local SDP %w", err)
	}
	return sdp, nil
}

func (n *nodeLink) setRemoteSDP(sdp string) error {
	err := n.webrtc.setRemoteSDP(sdp)
	if err != nil {
		n.webrtc.disconnect()
		return fmt.Errorf("failed to set remote SDP %w", err)
	}
	return nil
}

func (n *nodeLink) updateICE(ice string) error {
	err := n.webrtc.updateICE(ice)
	if err != nil {
		n.webrtc.disconnect()
		return fmt.Errorf("failed to update ICE %w", err)
	}
	return nil
}

func (n *nodeLink) disconnect() error {
	n.stateMtx.Lock()
	if n.state != nodeLinkStateDisabled {
		n.state = nodeLinkStateDisabled
		defer n.handler.nodeLinkChangeState(n, nodeLinkStateDisabled)
	}
	n.stateMtx.Unlock()

	if n.bufferTicker != nil {
		n.bufferTicker.Stop()
	}
	if n.keepaliveTicker != nil {
		n.keepaliveTicker.Stop()
	}

	n.cancel()
	return n.webrtc.disconnect()
}

func (n *nodeLink) getLinkState() nodeLinkState {
	n.stateMtx.RLock()
	defer n.stateMtx.RUnlock()
	return n.state
}

func (n *nodeLink) sendPacket(packet *shared.Packet) error {
	content, err := proto3.Marshal(packet.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal packet content %w", err)
	}

	waiting := &waiting{
		packet:  packet,
		content: content,
	}

	// send packet immediately if bufferInterval is 0
	if n.config.BufferInterval == 0 {
		n.queueMtx.Lock()
		n.queue = append(n.queue, waiting)
		n.queueMtx.Unlock()
		return n.flush()
	}

	// push packet to queue
	n.queueMtx.Lock()
	n.queue = append(n.queue, waiting)
	sum := 0
	count := len(n.queue)
	for _, w := range n.queue {
		sum += len(w.content)
	}
	n.queueMtx.Unlock()

	// send packet if passed bufferInterval or buffer size is over packetBaseBytes
	if sum > n.config.PacketBaseBytes {
		return n.flush()
	}

	if count == 1 {
		n.bufferTicker.Reset(n.config.BufferInterval)
	}

	return nil
}

func (n *nodeLink) flush() error {
	switch n.getLinkState() {
	case nodeLinkStateConnecting:
		return nil

	case nodeLinkStateDisabled:
		n.queueMtx.Lock()
		defer n.queueMtx.Unlock()
		n.queue = nil
		n.config.logger.Warn("link is disabled when flushing packet")
		return nil
	}

	n.queueMtx.Lock()
	queue := n.queue
	n.queue = make([]*waiting, 0)
	n.queueMtx.Unlock()

	p := &proto.NodePackets{}
	contentSize := 0
	for _, w := range queue {
		if contentSize+len(w.content) > n.config.PacketBaseBytes {
			count, r := bits.Div(0, uint(len(w.content)+contentSize), uint(n.config.PacketBaseBytes))
			if r != 0 {
				count++
			}
			send := 0
			for i := int(count - 1); i >= 0; i-- {
				size := len(w.content) - send
				if size+contentSize > n.config.PacketBaseBytes {
					size = n.config.PacketBaseBytes - contentSize
				}
				if i == 0 {
					p.Packets = append(p.Packets, &proto.NodePacket{
						Head: &proto.NodePacketHead{
							DstNodeId: w.packet.DstNodeID.Proto(),
							SrcNodeId: w.packet.SrcNodeID.Proto(),
							HopCount:  w.packet.HopCount,
							Mode:      uint32(w.packet.Mode),
						},
						Id:      w.packet.ID,
						Index:   uint32(i),
						Content: w.content[send : send+size],
					})
				} else {
					p.Packets = append(p.Packets, &proto.NodePacket{
						Id:      w.packet.ID,
						Index:   uint32(i),
						Content: w.content[send : send+size],
					})
					send += size
					if !n.send(p) {
						return nil
					}
					p = &proto.NodePackets{}
					contentSize = 0
				}
			}

		} else {
			p.Packets = append(p.Packets, &proto.NodePacket{
				Head: &proto.NodePacketHead{
					DstNodeId: w.packet.DstNodeID.Proto(),
					SrcNodeId: w.packet.SrcNodeID.Proto(),
					HopCount:  w.packet.HopCount,
					Mode:      uint32(w.packet.Mode),
				},
				Id:      w.packet.ID,
				Index:   0,
				Content: w.content,
			})
			contentSize += len(w.content)
		}
	}
	if len(p.Packets) != 0 {
		if !n.send(p) {
			return nil
		}
	}

	if n.config.KeepaliveInterval != 0 {
		n.keepaliveTicker.Reset(n.config.KeepaliveInterval)
	}
	// wait next packet idly
	if n.config.BufferInterval > 0 {
		n.bufferTicker.Reset(1 * time.Second)
	}

	return nil
}

func (n *nodeLink) routine() {
	// ticker to check keepalive timeout
	interval := n.config.KeepaliveInterval / 2
	if interval > 1*time.Second {
		interval = 1 * time.Second
	}
	ticker := time.NewTicker(interval)

	for {
		select {
		case <-n.ctx.Done():
			n.disconnect()
			return

		case <-n.keepaliveTicker.C:
			n.queueMtx.Lock()
			count := len(n.queue)
			n.queueMtx.Unlock()
			if count != 0 {
				err := n.flush()
				if err != nil {
					n.config.logger.Warn("failed to flush packet", slog.String("err", err.Error()))
					n.disconnect()
				}
			} else {
				n.sendKeepalive()
			}

		case <-n.bufferTicker.C:
			err := n.flush()
			if err != nil {
				n.config.logger.Warn("failed to flush packet", slog.String("err", err.Error()))
				n.disconnect()
			}

		case <-ticker.C:
			// wake up to check keepalive timeout
		}

		n.stateMtx.RLock()
		timedOut := time.Now().After(n.keepaliveTimestamp.Add(n.config.SessionTimeout))
		n.stateMtx.RUnlock()
		if timedOut {
			n.disconnect()
		}
	}
}

func (n *nodeLink) sendKeepalive() {
	if n.getLinkState() != nodeLinkStateOnline {
		return
	}

	if n.send(&proto.NodePackets{}) {
		n.keepaliveTicker.Reset(n.config.KeepaliveInterval)
	}
}

func (n *nodeLink) send(packet *proto.NodePackets) bool {
	data, err := proto3.Marshal(packet)
	if err != nil {
		panic(err)
	}

	err = n.webrtc.send(data)
	if err != nil {
		n.config.logger.Warn("failed to send packet", slog.String("err", err.Error()))
		err = n.disconnect()
		if err != nil {
			n.config.logger.Warn("failed to disconnect", slog.String("err", err.Error()))
		}
		return false
	}
	return true
}

func (n *nodeLink) webrtcRaiseError(err string) {
	n.config.logger.Warn("webrtc error", slog.String("err", err))
	n.disconnect()
}

func (n *nodeLink) webrtcChangeLinkState(active, online bool) {
	n.stateMtx.Lock()
	prevState := n.state
	if active {
		if online {
			n.state = nodeLinkStateOnline
		} else {
			n.state = nodeLinkStateConnecting
		}
	} else {
		n.state = nodeLinkStateDisabled
	}

	if prevState != n.state {
		defer func(s nodeLinkState) {
			n.handler.nodeLinkChangeState(n, s)
			switch s {
			case nodeLinkStateOnline:
				err := n.flush()
				if err != nil {
					n.config.logger.Warn("failed to flush packet", slog.String("err", err.Error()))
					n.disconnect()
				}

			case nodeLinkStateDisabled:
				n.disconnect()
			}
		}(n.state)
	}
	n.stateMtx.Unlock()
}

func (n *nodeLink) webrtcUpdateICE(ice string) {
	n.handler.nodeLinkUpdateICE(n, ice)
}

func (n *nodeLink) webrtcRecvData(data []byte) {
	p := &proto.NodePackets{}
	err := proto3.Unmarshal(data, p)
	if err != nil {
		n.config.logger.Warn("failed to unmarshal packet", slog.String("err", err.Error()))
		n.disconnect()
		return
	}

	n.stackMtx.Lock()
	defer n.stackMtx.Unlock()

	for _, packet := range p.Packets {
		if n.stack != nil && n.stackID != packet.Id {
			n.config.logger.Warn("received packet id is not continuous")
			n.disconnect()
			return
		}

		var packets []*proto.NodePacket
		if packet.Index != 0 {
			if n.stack == nil {
				n.stack = []*proto.NodePacket{packet}
				n.stackID = packet.Id
			} else {
				n.stack = append(n.stack, packet)
			}
			continue
		} else if n.stack != nil {
			packets = append(n.stack, packet)
			n.stack = nil
		} else {
			packets = []*proto.NodePacket{packet}
		}

		contentsList := make([][]byte, 0)
		var head *proto.NodePacketHead
		for i, p := range packets {
			// check packet format
			if p.Index != uint32(len(packets)-i-1) {
				n.config.logger.Warn("stacked packet index is not continuous")
				n.disconnect()
				return
			}
			if (p.Index == 0 && p.Head == nil) || (p.Index != 0 && p.Head != nil) {
				n.config.logger.Warn("packet head is not set correctly")
				n.disconnect()
				return
			}

			contentsList = append(contentsList, p.Content)
			if p.Index == 0 {
				head = p.Head
			}
		}

		content := &proto.PacketContent{}
		var contentBin []byte
		if len(contentsList) == 1 {
			contentBin = contentsList[0]
		} else {
			contentBin = bytes.Join(contentsList, []byte{})
		}
		err := proto3.Unmarshal(contentBin, content)
		if err != nil {
			n.config.logger.Warn("failed to unmarshal packet content", slog.String("err", err.Error()))
			n.disconnect()
			return
		}
		packet := &shared.Packet{
			DstNodeID: shared.NewNodeIDFromProto(head.DstNodeId),
			SrcNodeID: shared.NewNodeIDFromProto(head.SrcNodeId),
			ID:        packet.Id,
			HopCount:  head.HopCount,
			Mode:      shared.PacketMode(head.Mode),
			Content:   content,
		}
		n.handler.nodeLinkRecvPacket(n, packet)
	}

	n.stateMtx.Lock()
	n.keepaliveTimestamp = time.Now()
	n.stateMtx.Unlock()
}
