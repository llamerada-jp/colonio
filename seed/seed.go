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
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
	proto3 "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ContextKeyType string

/**
 * CONTEXT_REQUEST_KEY is used to describe the request id for logs.
 * caller module can set the request id for the value to `http.Request.ctx` using this key.
 */
const CONTEXT_REQUEST_KEY ContextKeyType = "requestID"

type Authenticator interface {
	Authenticate(token []byte, nodeID string) (bool, error)
}

type node struct {
	timestamp   time.Time
	sessionID   string
	online      bool
	chTimestamp time.Time
	c           chan uint32
}

type packet struct {
	timestamp  time.Time
	stepNodeID *shared.NodeID
	p          *proto.SeedPacket
}

type Seed struct {
	logger  *slog.Logger
	ctx     context.Context
	mutex   sync.Mutex
	running bool
	// map of node-id and node instance
	nodes map[shared.NodeID]*node
	// map of session-id and node-id
	sessions map[string]*shared.NodeID
	// packets to relay
	packets []packet

	configJS       []byte
	sessionTimeout time.Duration
	pollingTimeout time.Duration

	authenticator Authenticator
}

/**
 * NewSeedHandler creates an http handler to provide colonio's seed.
 * It also starts a go routine for the seed features.
 */
func NewSeed(config *config.Cluster, logger *slog.Logger, authenticator Authenticator) (*Seed, http.Handler) {
	if err := config.Validate(); err != nil {
		panic(err)
	}

	configJS, err := json.Marshal(config)
	if err != nil {
		panic(err)
	}

	seed := &Seed{
		logger:         logger,
		mutex:          sync.Mutex{},
		running:        false,
		nodes:          make(map[shared.NodeID]*node),
		sessions:       make(map[string]*shared.NodeID),
		configJS:       configJS,
		sessionTimeout: config.SessionTimeout,
		pollingTimeout: config.PollingTimeout,
		authenticator:  authenticator,
	}

	return seed, seed.makeHandler()
}

func (seed *Seed) WaitForRun() {
	for {
		seed.mutex.Lock()
		running := seed.running
		seed.mutex.Unlock()

		if running {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (seed *Seed) Run(ctx context.Context) {
	seed.mutex.Lock()
	seed.ctx = ctx
	seed.running = true
	seed.mutex.Unlock()

	ticker4Cleanup := time.NewTicker(time.Second)
	ticker4Random := time.NewTicker(10 * time.Second)

	defer func() {
		ticker4Cleanup.Stop()
		ticker4Random.Stop()
		seed.mutex.Lock()
		seed.running = false
		seed.mutex.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker4Cleanup.C:
			if err := seed.cleanup(); err != nil {
				seed.logger.Error("failed on cleanup", slog.String("err", err.Error()))
			}

		case <-ticker4Random.C:
			if err := seed.randomConnect(); err != nil {
				seed.logger.Error("failed on random connect", slog.String("err", err.Error()))
			}
		}
	}
}

func handleHelper[req, res protoreflect.ProtoMessage](seed *Seed, w http.ResponseWriter, r *http.Request, f func(context.Context, req) (res, int, error), buf req) {
	ctx := r.Context()

	if err := decodeRequest(w, r, buf); err != nil {
		seed.log(ctx).Warn("failed on decode request",
			slog.String("err", err.Error()))
		return
	}

	response, code, err := f(ctx, buf)
	if err != nil {
		seed.log(ctx).Warn("failed on request",
			slog.String("err", err.Error()))
	}

	if err := outputResponse(w, response, code); err != nil {
		seed.log(ctx).Warn("failed on output request",
			slog.String("err", err.Error()))
	}
}

func (seed *Seed) log(ctx context.Context) *slog.Logger {
	requestID := ctx.Value(CONTEXT_REQUEST_KEY)
	if requestID == nil {
		return seed.logger
	}

	return seed.logger.With(slog.String("id", requestID.(string)))
}

func decodeRequest(w http.ResponseWriter, r *http.Request, m protoreflect.ProtoMessage) error {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return fmt.Errorf("request method should be `post`")
	}

	if r.ProtoMajor != 2 && r.ProtoMajor != 3 {
		w.WriteHeader(http.StatusPreconditionFailed)
		return fmt.Errorf("request should be HTTP/2 or HTTP/3")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return err
	}

	if err := proto3.Unmarshal(body, m); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return err
	}

	return nil
}

