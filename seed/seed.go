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
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/sessions"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	service "github.com/llamerada-jp/colonio/api/colonio/v1alpha/v1alphaconnect"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type ContextKey string

const (
	/**
	 * CONTEXT_REQUEST_KEY is used to describe the request id for logs.
	 * caller module can set the request id for the value to `http.Request.ctx` using this key.
	 */
	CONTEXT_KEY_REQUEST_ID   = ContextKey("requestID")
	CONTEXT_KEY_SESSION      = ContextKey("session")
	DEFAULT_POLLING_INTERVAL = 10 * time.Minute
)

var (
	ErrSession  = errors.New("session error")
	ErrInternal = errors.New("internal error")
)

type node struct {
	timestamp time.Time
	mutex     sync.Mutex
	signal    chan bool
	signals   []*proto.Signal
}

type options struct {
	logger            *slog.Logger
	connectionHandler ConnectionHandler
	multiSeedHandler  MultiSeedHandler
	sessionStore      sessions.Store
	pollingInterval   time.Duration
	connectHandler    http.Handler
}

type ConnectionHandler interface {
	AssignNodeID(ctx context.Context) (*shared.NodeID, error)
	Unassign(nodeID *shared.NodeID)
}

type MultiSeedHandler interface {
	IsAlone(ctx context.Context, nodeID *shared.NodeID) (bool, error)
	RelaySignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error
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

func WithSessionStore(store sessions.Store) optionSetter {
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
	service.UnimplementedSeedServiceHandler
	mutex sync.Mutex
	// map of node-id and node instance
	nodes map[shared.NodeID]*node
}

/**
 * NewSeedHandler creates an http handler to provide colonio's seed.
 * It also starts a go routine for the seed features.
 */
func NewSeed(optionSetters ...optionSetter) *Seed {
	opts := options{
		logger:          slog.Default(),
		pollingInterval: DEFAULT_POLLING_INTERVAL,
	}

	for _, setter := range optionSetters {
		setter(&opts)
	}

	if opts.sessionStore == nil {
		cookieKeyPair := os.Getenv("COLONIO_COOKIE_SECRET_KEY_PAIR")
		if cookieKeyPair == "" {
			panic("please set COLONIO_COOKIE_SECRET_KEY_PAIR")
		}
		cookieStore := sessions.NewCookieStore([]byte(cookieKeyPair))
		cookieStore.MaxAge(int(opts.pollingInterval.Seconds()))
		opts.sessionStore = cookieStore

	}

	return &Seed{
		options: opts,
		mutex:   sync.Mutex{},
		nodes:   make(map[shared.NodeID]*node),
	}
}

func (seed *Seed) RegisterService(mux *http.ServeMux) {
	path, handler := service.NewSeedServiceHandler(seed)
	seed.connectHandler = handler
	mux.Handle(path, seed)
}

type bypass struct {
	http.ResponseWriter
	http.Flusher
	statusCode int
	bodySize   int
}

func (b *bypass) WriteHeader(statusCode int) {
	b.statusCode = statusCode
	b.ResponseWriter.WriteHeader(statusCode)
}

func (b *bypass) Write(data []byte) (int, error) {
	b.bodySize += len(data)
	return b.ResponseWriter.Write(data)
}

func (seed *Seed) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	// create session
	session, err := newSession(seed.sessionStore, request, response)
	if err != nil {
		seed.logger.Error("failed to create session", slog.String("error", err.Error()))
		http.Error(response, "session error", http.StatusInternalServerError)
		return
	}
	nodeID := session.getNodeID()
	if nodeID != nil {
		seed.mutex.Lock()
		if node, ok := seed.nodes[*nodeID]; ok {
			node.mutex.Lock()
			node.timestamp = time.Now()
			node.mutex.Unlock()
		}
		seed.mutex.Unlock()
	}
	ctxWithSession := context.WithValue(request.Context(), CONTEXT_KEY_SESSION, session)

	// generate random request ID
	requestID := fmt.Sprintf("(%016x)", rand.Int63())
	seed.logger.Info("request",
		slog.String("id", requestID),
		slog.String("from", request.RemoteAddr),
		slog.String("method", request.Method),
		slog.String("uri", request.RequestURI),
		slog.Int("contentLength", int(request.ContentLength)))

	bypass := &bypass{
		ResponseWriter: response,
		Flusher:        response.(http.Flusher),
		statusCode:     http.StatusOK,
	}

	defer func() {

		seed.logger.Info("response",
			slog.String("id", requestID),
			slog.Int("statusCode", bypass.statusCode),
			slog.Int("bodySize", bypass.bodySize))
	}()

	ctxWithRequestID := context.WithValue(ctxWithSession, CONTEXT_KEY_REQUEST_ID, requestID)

	seed.connectHandler.ServeHTTP(bypass, request.WithContext(ctxWithRequestID))
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

func (seed *Seed) HandleSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	dstNodeID := shared.NewNodeIDFromProto(signal.GetDstNodeId())
	srcNodeID := shared.NewNodeIDFromProto(signal.GetSrcNodeId())
	if dstNodeID == nil || srcNodeID == nil {
		return fmt.Errorf("invalid packet")
	}

	var node *node
	if relayToNext {
		node = seed.getNextNode(dstNodeID)
	} else {
		node = seed.getExplicitNode(dstNodeID)
	}
	if node == nil {
		return fmt.Errorf("node not found: %s", dstNodeID)
	}

	node.mutex.Lock()
	defer node.mutex.Unlock()

	if node.signal == nil {
		return fmt.Errorf("node is not online: %s", dstNodeID)
	}

	node.signals = append(node.signals, signal)

	if len(node.signal) == 0 {
		node.signal <- true
	}
	return nil
}

