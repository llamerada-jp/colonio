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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/go/proto"
	proto3 "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ContextKeyType string

/**
 * CONTEXT_REQUEST_KEY is used to describe the request id for logs.
 * caller module can set the request id for the value to `http.Request.ctx` using this key.
 */
const CONTEXT_REQUEST_KEY ContextKeyType = "requestID"

/**
 * TokenVerifier is an interface to implement the function of verifying the token received from the node.
 */
type TokenVerifier interface {
	Verify(token string) (bool, error)
}

type node struct {
	timestamp   time.Time
	sessionID   string
	online      bool
	chTimestamp time.Time
	c           chan uint32
}

type packet struct {
	timestamp time.Time
	stepNid   string
	p         *proto.SeedPacket
}

type Seed struct {
	Handler http.Handler

	ctx   context.Context
	mutex sync.Mutex
	// map of node-id and node instance
	nodes map[string]*node
	// map of session-id and node-id
	sessions map[string]string
	// packets to relay
	packets []packet

	config         string
	sessionTimeout time.Duration
	pollingTimeout time.Duration

	verifier TokenVerifier
}

/**
 * NewSeedHandler creates an http handler to provide colonio's seed.
 * It also starts a go routine for the seed features.
 */
func NewSeed(config *Config, verifier TokenVerifier) (*Seed, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	nodeConfig := config.Node
	nodeConfigJS, err := json.Marshal(nodeConfig)
	if err != nil {
		return nil, err
	}

	seed := &Seed{
		mutex:          sync.Mutex{},
		nodes:          make(map[string]*node),
		sessions:       make(map[string]string),
		config:         string(nodeConfigJS),
		sessionTimeout: time.Duration(config.SessionTimeout) * time.Millisecond,
		pollingTimeout: time.Duration(config.PollingTimeout) * time.Millisecond,
		verifier:       verifier,
	}

	mux := http.NewServeMux()
	seed.Handler = mux
	// TODO: output client information for logs
	mux.HandleFunc("/authenticate", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value(CONTEXT_REQUEST_KEY)
		if requestID == nil {
			requestID = ""
		}

		request := &proto.SeedAuthenticate{}
		if err := decodeRequest(w, r, request); err != nil {
			log.Println(requestID, err)
			return
		}

		response, code, err := seed.authenticate(request)
		if err != nil {
			log.Println(requestID, err)
		}

		if err := outputResponse(w, response, code); err != nil {
			log.Println(requestID, err)
		}
	})

	mux.HandleFunc("/close", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value(CONTEXT_REQUEST_KEY)
		if requestID == nil {
			requestID = ""
		}

		request := &proto.SeedClose{}
		if err := decodeRequest(w, r, request); err != nil {
			log.Println(requestID, err)
			return
		}

		seed.close(request)

		if err := outputResponse(w, nil, http.StatusOK); err != nil {
			log.Println(requestID, err)
		}
	})

	mux.HandleFunc("/relay", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value(CONTEXT_REQUEST_KEY)
		if requestID == nil {
			requestID = ""
		}

		request := &proto.SeedRelay{}
		if err := decodeRequest(w, r, request); err != nil {
			log.Println(requestID, err)
			return
		}

		response, code, err := seed.relay(request)
		if err != nil {
			log.Println(requestID, err)
		}

		if err := outputResponse(w, response, code); err != nil {
			log.Println(requestID, err)
		}
	})

	mux.HandleFunc("/poll", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value(CONTEXT_REQUEST_KEY)
		if requestID == nil {
			requestID = ""
		}

		request := &proto.SeedPoll{}
		if err := decodeRequest(w, r, request); err != nil {
			log.Println(requestID, err)
			return
		}

		response, code, err := seed.poll(r.Context(), request)
		if err != nil {
			log.Println(requestID, err)
		}

		if err := outputResponse(w, response, code); err != nil {
			log.Println(requestID, err)
		}
	})

	return seed, nil
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
		log.Fatal("logic error: status code should be OK when have response message ", code, m)
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