func outputResponse(w http.ResponseWriter, m protoreflect.ProtoMessage, code int) error {
	if code != http.StatusOK && m.ProtoReflect().IsValid() {
		panic(fmt.Sprint("logic error: status code should be OK when have response message ", code, m))
	}

	w.WriteHeader(code)
	if code != http.StatusOK {
		return nil
	}

	response, err := proto3.Marshal(m)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}

	w.Header().Add("Content-Type", "application/octet-stream")

	if _, err := w.Write(response); err != nil {
		return err
	}

	return nil
}

func (seed *Seed) makeHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/authenticate", func(w http.ResponseWriter, r *http.Request) {
		handleHelper(seed, w, r, seed.authenticate, &proto.SeedAuthenticate{})
	})
	mux.HandleFunc("/close", func(w http.ResponseWriter, r *http.Request) {
		handleHelper(seed, w, r, seed.close, &proto.SeedClose{})
	})
	mux.HandleFunc("/relay", func(w http.ResponseWriter, r *http.Request) {
		handleHelper(seed, w, r, seed.relay, &proto.SeedRelay{})
	})
	mux.HandleFunc("/poll", func(w http.ResponseWriter, r *http.Request) {
		handleHelper(seed, w, r, seed.poll, &proto.SeedPoll{})
	})

	return mux
}

func (seed *Seed) authenticate(ctx context.Context, param *proto.SeedAuthenticate) (*proto.SeedAuthenticateResponse, int, error) {
	if param.Version != shared.ProtocolVersion {
		return nil, http.StatusBadRequest, fmt.Errorf("wrong version was specified: %s", param.Version)
	}

	// verify the token
	if seed.authenticator != nil {
		nodeID := shared.NewNodeIDFromProto(param.NodeId)
		verified, err := seed.authenticator.Authenticate(param.Token, nodeID.String())
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}

		if !verified {
			return nil, http.StatusForbidden, fmt.Errorf("verify failed")
		}
	}

	seed.mutex.Lock()
	defer seed.mutex.Unlock()
	nodeID := shared.NewNodeIDFromProto(param.NodeId)

	// when detect duplicate node-id, disconnect existing node and return error to new node
	if _, ok := seed.nodes[*nodeID]; ok {
		seed.disconnect(nodeID, true)
		return nil, http.StatusInternalServerError, fmt.Errorf("detect duplicate node-id: %s", nodeID)
	}

	sessionID := seed.createNode(nodeID)

	return &proto.SeedAuthenticateResponse{
		Hint:      seed.getHint(true),
		Config:    seed.configJS,
		SessionId: sessionID,
	}, http.StatusOK, nil
}

func (seed *Seed) close(ctx context.Context, param *proto.SeedClose) (protoreflect.ProtoMessage, int, error) {
	nodeID, ok := seed.checkSession(param.SessionId)
	// return immediately when session is no exist
	if !ok {
		return nil, http.StatusOK, nil
	}

	return nil, http.StatusOK, seed.disconnect(nodeID, false)
}

