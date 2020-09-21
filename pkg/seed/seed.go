/*
 * Copyright 2019-2020 Yuji Ito <llamerada.jp@gmail.com>
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
	"io"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	proto "github.com/colonio/colonio-seed/pkg/seed/core"
	"github.com/gobwas/ws"
	proto3 "github.com/golang/protobuf/proto"
	"github.com/mailru/easygo/netpoll"
)

type ConfigNodeAccessor struct {
	BufferInterval *uint32 `json:"bufferInterval,omitempty"`
	HopCountMax    *uint32 `json:"hopCountMax,omitempty"`
	PacketSize     *uint32 `json:"packetSize,omitempty"`
}

type ConfigCoordSystem2D struct {
	Type *string `json:"type,omitempty"`
	// for sphere
	Radius *float64 `json:"radius,omitempty"`
	// for plane
	XMin *float64 `json:"xMin,omitempty"`
	XMax *float64 `json:"xMax,omitempty"`
	YMin *float64 `json:"yMin,omitempty"`
	YMax *float64 `json:"yMax,omitempty"`
}

type ConfigIceServer struct {
	Urls       []string `json:"urls,omitempty"`
	Username   *string  `json:"username,omitempty"`
	Credential *string  `json:"credential,omitempty"`
}

type ConfigRouting struct {
	UpdatePeriod     *uint32 `json:"updatePeriod,omitempty"`
	ForceUpdateTimes *uint32 `json:"forceUpdateTimes,omitempty"`
}

type ConfigModule struct {
	Type    string `json:"type"`
	Channel uint32 `json:"channel"`

	RetryMax         *uint32 `json:"retryMax,omitempty"`         // for map
	RetryIntervalMin *uint32 `json:"retryIntervalMin,omitempty"` // for map
	RetryIntervalMax *uint32 `json:"retryIntervalMax,omitempty"` // for map

	CacheTime *uint32 `json:"cacheTime,omitempty"` // for pubsub_2d
}

type ConfigNode struct {
	Revision      float64                 `json:"revision"`
	NodeAccessor  *ConfigNodeAccessor     `json:"nodeAccessor,omitempty"`
	CoordSystem2d *ConfigCoordSystem2D    `json:"coordSystem2D,omitempty"`
	IceServers    []ConfigIceServer       `json:"iceServers,omitempty"`
	Routing       *ConfigRouting          `json:"routing,omitempty"`
	Modules       map[string]ConfigModule `json:"modules,omitempty"`
}

type Config struct {
	Revision     float64     `json:"revision,omitempty"`
	PingInterval int64       `json:"pingInterval"`
	Timeout      int64       `json:"timeout"`
	Node         *ConfigNode `json:"node,omitempty"`
}

func (c *Config) CastConfig() *Config {
	return c
}

type Link struct {
	group        *Group
	nid          *proto.NodeID
	mutex        sync.Mutex
	timeCreate   time.Time
	timeLastRecv time.Time
	timeLastSend time.Time
	srcIP        string
	conn         net.Conn // WebSocket connection.
	assigned     bool
	offerID      uint32
}

type Group struct {
	seed         *Seed
	name         string
	mutex        sync.Mutex
	links        map[*Link]struct{}
	nidMap       map[string]*Link
	assigned     map[string]struct{}
	config       *Config
	tickerPeriod chan bool // tickerの終了イベントを通知するために利用
}
type GroupBinder interface {
	Bind(string) (*Group, error)
}

type Seed struct {
	binder      GroupBinder
	groups      map[string]*Group
	poolUpgrade *Pool
	poolReceive *Pool
	linkPoller  chan *Link
	//poller      netpoll.Poller
}

type context struct {
	routineLocal map[int]interface{}
}

const (
	// プロトコルバージョン
	ProtocolVersion = "A1"

	// Node ID.
	NidStrNone    = ""
	NidStrThis    = "."
	NidStrSeed    = "seed"
	NidStrNext    = "next"
	NidTypeNone   = 0
	NidTypeNormal = 1
	NidTypeThis   = 2
	NidTypeSeed   = 3
	NidTypeNext   = 4

	// Packet mode.
	ModeNone      = 0x0000
	ModeReply     = 0x0001
	ModeExplicit  = 0x0002
	ModeOneWay    = 0x0004
	ModeRelaySeed = 0x0008
	ModeNoRetry   = 0x0010

	ChannelNone    = 0
	ChannelColonio = 1

	ModuleChannelColonioSeedAccessor = 2
	ModuleChannelColonioNodeAccessor = 3

	// Commonly packet method.
	MethodError   = 0xffff
	MethodFailure = 0xfffe
	MethodSuccess = 0xfffd

	MethodSeedAuth          = 1
	MethodSeedHint          = 2
	MethodSeedPing          = 3
	MethodSeedRequireRandom = 4

	MethodWebrtcConnectOffer = 1

	// Offer type of WebRTC connect.
	OfferTypeFirst = 0

	// Hint
	HintOnlyone  = 0x0001
	HintAssigned = 0x0002

	// Key of routineLocal
	GroupMutex = 1
	LinkMutex  = 2
)

/* Logger is interface of used to output log for seed module.
 */