func (seed *Seed) Start(ctx context.Context) {
	seed.ctx = ctx
	go func() {
		ticker4Cleanup := time.NewTicker(time.Second)
		defer ticker4Cleanup.Stop()
		ticker4Random := time.NewTicker(10 * time.Second)
		defer ticker4Random.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker4Cleanup.C:
				if err := seed.cleanup(); err != nil {
					log.Println("failed on cleanup:", err)
				}

			case <-ticker4Random.C:
				if err := seed.randomConnect(); err != nil {
					log.Println("failed on random connect:", err)
				}
			}
		}
	}()
}

func (seed *Seed) authenticate(param *proto.SeedAuthenticate) (*proto.SeedAuthenticateResponse, int, error) {
	if param.Version != ProtocolVersion {
		return nil, http.StatusBadRequest, fmt.Errorf("wrong version was specified: %s", param.Version)
	}

	// verify the token
	if seed.verifier != nil {
		verified, err := seed.verifier.Verify(param.Token)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}

		if !verified {
			return nil, http.StatusUnauthorized, fmt.Errorf("verify failed")
		}
	}

	seed.mutex.Lock()
	defer seed.mutex.Unlock()
	nid := nidToString(param.Nid)

	// when detect duplicate node-id, disconnect existing node and return error to new node
	if _, ok := seed.nodes[nid]; ok {
		seed.disconnect(nid, true)
		return nil, http.StatusInternalServerError, fmt.Errorf("detect duplicate node-id: %s", nid)
	}

	sessionID := seed.createNode(nid)

	return &proto.SeedAuthenticateResponse{
		Hint:      seed.getHint(true),
		Config:    seed.config,
		SessionId: sessionID,
	}, http.StatusOK, nil
}

func (seed *Seed) close(param *proto.SeedClose) {
	nid, ok := seed.checkSession(param.SessionId)
	// return immediately when session is no exist
	if !ok {
		return
	}

	seed.disconnect(nid, false)
}