func (seed *Seed) AssignNodeID(ctx context.Context, _ *connect.Request[proto.AssignNodeIDRequest]) (*connect.Response[proto.AssignNodeIDResponse], error) {
	session := ctx.Value(CONTEXT_KEY_SESSION).(*session)

	var nodeID *shared.NodeID
	if seed.connectionHandler != nil {
		var err error
		nodeID, err = seed.connectionHandler.AssignNodeID(ctx)
		if err != nil {
			seed.log(ctx).Warn("failed on assign node-id", slog.String("error", err.Error()))
			return nil, ErrInternal
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
		signals:   make([]*proto.Signal, 0),
	}

	seed.mutex.Unlock()

	isAlone, err := seed.isAlone(ctx, nodeID)
	if err != nil {
		return nil, ErrInternal
	}

	session.setNodeID(nodeID)
	if err = session.write(); err != nil {
		seed.log(ctx).Warn("failed to write session", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	seed.log(ctx).Info("node assigned", slog.String("nodeID", nodeID.String()))

	return &connect.Response[proto.AssignNodeIDResponse]{
		Msg: &proto.AssignNodeIDResponse{
			NodeId:  nodeID.Proto(),
			IsAlone: isAlone,
		},
	}, nil
}

func (seed *Seed) SendSignal(ctx context.Context, request *connect.Request[proto.SendSignalRequest]) (*connect.Response[proto.SendSignalResponse], error) {
	session := ctx.Value(CONTEXT_KEY_SESSION).(*session)
	nodeID := session.getNodeID()
	if nodeID == nil {
		seed.log(ctx).Warn("session error (node id is not found)")
		return nil, ErrInternal
	}

	isAlone, err := seed.isAlone(ctx, nodeID)
	if err != nil {
		return nil, ErrInternal
	}

	if !isAlone {
		signal := request.Msg.GetSignal()
		if signal == nil {
			seed.log(ctx).Warn("request error (signal is required)")
			return nil, ErrInternal
		}

		if err := seed.validateSignal(signal, nodeID); err != nil {
			return nil, err
		}

		relayToNext := false
		if signal.GetOffer() != nil && signal.GetOffer().GetType() == proto.SignalOfferType_SIGNAL_OFFER_TYPE_NEXT {
			relayToNext = true
		}

		if seed.multiSeedHandler != nil {
			if err := seed.multiSeedHandler.RelaySignal(ctx, signal, relayToNext); err != nil {
				seed.log(ctx).Warn("failed to relay packet via multi seed", slog.String("error", err.Error()))
				return nil, ErrInternal
			}
		} else {
			if err := seed.HandleSignal(ctx, signal, relayToNext); err != nil {
				seed.log(ctx).Warn("failed to relay signal", slog.String("error", err.Error()))
				return nil, ErrInternal
			}
		}
	}

	if err := session.write(); err != nil {
		seed.log(ctx).Warn("failed to write session", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	return &connect.Response[proto.SendSignalResponse]{
		Msg: &proto.SendSignalResponse{
			IsAlone: isAlone,
		},
	}, nil
}

func (seed *Seed) validateSignal(signal *proto.Signal, srcNodeID *shared.NodeID) error {
	if signal.GetSrcNodeId() == nil {
		return fmt.Errorf("src_node_id is required")
	}
	if !srcNodeID.Equal(shared.NewNodeIDFromProto(signal.GetSrcNodeId())) {
		return fmt.Errorf("src_node_id is invalid")
	}
	if signal.GetDstNodeId() == nil {
		return fmt.Errorf("dst_node_id is required")
	}
	if signal.GetOffer() == nil && signal.GetAnswer() == nil && signal.GetIce() == nil {
		return fmt.Errorf("signal content is required")
	}
	return nil
}

func (seed *Seed) PollSignal(ctx context.Context, _ *connect.Request[proto.PollSignalRequest], stream *connect.ServerStream[proto.PollSignalResponse]) error {
	session := ctx.Value(CONTEXT_KEY_SESSION).(*session)
	nodeID := session.getNodeID()
	if nodeID == nil {
		seed.log(ctx).Warn("session error (node id is not found)")
		return ErrInternal
	}

	node := seed.getExplicitNode(nodeID)
	if node == nil {
		seed.log(ctx).Warn("node instance may be expired")
		return ErrInternal
	}

	signal := node.getSignal()
	if signal == nil {
		return nil
	}

	// send the first packet immediately to notify the connection
	node.send(stream, true)

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-signal:
			live, err := node.send(stream, false)
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
	requestID := ctx.Value(CONTEXT_KEY_REQUEST_ID)
	if requestID == nil {
		return seed.logger
	}

	return seed.logger.With(slog.String("id", requestID.(string)))
}

func (seed *Seed) getExplicitNode(nodeID *shared.NodeID) *node {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	return seed.nodes[*nodeID]
}

func (seed *Seed) getNextNode(nodeID *shared.NodeID) *node {
	seed.mutex.Lock()
	defer seed.mutex.Unlock()

	// TODO: improve performance, this logic is worst: O(n)
	minNodeID := shared.NodeIDNone
	nextNodeID := shared.NodeIDNone
	for n := range seed.nodes {
		if n.Equal(nodeID) {
			continue
		}
		if minNodeID == shared.NodeIDNone || n.Smaller(&minNodeID) {
			minNodeID = n
		}
		if n.Smaller(nodeID) {
			continue
		}
		if nextNodeID == shared.NodeIDNone || n.Smaller(&nextNodeID) {
			nextNodeID = n
		}
	}

	if nextNodeID != shared.NodeIDNone {
		return seed.nodes[nextNodeID]
	}
	if minNodeID != shared.NodeIDNone {
		return seed.nodes[minNodeID]
	}
	return nil
}

func (seed *Seed) isAlone(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
	if seed.multiSeedHandler != nil {
		isAlone, err := seed.multiSeedHandler.IsAlone(ctx, nodeID)
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
		node.mutex.Lock()
		timestamp := node.timestamp
		node.mutex.Unlock()

		if time.Since(timestamp) > seed.pollingInterval {
			seed.logger.Info("node expired", slog.String("nodeID", nodeID.String()))
			node.close()
			delete(seed.nodes, nodeID)
			if seed.connectionHandler != nil {
				seed.connectionHandler.Unassign(&nodeID)
			}
		}
	}

	return nil
}

func (n *node) getSignal() chan bool {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	return n.signal
}

func (n *node) send(stream *connect.ServerStream[proto.PollSignalResponse], force bool) (bool, error) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	// check if the node is still online
	if n.signal == nil {
		return false, nil
	}

	if !force && len(n.signals) == 0 {
		return true, nil
	}

	response := &proto.PollSignalResponse{
		Signals: n.signals,
	}
	n.signals = make([]*proto.Signal, 0)

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