type Logger struct {
	E *log.Logger
	W *log.Logger
	I *log.Logger
	D *log.Logger
}

var logger *Logger

func SetLogger(l *Logger) {
	logger = l
}

func newContext() *context {
	return &context{
		routineLocal: make(map[int]interface{}),
	}
}

func NewSeed(binder GroupBinder) *Seed {
	return &Seed{
		binder:      binder,
		groups:      make(map[string]*Group),
		poolUpgrade: NewPool(32),
		poolReceive: NewPool(32),
		linkPoller:  make(chan *Link),
	}
}

func nidToString(nid *proto.NodeID) string {
	switch nid.Type {
	//case NidTypeNone:
	//	return NidNone

	case NidTypeNormal:
		return fmt.Sprintf("%016x%016x", nid.Id0, nid.Id1)

	case NidTypeThis:
		return NidStrThis

	case NidTypeSeed:
		return NidStrSeed

	case NidTypeNext:
		return NidStrNext
	}

	return NidStrNone
}

func (seed *Seed) CreateGroup(groupName string, config *Config) (*Group, error) {
	if _, ok := seed.groups[groupName]; ok {
		logger.E.Fatalln("Duplicate group : " + groupName)
	}
	tickerPeriod := make(chan bool)

	if err := seed.checkConfig(config); err != nil {
		return nil, err
	}

	group := &Group{
		seed:         seed,
		name:         groupName,
		links:        make(map[*Link]struct{}),
		nidMap:       make(map[string]*Link),
		assigned:     make(map[string]struct{}),
		config:       config,
		tickerPeriod: tickerPeriod,
	}

	go func() {
		ticker := time.NewTicker(time.Duration(config.PingInterval) * time.Millisecond)

		for {
			select {
			case <-ticker.C:
				go func() {
					context := newContext()
					group.execEachTick(context)
				}()

			case <-tickerPeriod:
				goto Out
			}
		}

	Out:
		ticker.Stop()
	}()

	seed.groups[groupName] = group
	return group, nil
}

func (seed *Seed) GetGroup(name string) (group *Group, ok bool) {
	group, ok = seed.groups[name]
	return
}

func (seed *Seed) DestroyGroup(group *Group) {
	group.tickerPeriod <- true
	delete(seed.groups, group.name)
}

func (seed *Seed) Start(host string, port int) error {
	logger.I.Printf("Start colonio host = %s , port = %d\n", host, port)
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		logger.E.Print(err)
		return err
	}

	go seed.pollLink()

	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.E.Printf("Accept error (%s) %s\n", conn.RemoteAddr().String(), err)
			continue
		}

		seed.poolUpgrade.Schedule(func() {
			context := newContext()
			err := seed.upgradeSocket(context, conn)
			if err != nil {
				logger.W.Println(err)
			}
		})
	}
}

func (seed *Seed) checkConfig(config *Config) error {
	if config.PingInterval <= 0 {
		return errors.New("Config value of `pingInverval` must be larger then 0.")
	}

	if config.Timeout <= 0 {
		return errors.New("Config value of `inverval` must be larger then 0.")
	}

	if config.Node == nil {
		return errors.New("Config value of `node` must be map type structure.")

	} else {
		if config.Node.IceServers == nil || len(config.Node.IceServers) == 0 {
			return errors.New("Config value of `node.iceServers` required")
		}

		if config.Node.Routing == nil {
			return errors.New("Config value of `node.routing` required")
		}
	}

	return nil
}

func (seed *Seed) upgradeSocket(context *context, conn net.Conn) error {
	var group *Group
	var srcIP string = conn.RemoteAddr().String()

	upgrader := ws.Upgrader{
		OnHeader: func(key []byte, value []byte) error {
			if string(key) == "X-Forwarded-For" {
				srcIP = string(value)
			}
			return nil
		},
		OnRequest: func(uri []byte) error {
			g, err := seed.binder.Bind(string(uri))
			group = g
			return err
		},
	}

	_, err := upgrader.Upgrade(conn)
	if err != nil {
		logger.W.Printf("Upgrade error (%s) %v\n", srcIP, err)
		conn.Close()
		return err

	} else {
		logger.I.Printf("Upgrade success (%s)\n", srcIP)
		link := group.createLink(context, conn)
		link.srcIP = srcIP
		seed.linkPoller <- link

		return nil
	}
}