func (seed *Seed) relay(ctx context.Context, param *proto.SeedRelay) (*proto.SeedRelayResponse, int, error) {
	nodeID, ok := seed.checkSession(param.SessionId)
	if !ok {
		return nil, http.StatusUnauthorized, nil
	}

	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	for _, p := range param.Packets {
		// TODO: verify the packet

		seed.packets = append(seed.packets, packet{
			timestamp:  time.Now(),
			stepNodeID: nodeID,
			p:          p,
		})

		if (p.Mode & uint32(shared.PacketModeResponse)) != 0 {
			dstNodeID := shared.NewNodeIDFromProto(p.DstNodeId)
			seed.wakeUp(dstNodeID, 0, true)
			continue
		}

		srcNodeID := shared.NewNodeIDFromProto(p.SrcNodeId)
		for chNodeID, node := range seed.nodes {
			if (node.online || seed.countOnline(true) == 0) && !nodeID.Equal(&chNodeID) && !srcNodeID.Equal(&chNodeID) {
				if seed.wakeUp(&chNodeID, 0, true) {
					break
				}
			}
		}
	}

	return &proto.SeedRelayResponse{
		Hint: seed.getHint(true),
	}, http.StatusOK, nil
}

func (seed *Seed) poll(ctx context.Context, param *proto.SeedPoll) (*proto.SeedPollResponse, int, error) {
	nodeID, ok := seed.checkSession(param.SessionId)
	if !ok {
		return nil, http.StatusUnauthorized, nil
	}

	packets := seed.getPackets(nodeID, param.Online, false)
	if len(packets) != 0 {
		return &proto.SeedPollResponse{
			Hint:      seed.getHint(false),
			SessionId: param.SessionId,
			Packets:   packets,
		}, http.StatusOK, nil
	}

	ch, err := seed.assignChannel(nodeID, param.Online)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer seed.closeChannel(nodeID, false)

	select {
	case <-ctx.Done():
		return &proto.SeedPollResponse{
			Hint:      seed.getHint(false),
			SessionId: param.SessionId,
		}, http.StatusOK, nil

	case <-seed.ctx.Done():
		return &proto.SeedPollResponse{
			Hint:      seed.getHint(false),
			SessionId: param.SessionId,
		}, http.StatusOK, nil

	case hint, ok := <-ch:
		if !ok {
			return &proto.SeedPollResponse{
				Hint:      seed.getHint(false),
				SessionId: param.SessionId,
			}, http.StatusOK, nil
		}

		if hint == shared.HintRequireRandom {
			seed.log(ctx).Info("require random")
		}

		return &proto.SeedPollResponse{
			Hint:      hint | seed.getHint(false),
			SessionId: param.SessionId,
			Packets:   seed.getPackets(nodeID, param.Online, false),
		}, http.StatusOK, nil
	}
}

func (seed *Seed) getPackets(nodeID *shared.NodeID, online bool, locked bool) []*proto.SeedPacket {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	packets := make([]*proto.SeedPacket, 0)
	rest := make([]packet, 0)

	for _, packet := range seed.packets {
		dstNodeID := shared.NewNodeIDFromProto(packet.p.DstNodeId)
		srcNodeID := shared.NewNodeIDFromProto(packet.p.SrcNodeId)

		if nodeID.Equal(srcNodeID) || nodeID.Equal(packet.stepNodeID) {
			// skip if receiver node is equal to source node
			rest = append(rest, packet)

		} else if nodeID.Equal(dstNodeID) {
			// ok if receiver node is equal to destination node
			packets = append(packets, packet.p)

		} else if (packet.p.Mode & uint32(shared.PacketModeResponse)) != 0 {
			// skip if the packet is response mode and receiver is not equal to destination
			rest = append(rest, packet)

		} else if online || seed.countOnline(true) == 0 {
			packets = append(packets, packet.p)

		} else {
			rest = append(rest, packet)
		}
	}

	seed.packets = rest

	return packets
}

func (seed *Seed) wakeUp(nodeID *shared.NodeID, hint uint32, locked bool) bool {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	node, ok := seed.nodes[*nodeID]
	if !ok || node.c == nil {
		return false
	}

	node.c <- hint
	seed.closeChannel(nodeID, true)

	return true
}

