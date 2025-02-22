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
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/proto"
	"google.golang.org/grpc"
)

type ContextKeyType string

const (
	METADATA_SESSION_NAME    = "colonio-seed-session"
	SESSION_KEY_NODE_ID      = "nodeID"
	DEFAULT_POLLING_INTERVAL = 1 * time.Minute

	/**
	 * CONTEXT_REQUEST_KEY is used to describe the request id for logs.
	 * caller module can set the request id for the value to `http.Request.ctx` using this key.
	 */
	CONTEXT_REQUEST_KEY = ContextKeyType("requestID")
)

var (
	ErrSession  = errors.New("session error")
	ErrInternal = errors.New("internal error")
)

type node struct {
	timestamp time.Time
	mutex     sync.Mutex
	signal    chan bool
	packets   []*proto.SeedPacket
}

type options struct {
	logger            *slog.Logger
	connectionHandler ConnectionHandler
	multiSeedHandler  MultiSeedHandler
	sessionStore      SessionStore
	pollingInterval   time.Duration
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

func WithPollingInterval(interval time.Duration) optionSetter {
	return func(c *options) {
		c.pollingInterval = interval
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
		logger:          slog.Default(),
		pollingInterval: DEFAULT_POLLING_INTERVAL,
	}

	for _, setter := range optionSetters {
		setter(&opts)
	}

	if opts.sessionStore == nil {
		opts.sessionStore = NewSessionMemoryStore(ctx, METADATA_SESSION_NAME, opts.pollingInterval*10)
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

func (seed *Seed) HandlePacket(p *proto.SeedPacket) error {
	dstNodeID := shared.NewNodeIDFromProto(p.GetDstNodeId())
	srcNodeID := shared.NewNodeIDFromProto(p.GetSrcNodeId())
	if dstNodeID == nil || srcNodeID == nil {
		return fmt.Errorf("invalid packet")
	}

	var node *node
	if dstNodeID != srcNodeID {
		node = seed.getNode(dstNodeID)
	}
	if node == nil && p.GetMode()&uint32(shared.PacketModeExplicit) == 0 {
		node = seed.getRandomNode(dstNodeID)
	}
	if node == nil {
		return fmt.Errorf("node not found: %s", dstNodeID)
	}

	node.mutex.Lock()
	defer node.mutex.Unlock()

	if node.signal == nil {
		return fmt.Errorf("node is not online: %s", dstNodeID)
	}

	node.packets = append(node.packets, p)

	if len(node.signal) == 0 {
		node.signal <- true
	}
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

	seed.mutex.Lock()

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
		signal:    make(chan bool, 1),
		packets:   make([]*proto.SeedPacket, 0),
	}

	seed.mutex.Unlock()

	isAlone, err := seed.isAlone(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed on assign node-id")
	}

	seed.sessionStore.Save(ctx, nil, map[string]string{
		SESSION_KEY_NODE_ID: nodeID.String(),
	})

	return &proto.AssignNodeIDResponse{
		NodeId:  nodeID.Proto(),
		IsAlone: isAlone,
	}, nil
}

func (seed *Seed) Relay(ctx context.Context, request *proto.RelayRequest) (*proto.RelayResponse, error) {
	session, err := seed.sessionStore.Get(ctx, nil)
	if err != nil {
		return nil, ErrSession
	}

	nodeID := shared.NewNodeIDFromString(session[SESSION_KEY_NODE_ID])
	if nodeID == nil || !nodeID.IsNormal() {
		return nil, ErrInternal
	}

	for _, packet := range request.GetPackets() {
		srcNodeID := shared.NewNodeIDFromProto(packet.GetSrcNodeId())
		if srcNodeID != nodeID {
			return nil, ErrInternal
		}

		// filter packets which are not related to signaling
		pc := packet.GetContent()
		if pc.GetError() == nil || pc.GetSignalingIce() == nil || pc.GetSignalingOffer() == nil ||
			pc.GetSignalingOfferSuccess() == nil || pc.GetSignalingOfferFailure() == nil {
			return nil, ErrInternal
		}

		if seed.multiSeedHandler != nil {
			if err := seed.multiSeedHandler.RelayPacket(packet); err != nil {
				seed.log(ctx).Warn("failed to relay packet via multi seed", slog.String("error", err.Error()))
				return nil, ErrInternal
			}
		} else {
			if err := seed.HandlePacket(packet); err != nil {
				seed.log(ctx).Warn("failed to relay packet", slog.String("error", err.Error()))
				return nil, ErrInternal
			}
		}
	}

	return &proto.RelayResponse{}, nil
}

func (seed *Seed) PollRelaying(request *proto.PollRelayingRequest,
	stream grpc.ServerStreamingServer[proto.PollRelayingResponse]) error {
	session, err := seed.sessionStore.Get(stream.Context(), stream)
	if err != nil {
		return ErrSession
	}

	nodeID := shared.NewNodeIDFromString(session[SESSION_KEY_NODE_ID])
	if nodeID == nil || !nodeID.IsNormal() {
		return ErrInternal
	}

	node := seed.getNode(nodeID)
	if node == nil {
		return ErrInternal
	}

	for {
		select {
		case <-stream.Context().Done():
			return nil

		case <-node.signal:
			live, err := node.send(stream)
			if err != nil {
				return err
			}
			if !live {
				return nil
			}
		}
	}
}

func (seed *Seed) log(ctx context.Context) *slog.Logger {
	requestID := ctx.Value(CONTEXT_REQUEST_KEY)
	if requestID == nil {
		return seed.logger
	}

	return seed.logger.With(slog.String("id", requestID.(string)))
}

func (seed *Seed) getNode(nodeID *shared.NodeID) *node {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	return seed.nodes[*nodeID]
}

func (seed *Seed) getRandomNode(expectNodeID *shared.NodeID) *node {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	for nodeID, node := range seed.nodes {
		if expectNodeID == nil || *expectNodeID != nodeID {
			return node
		}
	}

	return nil
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
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	for nodeID, node := range seed.nodes {
		if time.Since(node.timestamp) > seed.pollingInterval {
			node.close()
			delete(seed.nodes, nodeID)
		}
	}

	return nil
}

func (n *node) send(stream grpc.ServerStreamingServer[proto.PollRelayingResponse]) (bool, error) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	// check if the node is still online
	if n.signal == nil {
		return false, nil
	}

	if len(n.packets) == 0 {
		return true, nil
	}

	response := &proto.PollRelayingResponse{
		Packets: n.packets,
	}
	n.packets = make([]*proto.SeedPacket, 0)

	if err := stream.Send(response); err != nil {
		return false, err
	}

	return true, nil
}

func (n *node) close() {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	close(n.signal)
	n.signal = nil
}
