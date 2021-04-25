/*
 * Copyright 2019-2021 Yuji Ito <llamerada.jp@gmail.com>
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

package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/golang/glog"
	proto3 "github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	proto "github.com/llamerada-jp/colonio-seed/pkg/seed/core"
)

type Node struct {
	nid          *proto.NodeID
	seed         *Seed
	mutex        sync.Mutex
	timeCreate   time.Time
	timeLastRecv time.Time
	timeLastSend time.Time
	socket       *websocket.Conn // WebSocket connection.
	assigned     bool
	offerID      uint32
}

type Seed struct {
	mutex    sync.Mutex
	nodes    map[*Node]struct{}
	nidMap   map[string]*Node
	assigned map[string]struct{}
	config   *Config
}

const (
	maxMessageSize   = 1024 * 1024
	pingPeriod       = 30 * time.Second
	pongWait         = 60 * time.Second
	socketBufferSize = 1024
	writeWait        = 10 * time.Second
)

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  socketBufferSize,
	WriteBufferSize: socketBufferSize,
}

func NewSeed(config *Config) (*Seed, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	return &Seed{
		nodes:    make(map[*Node]struct{}),
		nidMap:   make(map[string]*Node),
		assigned: make(map[string]struct{}),
		config:   config,
	}, nil
}

func (seed *Seed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Upgrading to WebSocket.
	socket, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		glog.Warning("Failed upgrading to WebSocket")
	}

	node := seed.newNode(socket)
	if err := node.start(); err != nil {
		glog.Fatal(err)
	}
}

func (seed *Seed) Start() error {
	go func() {
		ticker := time.NewTicker(time.Duration(seed.config.PingInterval) * time.Millisecond)

		for {
			select {
			case <-ticker.C:
				seed.execEachTick()
			}
		}

		// ticker.Stop()
	}()

	return nil
}

func (seed *Seed) newNode(socket *websocket.Conn) *Node {
	node := &Node{
		seed:       seed,
		timeCreate: time.Now(),
		socket:     socket,
	}

	seed.mutex.Lock()
	defer seed.mutex.Unlock()
	seed.nodes[node] = struct{}{}

	return node
}

func (seed *Seed) deleteNode(node *Node) {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	if node.nid != nil && node.nid.Type != NidTypeNone {
		nidStr := nidToString(node.nid)
		if _, ok := seed.nidMap[nidStr]; ok {
			delete(seed.nidMap, nidStr)
		}
		if _, ok := seed.assigned[nidStr]; ok {
			delete(seed.assigned, nidStr)
		}
	}
	delete(seed.nodes, node)
}

func (seed *Seed) execEachTick() {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	timeNow := time.Now()

	for node := range seed.nodes {
		base := node.timeCreate
		node.mutex.Lock()
		if base.Before(node.timeLastRecv) {
			base = node.timeLastRecv
		}
		node.mutex.Unlock()
		duration := timeNow.Sub(base).Milliseconds()
		// checking timeout
		if duration > seed.config.Timeout {
			node.close()
		}

		// checking ping interval
		if duration > seed.config.PingInterval {
			node.sendPing()
		}
	}

	// Connect a pair of node randomly.
	if len(seed.nidMap) >= 2 {
		i := rand.Intn(len(seed.nidMap))
		for _, node := range seed.nidMap {
			if i == 0 {
				node.sendRequireRandom()
				break
			}
			i--
		}
	}
}

func (node *Node) start() error {
	node.socket.SetReadLimit(maxMessageSize)
	node.socket.SetReadDeadline(time.Now().Add(pongWait))
	node.socket.SetPongHandler(func(string) error {
		node.socket.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	ticker := time.NewTicker(pingPeriod)
	go func() {
		for {
			select {
			case <-ticker.C:
				node.socket.SetWriteDeadline(time.Now().Add(writeWait))
				if err := node.socket.WriteMessage(websocket.PingMessage, nil); err != nil {
					glog.Warning(err)
					node.close()
					return
				}
			}
		}
	}()

	defer func() {
		ticker.Stop()
		node.close()
	}()

	for {
		messageType, message, err := node.socket.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				return err
			}
			return nil
		}
		if messageType != websocket.BinaryMessage {
			return fmt.Errorf("wrong message type")
		}

		packet := &proto.SeedAccessor{}
		if err := proto3.Unmarshal(message, packet); err != nil {
			return err
		}

		node.mutex.Lock()
		node.timeLastRecv = time.Now()
		node.mutex.Unlock()

		if packet.DstNid.Type == NidTypeSeed {
			if err := node.receivePacket(packet); err != nil {
				return err
			}

		} else if (packet.Mode & ModeRelaySeed) != 0x0 {
			if err := node.relayPacket(packet); err != nil {
				return err
			}

		} else {
			glog.Warning("packet dropped.")
		}
	}
}

func (node *Node) receivePacket(packet *proto.SeedAccessor) error {
	if packet.Channel == ChannelColonio && packet.ModuleChannel == ModuleChannelColonioSeedAccessor {
		switch packet.CommandId {
		case MethodSeedAuth:
			glog.Info("receive packet auth")
			return node.recvPacketAuth(packet)

		case MethodSeedPing:
			return nil

		default:
			node.close()
			return errors.New("wrong packet")
		}

	} else {
		node.close()
		return errors.New("wrong packet")
	}
}

func (node *Node) recvPacketAuth(packet *proto.SeedAccessor) error {
	var content proto.Auth
	if err := proto3.Unmarshal(packet.Content, &content); err != nil {
		return err
	}

	node.seed.mutex.Lock()
	defer node.seed.mutex.Unlock()

	srcNidStr := nidToString(packet.SrcNid)
	if l2, ok := node.seed.nidMap[srcNidStr]; ok && l2 != node {
		// Duplicate nid.
		glog.Warningf("Authenticate failed by duplicate nid (%s, %s)\n", node.socket.RemoteAddr().String(), srcNidStr)
		return node.sendFailure(packet, nil)

	} else if content.Version != ProtocolVersion {
		// Unsupported version.
		glog.Warningf("Authenticate failed by wrong protocol version (%s)\n", node.socket.RemoteAddr().String())
		return node.sendFailure(packet, nil)

	} else if node.nid == nil || node.nid.Type == NidTypeNone {
		node.nid = packet.SrcNid
		node.seed.nidMap[srcNidStr] = node
		node.seed.config.Node.Revision = node.seed.config.Revision
		configByte, err := json.Marshal(node.seed.config.Node)
		if err != nil {
			glog.Fatal(err)
		}
		contentReply := &proto.AuthSuccess{
			Config: (string)(configByte),
		}
		if err := node.sendSuccess(packet, contentReply); err == nil {
			glog.Infof("Authenticate success (%s, %s)", node.socket.RemoteAddr().String(), srcNidStr)
		} else {
			return err
		}
	} else {
		glog.Warningf("Authenticate failed (%s)", node.socket.RemoteAddr().String())
		return node.sendFailure(packet, nil)
	}

	if (content.Hint&HintAssigned) != 0 || len(node.seed.assigned) == 0 {
		node.assigned = true
		node.seed.assigned[nidToString(packet.SrcNid)] = struct{}{}
	}

	return node.sendHint(true)
}

func (node *Node) relayPacket(packet *proto.SeedAccessor) error {
	// Get node id from `WEBRTC_CONNECT::OFFER` packet.
	if packet.Channel == ChannelColonio && packet.ModuleChannel == ModuleChannelColonioNodeAccessor {
		switch packet.CommandId {
		case MethodWebrtcConnectOffer:
			var content proto.Offer
			if err := proto3.Unmarshal(packet.Content, &content); err != nil {
				return err
			}
			if content.Type == OfferTypeFirst {
				node.offerID = packet.Id
			}
		}
	}

	node.seed.mutex.Lock()
	defer node.seed.mutex.Unlock()

	if (packet.Mode & ModeReply) != 0x0 {
		if _, ok := node.seed.nidMap[nidToString(packet.DstNid)]; ok {
			glog.Info("Relay a packet.")
			node.seed.nidMap[nidToString(packet.DstNid)].sendPacket(packet)
		} else {
			glog.Warning("A packet dropped.")
		}

	} else {
		// Relay packet without the source node.
		srcNidStr := nidToString(node.nid)
		siz := len(node.seed.assigned)
		if _, ok := node.seed.assigned[srcNidStr]; ok {
			siz--
		}
		if siz != 0 {
			i := rand.Intn(siz)
			for dstNidStr := range node.seed.assigned {
				if i == 0 {
					return node.seed.nidMap[dstNidStr].sendPacket(packet)
				} else if dstNidStr != srcNidStr {
					i--
				}
			}

		} else {
			return node.sendHint(true)
		}
	}

	return nil
}

func (node *Node) sendHint(locked bool) error {
	var hint uint32
	if node.assigned {
		hint = hint | HintAssigned
	}
	if !locked {
		node.seed.mutex.Lock()
		defer node.seed.mutex.Unlock()
	}
	if len(node.seed.nodes) == 1 {
		hint = hint | HintOnlyone
	}
	content := &proto.Hint{
		Hint: hint,
	}
	contentByte, err := proto3.Marshal(content)
	if err != nil {
		glog.Fatal(err)
	}

	packet := &proto.SeedAccessor{
		DstNid:        node.nid,
		SrcNid:        &proto.NodeID{Type: NidTypeSeed},
		HopCount:      0,
		Id:            0,
		Mode:          ModeExplicit | ModeOneWay,
		Channel:       ChannelColonio,
		ModuleChannel: ModuleChannelColonioSeedAccessor,
		CommandId:     MethodSeedHint,
		Content:       contentByte,
	}

	glog.Infof("Send hint.(%d)\n", len(contentByte))
	return node.sendPacket(packet)
}

func (node *Node) sendPing() error {
	packet := &proto.SeedAccessor{
		DstNid:        node.nid,
		SrcNid:        &proto.NodeID{Type: NidTypeSeed},
		HopCount:      0,
		Id:            0,
		Mode:          ModeExplicit | ModeOneWay,
		Channel:       ChannelColonio,
		ModuleChannel: ModuleChannelColonioSeedAccessor,
		CommandId:     MethodSeedPing,
	}

	glog.Info("Send ping.")
	return node.sendPacket(packet)
}

func (node *Node) sendRequireRandom() error {
	packet := &proto.SeedAccessor{
		DstNid:        node.nid,
		SrcNid:        &proto.NodeID{Type: NidTypeSeed},
		HopCount:      0,
		Id:            0,
		Mode:          ModeExplicit | ModeOneWay,
		Channel:       ChannelColonio,
		ModuleChannel: ModuleChannelColonioSeedAccessor,
		CommandId:     MethodSeedRequireRandom,
	}

	glog.Info("Send require random.")
	return node.sendPacket(packet)
}

func (link *Node) sendSuccess(replyFor *proto.SeedAccessor, content proto3.Message) error {
	contentByte, err := proto3.Marshal(content)
	if err != nil {
		glog.Fatal(err)
	}

	packet := &proto.SeedAccessor{
		DstNid:        replyFor.SrcNid,
		SrcNid:        &proto.NodeID{Type: NidTypeSeed},
		HopCount:      0,
		Id:            replyFor.Id,
		Mode:          ModeExplicit | ModeOneWay,
		Channel:       replyFor.Channel,
		ModuleChannel: replyFor.ModuleChannel,
		CommandId:     MethodSuccess,
		Content:       contentByte,
	}

	glog.Info("Send success.")
	return link.sendPacket(packet)
}

func (node *Node) sendFailure(replyFor *proto.SeedAccessor, content proto3.Message) error {
	var contentByte []byte
	if content != nil {
		var err error
		contentByte, err = proto3.Marshal(content)
		if err != nil {
			log.Fatal(err)
		}
	}

	packet := &proto.SeedAccessor{
		DstNid:        replyFor.SrcNid,
		SrcNid:        &proto.NodeID{Type: NidTypeSeed},
		HopCount:      0,
		Id:            replyFor.Id,
		Mode:          ModeExplicit | ModeOneWay,
		Channel:       replyFor.Channel,
		ModuleChannel: replyFor.ModuleChannel,
		CommandId:     MethodFailure,
		Content:       contentByte,
	}

	glog.Info("Send failure.")
	return node.sendPacket(packet)
}

func (node *Node) sendPacket(packet *proto.SeedAccessor) error {
	node.mutex.Lock()
	defer node.mutex.Unlock()

	node.timeLastSend = time.Now()
	packetBin, err := proto3.Marshal(packet)
	if err != nil {
		glog.Fatal(err)
	}
	defer glog.Info("send packet.")
	node.socket.SetWriteDeadline(time.Now().Add(writeWait))
	return node.socket.WriteMessage(websocket.BinaryMessage, packetBin)
}

func (node *Node) close() error {
	glog.Info("close link")

	if node.seed != nil {
		node.seed.deleteNode(node)
	}

	if err := node.socket.Close(); err != nil {
		return err
	}

	return nil
}