func (seed *Seed) pollLink() {
	poller, err := netpoll.New(nil)
	if err != nil {
		logger.E.Fatalln(err)
		// handle error
		return
	}

	for {
		link := <-seed.linkPoller
		desc := netpoll.Must(netpoll.HandleRead(link.conn))
		poller.Start(desc, func(ev netpoll.Event) {
			if ev&netpoll.EventReadHup != 0 {
				logger.W.Printf("Poller setting failed and close (%s)\n", link.srcIP)
				context := newContext()
				poller.Stop(desc)
				link.close(context)
				return
			}

			seed.poolReceive.Schedule(func() {
				context := newContext()
				// 受信データが無くなったら切断
				if next, err := link.receive(context); !next || err != nil {
					logger.I.Printf("Close connection (%s)\n", link.srcIP)
					if err != nil {
						logger.W.Println(err)
					}
					poller.Stop(desc)
					link.close(context)
					desc.Close()
				}
			})
		})
	}
}

func (group *Group) createLink(context *context, conn net.Conn) *Link {
	link := &Link{
		group:      group,
		timeCreate: time.Now(),
		conn:       conn,
	}

	if group.lockMutex(context) {
		defer group.unlockMutex(context)
	}
	group.links[link] = struct{}{}

	return link
}

func (group *Group) removeLink(context *context, link *Link) {
	if link.lockMutex(context) {
		defer link.unlockMutex(context)
	}
	if group.lockMutex(context) {
		defer group.unlockMutex(context)
	}

	link.group = nil
	if link.nid != nil && link.nid.Type != NidTypeNone {
		nidStr := nidToString(link.nid)
		if _, ok := group.nidMap[nidStr]; ok {
			delete(group.nidMap, nidStr)
		}
		if _, ok := group.assigned[nidStr]; ok {
			delete(group.assigned, nidStr)
		}
	}
	delete(group.links, link)
}

func (group *Group) execEachTick(context *context) {
	if group.lockMutex(context) {
		defer group.unlockMutex(context)
	}

	timeNow := time.Now()

	for link := range group.links {
		base := link.timeCreate
		if base.Before(link.timeLastRecv) {
			base = link.timeLastRecv
		}
		duration := timeNow.Sub(base).Milliseconds()
		// checking timeout
		if duration > group.config.Timeout {
			link.close(context)
		}

		// checking ping interval
		if duration > group.config.PingInterval {
			link.sendPing(context)
		}
	}

	// ランダム接続
	if len(group.nidMap) >= 2 {
		i := rand.Intn(len(group.nidMap))
		for _, link := range group.nidMap {
			if i == 0 {
				link.sendRequireRandom(context)
				break
			}
			i--
		}
	}

	// スレッド廃棄
	if len(group.links) == 0 {
		group.seed.DestroyGroup(group)
	}
}

func (group *Group) isOnlyOne(context *context, link *Link) bool {
	if group.lockMutex(context) {
		defer group.unlockMutex(context)
	}

	return len(group.links) == 1
}

func (group *Group) lockMutex(context *context) bool {
	if _, ok := context.routineLocal[GroupMutex]; ok {
		return false
	} else {
		context.routineLocal[GroupMutex] = true
		group.mutex.Lock()
		return true
	}
}

func (group *Group) unlockMutex(context *context) {
	group.mutex.Unlock()
	delete(context.routineLocal, GroupMutex)
}

func (link *Link) receive(context *context) (bool, error) {
	if link.lockMutex(context) {
		defer link.unlockMutex(context)
	}
	if link.conn == nil {
		return false, fmt.Errorf("disconnected link")
	}
	// Reset the Masked flag, server frames must not be masked as
	// RFC6455 says.
	header, err := ws.ReadHeader(link.conn)
	if err != nil {
		return false, fmt.Errorf("failed to read header", err)
	}

	payload := make([]byte, header.Length)
	_, err = io.ReadFull(link.conn, payload)
	if err != nil {
		return false, fmt.Errorf("failed to read body", err)
	}
	if header.Masked {
		ws.Cipher(payload, header.Mask, 0)
	}

	switch header.OpCode {
	case ws.OpPing:
		logger.D.Println("receive OpPing")
		frame := ws.NewPongFrame(payload)
		if err := ws.WriteFrame(link.conn, frame); err != nil {
			return false, err
		}

	case ws.OpBinary:
		logger.D.Println("receive OpBinary")
		var packet proto.SeedAccessor
		if err := proto3.Unmarshal(payload, &packet); err != nil {
			return false, err
		}

		link.timeLastRecv = time.Now()

		if packet.DstNid.Type == NidTypeSeed {
			if err := link.receivePacket(context, &packet); err != nil {
				return false, err
			}

		} else if (packet.Mode & ModeRelaySeed) != 0x0 {
			if err := link.relayPacket(context, &packet); err != nil {
				return false, err
			}

		} else {
			logger.W.Println("packet dropped.")
		}

	case ws.OpClose:
		logger.D.Println("receive OpClose")
		link.close(context)
		return false, nil

	default:
		logger.D.Println("unsupported OpCode", header.OpCode)
	}

	return true, nil
}

