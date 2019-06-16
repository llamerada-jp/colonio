/*
 * Copyright 2019 Yuji Ito <llamerada.jp@gmail.com>
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
	"strconv"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/mailru/easygo/netpoll"
)

type Config struct {
	Revision     float64                `json:"revision"`
	PingInterval int64                  `json:"pingInterval"`
	Timeout      int64                  `json:"timeout"`
	Node         map[string]interface{} `json:"node"`
}

func (c *Config) CastConfig() *Config {
	return c
}

type Link struct {
	group        *Group
	nid          string
	timeCreate   int64
	timeLastRecv int64
	timeLastSend int64
	conn         net.Conn // WebSocket connection.
	assigned     bool
	offerID      string
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

type Packet struct {
	DstNid    string `json:"dst_nid"`
	SrcNid    string `json:"src_nid"`
	ID        string `json:"id"`
	Mode      int    `json:"mode"`
	Channel   int    `json:"channel"`
	CommandID int    `json:"command_id"`
	Content   string `json:"content"`
}

type ContentSeedAuth struct {
	Version string `json:"version"`
	Token   string `json:"token"`
	Hint    string `json:"hint"`
}

type ContentReplySeedAuth struct {
	Config map[string]interface{} `json:"config"`
}

type ContentRelayWebrtcConnectOffer struct {
	PrimeNid  string `json:"prime_nid"`
	SecondNid string `json:"second_nid"`
	Sdp       string `json:"sdp"`
	Type      string `json:"type"`
}

type ContentHint struct {
	Hint string `json:"hint"`
}

type ContentPing struct {
}

type ContentRequireRandom struct {
}

const (
	// プロトコルバージョン
	ProtocolVersion = "A1"

	// Node ID.
	NidNone = ""
	NidSeed = "seed"

	// Packet mode.
	ModeNone      = 0x0000
	ModeReply     = 0x0001
	ModeExplicit  = 0x0002
	ModeOneWay    = 0x0004
	ModeRelaySeed = 0x0008
	ModeNoRetry   = 0x0010

	ChannelNone          = 0
	ChannelSeed          = 1
	ChannelwebrtcConnect = 2

	// Commonly packet method.
	MethodError   = 0xffff
	MethodFailure = 0xfffe
	MethodSuccess = 0xfffd

	MethodSeedAuth          = 1
	MethodSeedHint          = 2
	MethodSeedPing          = 3
	MethodSeedRequireRandom = 4

	MethodWebrtcConnectOffer = 1

	// Offer type of webrtc connect.
	OfferTypeFirst = 0

	// Hint
	HintOnlyone  = 0x0001
	HintAssigned = 0x0002
)

/**
 * 現在時刻(Unix time)をミリ秒単位で取得する。
 * @return ms
 */
func getNowMs() int64 {
	return time.Now().UTC().UnixNano() / 1000000
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

func (seed *Seed) CreateGroup(groupName string, config *Config) *Group {
	if _, ok := seed.groups[groupName]; ok {
		log.Fatal("Duplicate group : " + groupName)
	}
	tickerPeriod := make(chan bool)

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
				group.execEachTick()

			case <-tickerPeriod:
				goto Out
			}
		}

	Out:
		ticker.Stop()
	}()

	seed.groups[groupName] = group
	return group
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
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}

	go seed.pollLink()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}

		seed.poolUpgrade.Schedule(func() {
			err := seed.upgradeSocket(conn)
			if err != nil {
				log.Println(err)
			}
		})
	}
}

func (seed *Seed) upgradeSocket(conn net.Conn) error {
	var group *Group

	upgrader := ws.Upgrader{
		OnRequest: func(uri []byte) error {
			g, err := seed.binder.Bind(string(uri))
			group = g
			return err
		},
	}
	_, err := upgrader.Upgrade(conn)
	if err != nil {
		conn.Close()
		return err
	}

	link := group.createLink(conn)
	seed.linkPoller <- link

	return nil
}

func (seed *Seed) pollLink() {
	poller, err := netpoll.New(nil)
	if err != nil {
		log.Fatal(err)
		// handle error
		return
	}

	for {
		link := <-seed.linkPoller
		desc := netpoll.Must(netpoll.HandleRead(link.conn))
		poller.Start(desc, func(ev netpoll.Event) {
			if ev&netpoll.EventReadHup != 0 {
				poller.Stop(desc)
				link.close()
				return
			}

			seed.poolReceive.Schedule(func() {
				// 受信データが無くなったら切断
				if next, err := link.receive(); !next || err != nil {
					if err != nil {
						log.Fatal(err)
					}
					poller.Stop(desc)
					link.close()
					desc.Close()
				}
			})
		})
	}
}

