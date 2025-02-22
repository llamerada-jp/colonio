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
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/proto"
	"google.golang.org/grpc"
)

const METADATA_KEY = "colonio-seed-metadata"

type ContextKeyType string

/**
 * CONTEXT_REQUEST_KEY is used to describe the request id for logs.
 * caller module can set the request id for the value to `http.Request.ctx` using this key.
 */
const CONTEXT_REQUEST_KEY ContextKeyType = "requestID"

type node struct {
	timestamp time.Time
	mutex     sync.Mutex
	packets   []*proto.SeedPacket
}

type options struct {
	logger            *slog.Logger
	connectionHandler ConnectionHandler
	multiSeedHandler  MultiSeedHandler
	sessionStore      SessionStore
}

type ConnectionHandler interface {
	AssignNodeID(ctx context.Context) (*shared.NodeID, error)
	Unassign(nodeID *shared.NodeID)
}

type MultiSeedHandler interface {
	IsAlone(nodeID *shared.NodeID) (bool, error)
	RelayPacket(p *proto.SeedPacket) error
}

type optionSetter func(*options)

func WithLogger(logger *slog.Logger) optionSetter {
	return func(c *options) {
		c.logger = logger
	}
}

func WithConnectionHandler(handler ConnectionHandler) optionSetter {
	return func(c *options) {
		c.connectionHandler = handler
	}
}

func WithMultiSeedHandler(handler MultiSeedHandler) optionSetter {
	return func(c *options) {
		c.multiSeedHandler = handler
	}
}

func WithSessionStore(store SessionStore) optionSetter {
	return func(c *options) {
		c.sessionStore = store
	}
}

type Seed struct {
	options
	proto.UnimplementedSeedServer
	mutex sync.Mutex
	// map of node-id and node instance
	nodes map[shared.NodeID]*node
}

/**
 * NewSeedHandler creates an http handler to provide colonio's seed.
 * It also starts a go routine for the seed features.
 */
func NewSeed(ctx context.Context, optionSetters ...optionSetter) *Seed {
	opts := options{
		logger: slog.Default(),
	}

	for _, setter := range optionSetters {
		setter(&opts)
	}

	if opts.sessionStore == nil {
		opts.sessionStore = NewSessionMemoryStore(ctx, 10*time.Minute)
	}

	return &Seed{
		options: opts,
		mutex:   sync.Mutex{},
		nodes:   make(map[shared.NodeID]*node),
	}
}

func (seed *Seed) RegisterGRPCServer(s *grpc.Server) {
	proto.RegisterSeedServer(s, seed)
}

func (seed *Seed) Run(ctx context.Context) {
	ticker4Cleanup := time.NewTicker(time.Second)

	defer func() {
		ticker4Cleanup.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker4Cleanup.C:
			if err := seed.cleanup(); err != nil {
				seed.logger.Error("failed on cleanup", slog.String("err", err.Error()))
			}
		}
	}
}

func (seed *Seed) RelayPacket(p *proto.SeedPacket) error {
	panic("not implemented")
	return nil
}

func (seed *Seed) AssignNodeID(ctx context.Context, _ *proto.AssignNodeIDRequest) (*proto.AssignNodeIDResponse, error) {
	var nodeID *shared.NodeID
	if seed.connectionHandler != nil {
		var err error
		nodeID, err = seed.connectionHandler.AssignNodeID(ctx)
		if err != nil {
			seed.log(ctx).Warn("failed on assign node-id", slog.String("error", err.Error()))
			return nil, fmt.Errorf("failed on assign node-id")
		}
	}

	seed.bindNodeID(nodeID)

	isAlone, err := seed.isAlone(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed on assign node-id")
	}

	return &proto.AssignNodeIDResponse{
		NodeId:  nodeID.Proto(),
		IsAlone: isAlone,
	}, nil
}

func (seed *Seed) Relay(context.Context, *proto.RelayRequest) (*proto.RelayResponse, error) {

}

func (seed *Seed) PollRelaying(*proto.PollRelayingRequest, grpc.ServerStreamingServer[proto.PollRelayingResponse]) error {

}

func (seed *Seed) log(ctx context.Context) *slog.Logger {
	requestID := ctx.Value(CONTEXT_REQUEST_KEY)
	if requestID == nil {
		return seed.logger
	}

	return seed.logger.With(slog.String("id", requestID.(string)))
}

func (seed *Seed) bindNodeID(nodeID *shared.NodeID) {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	if nodeID != nil {
		// check duplicate node-id if it is specified
		if _, ok := seed.nodes[*nodeID]; ok {
			panic("logic error: duplicate node-id in nodes")
		}
	} else {
		// generate new node-id if it is not specified
		for {
			nodeID = shared.NewRandomNodeID()
			if _, ok := seed.nodes[*nodeID]; !ok {
				break
			}
		}
	}

	seed.nodes[*nodeID] = &node{
		timestamp: time.Now(),
		packets:   make([]*proto.SeedPacket, 0),
	}
}

func (seed *Seed) getNode(nodeID *shared.NodeID) *node {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	return seed.nodes[*nodeID]
}

func (seed *Seed) isAlone(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
	if seed.multiSeedHandler != nil {
		isAlone, err := seed.multiSeedHandler.IsAlone(nodeID)
		if err != nil {
			seed.log(ctx).Warn("failed on check alone", slog.String("err", err.Error()))
		}
		return isAlone, err
	}

	seed.mutex.Lock()
	defer seed.mutex.Unlock()
	return len(seed.nodes) == 0 || (len(seed.nodes) == 1 && seed.nodes[*nodeID] != nil), nil
}

func (seed *Seed) cleanup() error {
	panic("not implemented")
}

/*

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

func (seed *Seed) authenticate(ctx context.Context, param *proto.SeedAuthenticate) (*proto.SeedAuthenticateResponse, int, error) {
	if param.Version != shared.ProtocolVersion {
		return nil, http.StatusBadRequest, fmt.Errorf("wrong version was specified: %s", param.Version)
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

//*/