func (link *Link) receivePacket(context *context, packet *proto.SeedAccessor) error {
	if packet.Channel == ChannelColonio && packet.ModuleChannel == ModuleChannelColonioSeedAccessor {
		switch packet.CommandId {
		case MethodSeedAuth:
			logger.D.Println("receive packet auth")
			return link.recvPacketAuth(context, packet)

		case MethodSeedPing:
			return nil

		default:
			link.close(context)
			return errors.New("wrong packet")
		}

	} else {
		link.close(context)
		return errors.New("wrong packet")
	}
}

func (link *Link) recvPacketAuth(context *context, packet *proto.SeedAccessor) error {
	var content proto.Auth
	if err := proto3.Unmarshal(packet.Content, &content); err != nil {
		return err
	}

	if link.group.lockMutex(context) {
		defer link.group.unlockMutex(context)
	}

	if l2, ok := link.group.nidMap[nidToString(packet.SrcNid)]; ok && l2 != link {
		// IDが重複している
		logger.W.Printf("Authenticate failed by duplicate nid (%s)\n", link.srcIP)
		return link.sendFailure(context, packet, nil)

	} else if content.Version != ProtocolVersion {
		// バージョンがサポート外
		logger.W.Printf("Authenticate failed by wrong protocol version (%s)\n", link.srcIP)
		return link.sendFailure(context, packet, nil)

	} else if link.nid == nil || link.nid.Type == NidTypeNone {
		link.nid = packet.SrcNid
		link.group.nidMap[nidToString(packet.SrcNid)] = link
		link.group.config.Node.Revision = link.group.config.Revision
		configByte, err := json.Marshal(link.group.config.Node)
		if err != nil {
			logger.E.Fatalln(err)
		}
		contentReply := &proto.AuthSuccess{
			Config: (string)(configByte),
		}
		if err := link.sendSuccess(context, packet, contentReply); err == nil {
			logger.I.Printf("Authenticate success (%s)\n", link.srcIP)
		} else {
			return err
		}
	} else {
		logger.W.Printf("Authenticate failed (%s)\n", link.srcIP)
		return link.sendFailure(context, packet, nil)
	}

	// if hint of assigned is ON, bind node-id as assigned it yet
	if link.group.lockMutex(context) {
		defer link.group.unlockMutex(context)
	}

	if (content.Hint&HintAssigned) != 0 || len(link.group.assigned) == 0 {
		link.assigned = true
		link.group.assigned[nidToString(packet.SrcNid)] = struct{}{}
	}

	return link.sendHint(context)
}

func (link *Link) relayPacket(context *context, packet *proto.SeedAccessor) error {
	if link.lockMutex(context) {
		defer link.unlockMutex(context)
	}
	// IDの確認用にWEBRTC_CONNECT::OFFERパケットとその返事を利用する
	if packet.Channel == ChannelColonio && packet.ModuleChannel == ModuleChannelColonioNodeAccessor {
		switch packet.CommandId {
		case MethodWebrtcConnectOffer:
			var content proto.Offer
			if err := proto3.Unmarshal(packet.Content, &content); err != nil {
				return err
			}
			if content.Type == OfferTypeFirst {
				link.offerID = packet.Id
			}
		}
	}

	if link.group.lockMutex(context) {
		defer link.group.unlockMutex(context)
	}

	if (packet.Mode & ModeReply) != 0x0 {
		if _, ok := link.group.nidMap[nidToString(packet.DstNid)]; ok {
			logger.D.Println("Relay a packet.")
			link.group.nidMap[nidToString(packet.DstNid)].sendPacket(context, packet)
		} else {
			logger.W.Println("A packet dropped.")
		}

	} else {
		// 自分以外のランダムなlinkにパケットを転送
		srcNidStr := nidToString(link.nid)
		siz := len(link.group.assigned)
		if _, ok := link.group.assigned[srcNidStr]; ok {
			siz--
		}
		if siz != 0 {
			i := rand.Intn(siz)
			for dstNidStr := range link.group.assigned {
				if i == 0 {
					return link.group.nidMap[dstNidStr].sendPacket(context, packet)
				} else if dstNidStr != srcNidStr {
					i--
				}
			}

		} else {
			return link.sendHint(context)
		}
	}

	return nil
}