func (group *Group) createLink(conn net.Conn) *Link {
	link := &Link{
		group:      group,
		timeCreate: getNowMs(),
		conn:       conn,
	}

	group.mutex.Lock()
	defer group.mutex.Unlock()
	group.links[link] = struct{}{}

	return link
}

func (group *Group) removeLink(link *Link) {
	group.mutex.Lock()
	defer group.mutex.Unlock()

	link.group = nil
	if link.nid != "" {
		if _, ok := group.nidMap[link.nid]; ok {
			delete(group.nidMap, link.nid)
		}
		if _, ok := group.assigned[link.nid]; ok {
			delete(group.assigned, link.nid)
		}
	}
	delete(group.links, link)
}

func (group *Group) execEachTick() {
	group.mutex.Lock()
	defer group.mutex.Unlock()

	timeNow := getNowMs()

	for link := range group.links {
		// タイムアウトのチェック
		if link.timeLastRecv+group.config.Timeout < timeNow {
			fmt.Println("timeout")
			link.close()
		}

		if link.timeLastSend+group.config.PingInterval < timeNow &&
			link.timeLastRecv+group.config.PingInterval < timeNow {
			link.sendPing()
		}
	}

	// ランダム接続
	if len(group.nidMap) >= 2 {
		i := rand.Intn(len(group.nidMap))
		for _, link := range group.nidMap {
			if i == 0 {
				link.sendRequireRandom()
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

func (group *Group) isOnlyOne(link *Link) bool {
	_, ok := group.assigned[link.nid]
	return len(group.assigned) == 0 || (len(group.assigned) == 1 && ok)
}

func (link *Link) receive() (bool, error) {
	// Reset the Masked flag, server frames must not be masked as
	// RFC6455 says.
	header, err := ws.ReadHeader(link.conn)
	if err != nil {
		return false, err
	}

	payload := make([]byte, header.Length)
	_, err = io.ReadFull(link.conn, payload)
	if err != nil {
		return false, err
	}
	if header.Masked {
		ws.Cipher(payload, header.Mask, 0)
	}

	switch header.OpCode {
	case ws.OpPing:
		frame := ws.NewPongFrame(payload)
		if err := ws.WriteFrame(link.conn, frame); err != nil {
			return false, err
		}

	case ws.OpText:
		var packet Packet
		if err := json.Unmarshal(payload, &packet); err != nil {
			return false, err
		}

		if packet.DstNid == NidSeed {
			if err := link.receivePacket(&packet); err != nil {
				return false, err
			}

		} else if (packet.Mode & ModeRelaySeed) != 0x0 {
			if err := link.relayPacket(&packet); err != nil {
				return false, err
			}

		} else {
			fmt.Println("packet dropped.")
		}

	case ws.OpClose:
		link.close()
		return false, nil
	}

	return true, nil
}

func (link *Link) receivePacket(packet *Packet) error {
	if packet.Channel == ChannelSeed {
		switch packet.CommandID {
		case MethodSeedAuth:
			return link.recvPacketAuth(packet)

		case MethodSeedPing:
			return nil

		default:
			link.close()
			return errors.New("wrong packet")
		}

	} else {
		link.close()
		return errors.New("wrong packet")
	}
}

func (link *Link) recvPacketAuth(packet *Packet) error {
	var content ContentSeedAuth
	if err := json.Unmarshal([]byte(packet.Content), &content); err != nil {
		return err
	}

	link.group.mutex.Lock()
	defer link.group.mutex.Unlock()

	if l2, ok := link.group.nidMap[packet.SrcNid]; ok && l2 != link {
		// IDが重複している
		return link.sendFailure(packet, struct{}{})

	} else if content.Version != ProtocolVersion {
		// バージョンがサポート外
		return link.sendFailure(packet, struct{}{})

	} else if link.nid == NidNone {
		link.nid = packet.SrcNid
		link.group.nidMap[packet.SrcNid] = link
		contentReply := ContentReplySeedAuth{
			Config: link.group.config.Node,
		}
		contentReply.Config["revision"] = link.group.config.Revision
		if err := link.sendSuccess(packet, contentReply); err != nil {
			return err
		}
	} else {
		return link.sendFailure(packet, struct{}{})
	}

	// if hint of assigned is ON, regist node-id as assigned it yet
	hint, err := strconv.ParseInt(content.Hint, 16, 32)
	if err != nil {
		return err
	}
	if (hint&HintAssigned) != 0 || link.group.isOnlyOne(link) {
		link.assigned = true
		link.group.assigned[packet.SrcNid] = struct{}{}
	}

	return link.sendHint()
}

func (link *Link) relayPacket(packet *Packet) error {
	// IDの確認用にWEBRTC_CONNECT::OFFERパケットとその返事を利用する
	if packet.Channel == ChannelwebrtcConnect {
		switch packet.CommandID {
		case MethodWebrtcConnectOffer:
			var content ContentRelayWebrtcConnectOffer
			if err := json.Unmarshal([]byte(packet.Content), &content); err != nil {
				return err
			}
			t, err := strconv.ParseInt(content.Type, 16, 32)
			if err != nil {
				return err
			}
			if t == OfferTypeFirst {
				link.offerID = packet.ID
			}
		}
	}

	link.group.mutex.Lock()
	defer link.group.mutex.Unlock()

	if (packet.Mode & ModeReply) != 0x0 {
		if _, ok := link.group.nidMap[packet.DstNid]; ok {
			fmt.Println("Relay a packet.")
			link.group.nidMap[packet.DstNid].sendPacket(packet)
		} else {
			fmt.Println("A packet dropped.")
		}

	} else {
		// 自分以外のランダムなlinkにパケットを転送
		srcNid := link.nid
		siz := len(link.group.assigned)
		if _, ok := link.group.assigned[srcNid]; ok {
			siz--
		}
		if siz != 0 {
			i := rand.Intn(siz)
			for dstNid := range link.group.assigned {
				if i == 0 {
					return link.group.nidMap[dstNid].sendPacket(packet)
				} else if dstNid != srcNid {
					i--
				}
			}

		} else {
			return link.sendHint()
		}
	}

	return nil
}

func (link *Link) sendHint() error {
	var hint int64
	if link.assigned {
		hint = hint | HintAssigned
	}
	if link.group.isOnlyOne(link) {
		hint = hint | HintOnlyone
	}
	content := &ContentHint{
		Hint: strconv.FormatInt(hint, 16),
	}
	contentStr, err := json.Marshal(content)
	if err != nil {
		log.Fatal(err)
	}

	packet := &Packet{
		DstNid:    link.nid,
		SrcNid:    NidSeed,
		ID:        "00000000",
		Mode:      ModeExplicit | ModeOneWay,
		Channel:   1, // seed
		CommandID: MethodSeedHint,
		Content:   string(contentStr),
	}

	fmt.Println("Send hint.")
	return link.sendPacket(packet)
}

func (link *Link) sendPing() error {
	content := &ContentPing{}
	contentStr, err := json.Marshal(content)
	if err != nil {
		log.Fatal(err)
	}

	packet := &Packet{
		DstNid:    link.nid,
		SrcNid:    NidSeed,
		ID:        "00000000",
		Mode:      ModeExplicit | ModeOneWay,
		Channel:   1, //seed
		CommandID: MethodSeedPing,
		Content:   string(contentStr),
	}

	fmt.Println("Send ping.")
	return link.sendPacket(packet)
}

func (link *Link) sendRequireRandom() error {
	content := &ContentRequireRandom{}
	contentStr, err := json.Marshal(content)
	if err != nil {
		log.Fatal(err)
	}

	packet := &Packet{
		DstNid:    link.nid,
		SrcNid:    NidSeed,
		ID:        "00000000",
		Mode:      ModeExplicit | ModeOneWay,
		Channel:   1, //seed
		CommandID: MethodSeedRequireRandom,
		Content:   string(contentStr),
	}

	fmt.Println("Send require random.")
	return link.sendPacket(packet)
}

func (link *Link) sendSuccess(replyFor *Packet, content interface{}) error {
	contentStr, err := json.Marshal(content)
	if err != nil {
		log.Fatal(err)
	}

	packet := &Packet{
		DstNid:    replyFor.SrcNid,
		SrcNid:    NidSeed,
		ID:        replyFor.ID,
		Mode:      ModeExplicit | ModeOneWay,
		Channel:   replyFor.Channel,
		CommandID: MethodSuccess,
		Content:   string(contentStr),
	}

	fmt.Println("Send success.")
	return link.sendPacket(packet)
}

func (link *Link) sendFailure(replyFor *Packet, content interface{}) error {
	contentStr, err := json.Marshal(content)
	if err != nil {
		log.Fatal(err)
	}

	packet := &Packet{
		DstNid:    replyFor.SrcNid,
		SrcNid:    NidSeed,
		ID:        replyFor.ID,
		Mode:      ModeExplicit | ModeOneWay,
		Channel:   replyFor.Channel,
		CommandID: MethodFailure,
		Content:   string(contentStr),
	}

	fmt.Println("Send failure.")
	return link.sendPacket(packet)
}

func (link *Link) sendPacket(packet *Packet) error {
	link.timeLastSend = getNowMs()
	packetStr, err := json.Marshal(packet)
	if err != nil {
		log.Fatal(err)
	}
	frame := ws.NewTextFrame(packetStr)
	return ws.WriteFrame(link.conn, frame)
}

func (link *Link) close() error {
	if link.group != nil {
		link.group.mutex.Lock()
		defer link.group.mutex.Unlock()

		if link.nid != "" {
			delete(link.group.nidMap, link.nid)
			delete(link.group.assigned, link.nid)
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
