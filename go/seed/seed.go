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
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"github.com/llamerada-jp/colonio/go/proto"
	proto3 "google.golang.org/protobuf/proto"
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
	upgrader *websocket.Upgrader
}

const (
	maxMessageSize   = 1024 * 1024
	pingPeriod       = 30 * time.Second
	pongWait         = 60 * time.Second
	socketBufferSize = 1024
	writeWait        = 10 * time.Second
)

func NewSeed(config *Config) (*Seed, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	upgrader := &websocket.Upgrader{
		ReadBufferSize:  socketBufferSize,
		WriteBufferSize: socketBufferSize,
	}

	return &Seed{
		nodes:    make(map[*Node]struct{}),
		nidMap:   make(map[string]*Node),
		assigned: make(map[string]struct{}),
		config:   config,
		upgrader: upgrader,
	}, nil
}

func (seed *Seed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Upgrading to WebSocket.
	socket, err := seed.upgrader.Upgrade(w, r, nil)
	if err != nil {
		glog.Warning("Failed upgrading to WebSocket", err)
		return
	}

	node := seed.newNode(socket)
	if err := node.start(); err != nil {
		glog.Error(err)
	}
}

func (seed *Seed) Start() error {
	go func() {
		ticker := time.NewTicker(time.Duration(seed.config.PingInterval) * time.Millisecond)

		for {
			<-ticker.C
			seed.execEachTick()
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
		delete(seed.nidMap, nidStr)
		delete(seed.assigned, nidStr)
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
		duration := timeNow.Sub(base)
		// checking timeout
		if duration > time.Duration(seed.config.Timeout)*time.Millisecond {
			node.close()
		}

		// checking ping interval
		if duration > time.Duration(seed.config.PingInterval)*time.Millisecond {
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
			<-ticker.C
			node.socket.SetWriteDeadline(time.Now().Add(writeWait))
			if err := node.socket.WriteMessage(websocket.PingMessage, nil); err != nil {
				glog.Warning(err)
				node.close()
				return
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

		packet := &proto.SeedPacket{}
		if err := proto3.Unmarshal(message, packet); err != nil {
			return err
		}

		node.mutex.Lock()
		node.timeLastRecv = time.Now()
		node.mutex.Unlock()

		err = node.routePacket(packet)
		if err != nil {
			return err
		}
	}
}

func (node *Node) routePacket(packet *proto.SeedPacket) error {
	switch payload := packet.Payload.(type) {
	case *proto.SeedPacket_Auth:
		glog.Info("receive packet auth")
		return node.recvPacketAuth(payload.Auth)

	case *proto.SeedPacket_Ping:
		return nil

	case *proto.SeedPacket_RelayPacket:
		return node.relayPacket(payload.RelayPacket)

	default:
		node.close()
		return fmt.Errorf("wrong packet by command %+v", packet)
	}
}

func (node *Node) recvPacketAuth(payload *proto.SeedAuth) error {
	node.seed.mutex.Lock()
	defer node.seed.mutex.Unlock()

	srcNidStr := nidToString(payload.Nid)
	if l2, ok := node.seed.nidMap[srcNidStr]; ok && l2 != node {
		// Duplicate nid.
		glog.Warningf("Authenticate failed by duplicate nid (%s, %s)\n", node.socket.RemoteAddr().String(), srcNidStr)
		return node.sendAuthResponse(false, "")

	} else if payload.Version != ProtocolVersion {
		// Unsupported version.
		glog.Warningf("Authenticate failed by wrong protocol version (%s)\n", node.socket.RemoteAddr().String())
		return node.sendAuthResponse(false, "")

	} else if node.nid == nil || node.nid.Type == NidTypeNone {
		node.nid = payload.Nid
		node.seed.nidMap[srcNidStr] = node
		node.seed.config.Node.Revision = node.seed.config.Revision
		configByte, err := json.Marshal(node.seed.config.Node)
		if err != nil {
			return err
		}
		if err := node.sendAuthResponse(true, (string)(configByte)); err == nil {
			glog.Infof("Authenticate success (%s, %s)", node.socket.RemoteAddr().String(), srcNidStr)
		} else {
			return err
		}
	} else {
		glog.Warningf("Authenticate failed (%s)", node.socket.RemoteAddr().String())
		return node.sendAuthResponse(false, "")
	}

	if (payload.Hint&HintAssigned) != 0 || len(node.seed.assigned) == 0 {
		node.assigned = true
		node.seed.assigned[nidToString(payload.Nid)] = struct{}{}
	}

	return node.sendHint(true)
}

func (node *Node) relayPacket(payload *proto.SeedRelayPacket) error {
	// Get node id from `WEBRTC_CONNECT::OFFER` packet.
	if offer := payload.Content.GetSignalingOffer(); offer != nil {
		if offer.Type == OfferTypeFirst {
			node.offerID = payload.Id
		}
	}

	node.seed.mutex.Lock()
	defer node.seed.mutex.Unlock()

	packet := &proto.SeedPacket{
		Payload: &proto.SeedPacket_RelayPacket{
			RelayPacket: payload,
		},
	}

	if (payload.Mode & ModeReply) != 0x0 {
		if _, ok := node.seed.nidMap[nidToString(payload.DstNid)]; ok {
			glog.Info("Relay a packet.")
			node.seed.nidMap[nidToString(payload.DstNid)].sendPacket(packet)
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

func (node *Node) sendAuthResponse(success bool, config string) error {
	packet := &proto.SeedPacket{
		Payload: &proto.SeedPacket_AuthResponse{
			AuthResponse: &proto.SeedAuthResponse{
				Success: success,
				Config:  config,
			},
		},
	}

	glog.Infoln("Send auth response." + packet.String())
	return node.sendPacket(packet)
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
		hint = hint | HintOnlyOne
	}

	packet := &proto.SeedPacket{
		Payload: &proto.SeedPacket_Hint{
			Hint: &proto.SeedHint{
				Hint: hint,
			},
		},
	}

	glog.Infoln("Send hint." + packet.String())
	return node.sendPacket(packet)
}

func (node *Node) sendPing() error {
	packet := &proto.SeedPacket{
		Payload: &proto.SeedPacket_Ping{
			Ping: true,
		},
	}

	glog.Info("Send ping.")
	return node.sendPacket(packet)
}

func (node *Node) sendRequireRandom() error {
	packet := &proto.SeedPacket{
		Payload: &proto.SeedPacket_RequireRandom{
			RequireRandom: true,
		},
	}

	glog.Info("Send require random.")
	return node.sendPacket(packet)
}

func (node *Node) sendPacket(packet *proto.SeedPacket) error {
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