func (link *Link) sendHint(context *context) error {
	var hint uint32
	if link.assigned {
		hint = hint | HintAssigned
	}
	if link.group.isOnlyOne(context, link) {
		hint = hint | HintOnlyone
	}
	content := &proto.Hint{
		Hint: hint,
	}
	contentByte, err := proto3.Marshal(content)
	if err != nil {
		logger.E.Fatalln(err)
	}

	packet := &proto.SeedAccessor{
		DstNid:        link.nid,
		SrcNid:        &proto.NodeID{Type: NidTypeSeed},
		HopCount:      0,
		Id:            0,
		Mode:          ModeExplicit | ModeOneWay,
		Channel:       ChannelColonio,
		ModuleChannel: ModuleChannelColonioSeedAccessor,
		CommandId:     MethodSeedHint,
		Content:       contentByte,
	}

	logger.D.Printf("Send hint.(%d)\n", len(contentByte))
	return link.sendPacket(context, packet)
}

func (link *Link) sendPing(context *context) error {
	packet := &proto.SeedAccessor{
		DstNid:        link.nid,
		SrcNid:        &proto.NodeID{Type: NidTypeSeed},
		HopCount:      0,
		Id:            0,
		Mode:          ModeExplicit | ModeOneWay,
		Channel:       ChannelColonio,
		ModuleChannel: ModuleChannelColonioSeedAccessor,
		CommandId:     MethodSeedPing,
	}

	logger.D.Println("Send ping.")
	return link.sendPacket(context, packet)
}

func (link *Link) sendRequireRandom(context *context) error {
	packet := &proto.SeedAccessor{
		DstNid:        link.nid,
		SrcNid:        &proto.NodeID{Type: NidTypeSeed},
		HopCount:      0,
		Id:            0,
		Mode:          ModeExplicit | ModeOneWay,
		Channel:       ChannelColonio,
		ModuleChannel: ModuleChannelColonioSeedAccessor,
		CommandId:     MethodSeedRequireRandom,
	}

	logger.D.Println("Send require random.")
	return link.sendPacket(context, packet)
}

func (link *Link) sendSuccess(context *context, replyFor *proto.SeedAccessor, content proto3.Message) error {
	contentByte, err := proto3.Marshal(content)
	if err != nil {
		logger.E.Fatalln(err)
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

	logger.D.Println("Send success.")
	return link.sendPacket(context, packet)
}

func (link *Link) sendFailure(context *context, replyFor *proto.SeedAccessor, content proto3.Message) error {
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

	logger.D.Println("Send failure.")
	return link.sendPacket(context, packet)
}

func (link *Link) sendPacket(context *context, packet *proto.SeedAccessor) error {
	if link.lockMutex(context) {
		defer link.unlockMutex(context)
	}
	if link.conn == nil {
		return nil
	}
	link.timeLastSend = time.Now()
	packetBin, err := proto3.Marshal(packet)
	if err != nil {
		logger.E.Fatalln(err)
	}
	frame := ws.NewBinaryFrame(packetBin)
	return ws.WriteFrame(link.conn, frame)
}

func (link *Link) close(context *context) error {
	logger.D.Println("close link")
	if link.lockMutex(context) {
		defer link.unlockMutex(context)
	}
	if link.group != nil {
		if link.group.lockMutex(context) {
			defer link.group.unlockMutex(context)
		}

		if link.nid != nil && link.nid.Type != NidTypeNone {
			nidStr := nidToString(link.nid)
			delete(link.group.nidMap, nidStr)
			delete(link.group.assigned, nidStr)
		}
		delete(link.group.links, link)
		link.group = nil
	}
	if link.conn != nil {
		if err := link.conn.Close(); err != nil {
			return err
		}
		link.conn = nil
	}
	return nil
}

func (link *Link) lockMutex(context *context) bool {
	if _, ok := context.routineLocal[LinkMutex]; ok {
		return false
	} else {
		context.routineLocal[LinkMutex] = true
		link.mutex.Lock()
		return true
	}
}

func (link *Link) unlockMutex(context *context) {
	link.mutex.Unlock()
	delete(context.routineLocal, LinkMutex)
}
