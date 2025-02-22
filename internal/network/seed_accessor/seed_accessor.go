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
package seed_accessor

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/proto"
	proto3 "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Handler interface {
	SeedAuthorizeFailed()
	SeedChangeState(bool)
	SeedRecvConfig(*config.Cluster)
	SeedRecvPacket(*shared.Packet)
	SeedRequireRandomConnect()
}

type Config struct {
	Ctx         context.Context
	Logger      *slog.Logger
	Transporter SeedTransporter
	Handler     Handler
	URL         string
	LocalNodeID *shared.NodeID
	Token       []byte
	// Interval between retries when a network error is detected
	TripInterval time.Duration
}

type SeedAccessor struct {
	config *Config
	ctx    context.Context
	cancel func()

	// statMtx is for enabled, sessionID, waiting and isNodeOnline
	statMtx   sync.RWMutex
	enabled   bool
	sessionID string
	// shared session timeout of the cluster
	sessionTimeout time.Duration
	waiting        []*proto.SeedPacket
	// isNodeOnline is true if the node have connections to the other nodes (not contain the seed)
	isNodeOnline bool
	// onlyOne is true if this node is the only online node of the seed
	onlyOne bool

	timestampMtx      sync.RWMutex
	livenessTimestamp time.Time
	tripTimestamp     time.Time

	// branch mutexes must lock earlier than statMtx
	connectiveMtx sync.Mutex
	pollMtx       sync.Mutex
	relayMtx      sync.Mutex
}

func NewSeedAccessor(config *Config) *SeedAccessor {
	ctx, cancel := context.WithCancel(config.Ctx)

	sa := &SeedAccessor{
		config:            config,
		ctx:               ctx,
		cancel:            cancel,
		enabled:           true,
		waiting:           make([]*proto.SeedPacket, 0),
		isNodeOnline:      false,
		onlyOne:           false,
		livenessTimestamp: time.Now(),
		tripTimestamp:     time.Unix(0, 0),
	}

	// call round every second or finish when context is done
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-sa.ctx.Done():
				return
			case <-ticker.C:
				sa.timestampMtx.Lock()
				willRun := time.Now().After(sa.tripTimestamp.Add(sa.config.TripInterval))
				sa.timestampMtx.Unlock()
				if willRun {
					sa.subRoutine()
				}
			}
		}
	}()

	return sa
}

// IsAuthenticated indicates whether the seedAccessor is authenticated or not.
// return true if authenticated, otherwise false, contain the state need to re-authenticate.
func (sa *SeedAccessor) IsAuthenticated() bool {
	sa.statMtx.RLock()
	defer sa.statMtx.RUnlock()
	return sa.sessionID != ""
}

// IsOnlyOne indicates whether the node is the only one online of the seed.
func (sa *SeedAccessor) IsOnlyOne() bool {
	sa.statMtx.RLock()
	defer sa.statMtx.RUnlock()
	return sa.onlyOne
}

func (sa *SeedAccessor) SetEnabled(sw bool) {
	sa.statMtx.Lock()
	defer sa.statMtx.Unlock()
	sa.enabled = sw
}

func (sa *SeedAccessor) SetNodeOnlineState(sw bool) {
	sa.statMtx.Lock()
	defer sa.statMtx.Unlock()
	sa.isNodeOnline = sw
}

func (sa *SeedAccessor) RelayPacket(packet *shared.Packet) {
	sa.statMtx.Lock()
	defer sa.statMtx.Unlock()
	sa.waiting = append(sa.waiting, &proto.SeedPacket{
		DstNodeId: packet.DstNodeID.Proto(),
		SrcNodeId: packet.SrcNodeID.Proto(),
		Id:        packet.ID,
		HopCount:  packet.HopCount,
		Mode:      uint32(packet.Mode),
		Content:   packet.Content,
	})
}

func (sa *SeedAccessor) Destruct() {
	sa.statMtx.Lock()
	sa.enabled = false
	sa.waiting = nil
	sa.isNodeOnline = false
	sa.onlyOne = false
	sa.statMtx.Unlock()

	defer sa.cancel()
	for {
		sa.statMtx.RLock()
		haveSession := sa.sessionID != ""
		sa.statMtx.RUnlock()
		if !haveSession {
			return
		}
		sa.close()
		time.Sleep(100 * time.Millisecond)
	}
}