func (seed *Seed) relay(param *proto.SeedRelay) (*proto.SeedRelayResponse, int, error) {
	nid, ok := seed.checkSession(param.SessionId)
	if !ok {
		return nil, http.StatusUnauthorized, nil
	}

	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	for _, p := range param.Packets {
		// TODO: verify the packet

		seed.packets = append(seed.packets, packet{
			timestamp: time.Now(),
			stepNid:   nid,
			p:         p,
		})

		if (p.Mode & ModeResponse) != 0 {
			dstNid := nidToString(p.DstNid)
			seed.wakeUp(dstNid, 0, true)
			continue
		}

		srcNid := nidToString(p.SrcNid)
		for chNid, node := range seed.nodes {
			if (node.online || seed.countOnline(true) == 0) && chNid != nid && chNid != srcNid {
				if seed.wakeUp(chNid, 0, true) {
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
	requestID := ctx.Value(CONTEXT_REQUEST_KEY)
	if requestID == nil {
		requestID = ""
	}

	nid, ok := seed.checkSession(param.SessionId)
	if !ok {
		return nil, http.StatusUnauthorized, nil
	}

	packets := seed.getPackets(nid, param.Online, false)
	if len(packets) != 0 {
		return &proto.SeedPollResponse{
			Hint:      seed.getHint(false),
			SessionId: param.SessionId,
			Packets:   packets,
		}, http.StatusOK, nil
	}

	ch, err := seed.assignChannel(nid, param.Online)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer seed.closeChannel(nid, false)

	select {
	case <-ctx.Done():
		return nil, http.StatusOK, nil

	case <-seed.ctx.Done():
		return &proto.SeedPollResponse{
			Hint:      seed.getHint(false),
			SessionId: param.SessionId,
		}, http.StatusOK, nil

	case hint, ok := <-ch:
		if !ok {
			return nil, http.StatusOK, nil
		}

		if hint == HintRequireRandom {
			log.Println(requestID, "require random")
		}

		return &proto.SeedPollResponse{
			Hint:      hint | seed.getHint(false),
			SessionId: param.SessionId,
			Packets:   seed.getPackets(nid, param.Online, false),
		}, http.StatusOK, nil
	}
}

func (seed *Seed) getPackets(nid string, online bool, locked bool) []*proto.SeedPacket {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	packets := make([]*proto.SeedPacket, 0)
	rest := make([]packet, 0)

	for _, packet := range seed.packets {
		dstNid := nidToString(packet.p.DstNid)
		srcNid := nidToString(packet.p.SrcNid)

		if nid == srcNid || nid == packet.stepNid {
			// skip if receiver node is equal to source node
			rest = append(rest, packet)

		} else if nid == dstNid {
			// ok if receiver node is equal to destination node
			packets = append(packets, packet.p)

		} else if (packet.p.Mode & ModeResponse) != 0 {
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

func (seed *Seed) wakeUp(nid string, hint uint32, locked bool) bool {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	node, ok := seed.nodes[nid]
	if !ok || node.c == nil {
		return false
	}

	node.c <- hint
	seed.closeChannel(nid, true)

	return true
}

func (seed *Seed) cleanup() error {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	now := time.Now()
	for nid, node := range seed.nodes {
		// close timeout channels
		if now.After(node.chTimestamp.Add(seed.pollingTimeout)) {
			seed.closeChannel(nid, true)
		}

		// disconnect node if it had pass keep alive timeout
		if now.After(node.timestamp.Add(seed.sessionTimeout)) {
			seed.disconnect(nid, true)
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

	for nid := range seed.nodes {
		if seed.wakeUp(nid, HintRequireRandom, true) {
			return nil
		}
	}

	return nil
}

func (seed *Seed) checkSession(sessionID string) (string, bool) {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	nid, ok := seed.sessions[sessionID]
	if !ok {
		return "", false
	}

	node, ok := seed.nodes[nid]
	if !ok {
		log.Fatal("logic error: node should be exist when session exits")
	}

	node.timestamp = time.Now()

	return nid, true
}

func (seed *Seed) createNode(nid string) string {
	// should be locked
	if _, ok := seed.nodes[nid]; ok {
		log.Fatal("logic error: duplicate node-id in nodes")
	}

	sessionID := seed.assignSessionID(nid)
	seed.nodes[nid] = &node{
		timestamp: time.Now(),
		sessionID: sessionID,
	}

	return sessionID
}

func (seed *Seed) assignSessionID(nid string) string {
	// should be locked
	for {
		// generate new session-id
		sessionID := fmt.Sprintf("%016x%016x%016x%016x", rand.Uint64(), rand.Uint64(), rand.Uint64(), rand.Uint64())

		// retry if duplicated
		if _, ok := seed.sessions[sessionID]; ok {
			continue
		}

		// assign new session id
		seed.sessions[sessionID] = nid
		return sessionID
	}
}

func (seed *Seed) closeChannel(nid string, locked bool) {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	node, ok := seed.nodes[nid]
	if !ok || node.c == nil {
		return
	}

	close(node.c)
	node.c = nil
	node.online = false
}

func (seed *Seed) getHint(locked bool) uint32 {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	hint := uint32(0)

	if len(seed.nodes) == 1 {
		hint = HintOnlyOne
	}

	return hint
}

func (seed *Seed) disconnect(nid string, locked bool) error {
	if !locked {
		seed.mutex.Lock()
		defer seed.mutex.Unlock()
	}

	seed.closeChannel(nid, true)

	node, ok := seed.nodes[nid]
	if !ok {
		return nil
	}
	delete(seed.nodes, nid)

	if _, ok = seed.sessions[node.sessionID]; !ok {
		log.Fatal("logic error: session should exist when the node exists")
	}
	delete(seed.sessions, node.sessionID)

	return nil
}

func (seed *Seed) assignChannel(nid string, online bool) (chan uint32, error) {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	node, ok := seed.nodes[nid]
	if !ok {
		return nil, fmt.Errorf("logic error: there is the node to assign a channel")
	}

	if node.c != nil {
		return nil, fmt.Errorf("duplicate channel")
	}

	c := make(chan uint32)
	node.chTimestamp = time.Now()
	node.c = c
	node.online = online

	return c, nil
}