func (seed *Seed) cleanup() error {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	now := time.Now()
	for nodeID, node := range seed.nodes {
		// close timeout channels
		if now.After(node.chTimestamp.Add(seed.pollingTimeout)) {
			seed.closeChannel(&nodeID, true)
		}

		// disconnect node if it had pass keep alive timeout
		if now.After(node.timestamp.Add(seed.sessionTimeout)) {
			seed.disconnect(&nodeID, true)
		}
	}

	// drop timeout packets
	keep := make([]packet, 0)
	for _, packet := range seed.packets {
		if now.After(packet.timestamp.Add(seed.pollingTimeout)) {
			continue
		}
		keep = append(keep, packet)
	}
	seed.packets = keep

	return nil
}

func (seed *Seed) countChannels(locked bool) int {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	count := 0
	for _, node := range seed.nodes {
		if node.c != nil {
			count++
		}
	}

	return count
}

func (seed *Seed) countOnline(locked bool) int {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	count := 0
	for _, node := range seed.nodes {
		if node.c != nil && node.online {
			count++
		}
	}

	return count
}

func (seed *Seed) randomConnect() error {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	// skip if there are few online channels
	if seed.countOnline(true) < 2 {
		return nil
	}

	for nodeID := range seed.nodes {
		if seed.wakeUp(&nodeID, shared.HintRequireRandom, true) {
			return nil
		}
	}

	return nil
}

func (seed *Seed) checkSession(sessionID string) (*shared.NodeID, bool) {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	nodeID, ok := seed.sessions[sessionID]
	if !ok {
		return nil, false
	}

	node, ok := seed.nodes[*nodeID]
	if !ok {
		panic("logic error: node should be exist when session exits")
	}

	node.timestamp = time.Now()

	return nodeID, true
}

func (seed *Seed) createNode(nodeID *shared.NodeID) string {
	// should be locked
	if _, ok := seed.nodes[*nodeID]; ok {
		panic("logic error: duplicate node-id in nodes")
	}

	sessionID := seed.assignSessionID(nodeID)
	seed.nodes[*nodeID] = &node{
		timestamp: time.Now(),
		sessionID: sessionID,
	}

	return sessionID
}

func (seed *Seed) assignSessionID(nodeID *shared.NodeID) string {
	// should be locked
	for {
		// generate new session-id
		sessionID := fmt.Sprintf("%016x%016x%016x%016x", rand.Uint64(), rand.Uint64(), rand.Uint64(), rand.Uint64())

		// retry if duplicated
		if _, ok := seed.sessions[sessionID]; ok {
			continue
		}

		// assign new session id
		seed.sessions[sessionID] = nodeID
		return sessionID
	}
}

func (seed *Seed) closeChannel(nodeID *shared.NodeID, locked bool) {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	node, ok := seed.nodes[*nodeID]
	if !ok || node.c == nil {
		return
	}

	close(node.c)
	node.c = nil
}

func (seed *Seed) getHint(locked bool) uint32 {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	hint := uint32(0)

	if len(seed.nodes) == 1 {
		hint = shared.HintOnlyOne
	}

	return hint
}

func (seed *Seed) disconnect(nodeID *shared.NodeID, locked bool) error {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	seed.closeChannel(nodeID, true)

	node, ok := seed.nodes[*nodeID]
	if !ok {
		return nil
	}
	delete(seed.nodes, *nodeID)

	if _, ok = seed.sessions[node.sessionID]; !ok {
		panic("logic error: session should exist when the node exists")
	}
	delete(seed.sessions, node.sessionID)

	return nil
}

func (seed *Seed) assignChannel(nodeID *shared.NodeID, online bool) (chan uint32, error) {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	node, ok := seed.nodes[*nodeID]
	if !ok {
		return nil, fmt.Errorf("logic error: there is the node to assign a channel")
	}

	for i := 0; i < 10; i++ {
		if node.c == nil {
			break
		} else {
			// avoid duplicate channel error by the lock timing
			if i == 9 {
				return nil, fmt.Errorf("duplicate channel")
			} else {
				seed.mutex.Unlock()
				time.Sleep(100 * time.Millisecond)
				seed.mutex.Lock()
			}
		}
	}

	c := make(chan uint32)
	node.chTimestamp = time.Now()
	node.c = c
	node.online = online

	return c, nil
}