func (sa *SeedAccessor) subRoutine() {
	// check if authentication or disconnect is processing
	runningAuth := !sa.connectiveMtx.TryLock()
	if runningAuth {
		return
	}
	sa.connectiveMtx.Unlock()

	sa.statMtx.RLock()
	enabled := sa.enabled
	sessionID := sa.sessionID
	haveSession := sessionID != ""
	haveWaiting := len(sa.waiting) > 0
	isNodeOnline := sa.isNodeOnline
	sa.statMtx.RUnlock()

	// disconnect if !online and have session
	if !enabled {
		if haveSession {
			sa.close()
		}
		return
	}

	// connect if !haveSession
	if !haveSession {
		sa.authenticate()
		return
	}

	sa.polling(sessionID, isNodeOnline)

	if haveWaiting {
		sa.relay()
	}

	sa.statMtx.Lock()
	sa.timestampMtx.Lock()
	defer func() {
		sa.statMtx.Unlock()
		sa.timestampMtx.Unlock()
	}()
	if sa.sessionTimeout != 0 &&
		sa.sessionID != "" &&
		time.Now().After(sa.livenessTimestamp.Add(sa.sessionTimeout*2)) {

		sa.sessionID = ""
		go sa.config.Handler.SeedChangeState(false)
	}
}

func (sa *SeedAccessor) authenticate() {
	// try lock connectiveMtx to check if authentication or disconnect are processing
	if !sa.connectiveMtx.TryLock() {
		return
	}
	// when sending an authenticate packet, unlock mutex after end of sending process
	sending := false
	defer func() {
		if !sending {
			sa.connectiveMtx.Unlock()
		}
	}()

	sa.statMtx.RLock()
	defer sa.statMtx.RUnlock()
	if !sa.enabled || sa.sessionID != "" {
		return
	}

	packet := &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  sa.config.LocalNodeID.Proto(),
		Token:   sa.config.Token,
	}

	sending = true
	go func() {
		defer sa.connectiveMtx.Unlock()

		resBin, code := sa.send("authenticate", packet)
		if resBin == nil {
			return
		}

		switch code {
		case 0:
			// may network error and should retry later
			return

		case http.StatusOK:
			// success and to be continue
			break

		case http.StatusForbidden:
			// failed to authenticate
			sa.statMtx.Lock()
			sa.enabled = false
			sa.statMtx.Unlock()
			go sa.config.Handler.SeedAuthorizeFailed()
			return

		default:
			// unexpected status code
			sa.config.Logger.Warn("unexpected status code from the seed", slog.Int("code", code))
			return
		}

		var res proto.SeedAuthenticateResponse
		if err := proto3.Unmarshal(resBin, &res); err != nil {
			sa.config.Logger.Warn("failed to unmarshal authenticate response from the seed", slog.String("err", err.Error()))
			return
		}

		var config config.Cluster
		if err := json.Unmarshal(res.Config, &config); err != nil {
			sa.config.Logger.Warn("failed to unmarshal config from the seed", slog.String("err", err.Error()))
			return
		}

		sa.statMtx.Lock()
		defer sa.statMtx.Unlock()
		sa.sessionID = res.SessionId
		sa.sessionTimeout = config.SessionTimeout
		sa.decodeHint(res.Hint)

		go func() {
			sa.config.Handler.SeedRecvConfig(&config)
			sa.config.Handler.SeedChangeState(true)
		}()
	}()
}

func (sa *SeedAccessor) close() {
	// try lock connectiveMtx to check if authentication or disconnect are processing
	if !sa.connectiveMtx.TryLock() {
		return
	}
	// when sending a disconnect packet, unlock mutex after end of sending process
	sending := false
	defer func() {
		if !sending {
			sa.connectiveMtx.Unlock()
		}
	}()

	sa.statMtx.Lock()
	defer sa.statMtx.Unlock()
	if sa.sessionID == "" {
		return
	}

	packet := &proto.SeedClose{
		SessionId: sa.sessionID,
	}

	sending = true
	go func() {
		defer sa.connectiveMtx.Unlock()
		sa.send("close", packet)
		sa.statMtx.Lock()
		defer sa.statMtx.Unlock()
		sa.sessionID = ""
		go sa.config.Handler.SeedChangeState(false)
	}()
}

func (sa *SeedAccessor) polling(sessionID string, isNodeOnline bool) {
	runningPoll := !sa.pollMtx.TryLock()
	if runningPoll {
		return
	}

	packet := &proto.SeedPoll{
		SessionId: sessionID,
		Online:    isNodeOnline,
	}

	go func() {
		defer sa.pollMtx.Unlock()
		resBin, code := sa.send("poll", packet)

		if code != http.StatusOK {
			return
		}

		var res proto.SeedPollResponse
		if err := proto3.Unmarshal(resBin, &res); err != nil {
			sa.config.Logger.Warn("failed to unmarshal poll response from the seed", slog.String("err", err.Error()))
			return
		}

		sa.statMtx.Lock()
		defer sa.statMtx.Unlock()
		sa.sessionID = res.SessionId
		if sa.sessionID == "" {
			panic("sessionID should not be empty")
		}
		sa.decodeHint(res.Hint)

		go func() {
			for _, packet := range res.Packets {
				sa.config.Handler.SeedRecvPacket(&shared.Packet{
					DstNodeID: shared.NewNodeIDFromProto(packet.DstNodeId),
					SrcNodeID: shared.NewNodeIDFromProto(packet.SrcNodeId),
					ID:        packet.Id,
					HopCount:  packet.HopCount,
					Mode:      shared.PacketMode(packet.Mode),
					Content:   packet.Content,
				})
			}
		}()
	}()
}

func (sa *SeedAccessor) relay() {
	runningRelay := !sa.relayMtx.TryLock()
	if runningRelay {
		return
	}

	sa.statMtx.Lock()
	defer sa.statMtx.Unlock()
	packet := &proto.SeedRelay{
		SessionId: sa.sessionID,
		Packets:   sa.waiting,
	}
	sa.waiting = make([]*proto.SeedPacket, 0)

	go func() {
		defer sa.relayMtx.Unlock()
		resBin, code := sa.send("relay", packet)

		if code != http.StatusOK {
			return
		}

		var res proto.SeedRelayResponse
		if err := proto3.Unmarshal(resBin, &res); err != nil {
			sa.config.Logger.Warn("failed to unmarshal relay response from the seed", slog.String("err", err.Error()))
			return
		}

		sa.statMtx.Lock()
		defer sa.statMtx.Unlock()
		sa.decodeHint(res.Hint)
	}()
}

func (sa *SeedAccessor) decodeHint(hint uint32) {
	// stateMtx should be locked

	sa.onlyOne = (hint & shared.HintOnlyOne) != 0

	if (hint & shared.HintRequireRandom) != 0 {
		go sa.config.Handler.SeedRequireRandomConnect()
	}
}

func (sa *SeedAccessor) send(path string, packet protoreflect.ProtoMessage) ([]byte, int) {
	bin, err := proto3.Marshal(packet)
	if err != nil {
		panic(err)
	}

	url, err := url.JoinPath(sa.config.URL, path)
	if err != nil {
		panic(err)
	}

	res, code, err := sa.config.Transporter.Send(sa.ctx, url, bin)
	sa.timestampMtx.Lock()
	defer sa.timestampMtx.Unlock()

	if err != nil {
		sa.config.Logger.Warn("failed to send packet", slog.String("err", err.Error()))
		sa.tripTimestamp = time.Now()
		return nil, 0
	}

	sa.livenessTimestamp = time.Now()
	sa.tripTimestamp = time.Unix(0, 0)

	if code == http.StatusUnauthorized {
		sa.statMtx.Lock()
		authenticated := sa.sessionID != ""
		sa.sessionID = ""
		sa.statMtx.Unlock()
		if authenticated {
			go sa.config.Handler.SeedChangeState(false)
		}
	}

	return res, code
}
