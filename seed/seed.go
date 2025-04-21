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
	nodeID    shared.NodeID
	timestamp time.Time
	mutex     sync.Mutex
	term      chan struct{}
	signals   []*proto.Signal
	stream    *connect.ServerStream[proto.PollSignalResponse]
}

type options struct {
	logger            *slog.Logger
	assignmentHandler AssignmentHandler
	multiSeedHandler  MultiSeedHandler
	sessionStore      sessions.Store
	pollingInterval   time.Duration
	connectHandler    http.Handler
}

type AssignmentHandler interface {
	AssignNode(ctx context.Context) (*shared.NodeID, error)
	UnassignNode(nodeID *shared.NodeID)
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

func WithAssignmentHandler(handler AssignmentHandler) optionSetter {
	return func(c *options) {
		c.assignmentHandler = handler
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

func (s *Seed) RegisterService(mux *http.ServeMux) {
	path, handler := service.NewSeedServiceHandler(s)
	s.connectHandler = handler
	mux.Handle(path, s)
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

func (s *Seed) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	// create session
	session, err := newSession(s.sessionStore, request, response)
	if err != nil {
		s.logger.Error("failed to create session", slog.String("error", err.Error()))
		http.Error(response, "session error", http.StatusInternalServerError)
		return
	}
	nodeID := session.getNodeID()
	if nodeID != nil {
		s.mutex.Lock()
		if node, ok := s.nodes[*nodeID]; ok {
			node.mutex.Lock()
			node.timestamp = time.Now()
			node.mutex.Unlock()
		}
		s.mutex.Unlock()
	}
	ctxWithSession := context.WithValue(request.Context(), CONTEXT_KEY_SESSION, session)

	// generate random request ID
	requestID := fmt.Sprintf("(%016x)", rand.Int63())
	s.logger.Info("request",
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
		s.logger.Info("response",
			slog.String("id", requestID),
			slog.Int("statusCode", bypass.statusCode),
			slog.Int("bodySize", bypass.bodySize))
	}()

	ctxWithRequestID := context.WithValue(ctxWithSession, CONTEXT_KEY_REQUEST_ID, requestID)

	s.connectHandler.ServeHTTP(bypass, request.WithContext(ctxWithRequestID))
}

func (s *Seed) Run(ctx context.Context) {
	ticker4Cleanup := time.NewTicker(time.Second)

	defer func() {
		ticker4Cleanup.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker4Cleanup.C:
			if err := s.cleanup(); err != nil {
				s.logger.Error("failed on cleanup", slog.String("err", err.Error()))
			}
		}
	}
}

func (s *Seed) HandleSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	dstNodeID := shared.NewNodeIDFromProto(signal.GetDstNodeId())
	srcNodeID := shared.NewNodeIDFromProto(signal.GetSrcNodeId())
	if dstNodeID == nil || srcNodeID == nil {
		return fmt.Errorf("invalid packet")
	}

	var node *node
	if relayToNext {
		node = s.getNextNode(dstNodeID)
	} else {
		node = s.getExplicitNode(dstNodeID)
	}
	if node == nil {
		return fmt.Errorf("node not found: %s", dstNodeID)
	}

	node.mutex.Lock()
	defer node.mutex.Unlock()

	if node.stream != nil {
		if err := node.stream.Send(&proto.PollSignalResponse{
			Signals: []*proto.Signal{signal},
		}); err != nil {
			return fmt.Errorf("failed to send signal: %w", err)
		}
		return nil
	}

	if node.signals != nil {
		if len(node.signals) > 100 {
			return fmt.Errorf("too many signals: %s -> %s", srcNodeID.String(), node.nodeID.String())
		}
		node.signals = append(node.signals, signal)
		return nil
	}

	return nil
}

func (s *Seed) AssignNode(ctx context.Context, _ *connect.Request[proto.AssignNodeRequest]) (*connect.Response[proto.AssignNodeResponse], error) {
	session := ctx.Value(CONTEXT_KEY_SESSION).(*session)

	var nodeID *shared.NodeID
	if s.assignmentHandler != nil {
		var err error
		nodeID, err = s.assignmentHandler.AssignNode(ctx)
		if err != nil {
			s.log(ctx).Warn("failed on assign node-id", slog.String("error", err.Error()))
			return nil, ErrInternal
		}
	}

	s.mutex.Lock()

	if nodeID != nil {
		// check duplicate node-id if it is specified
		if _, ok := s.nodes[*nodeID]; ok {
			panic("logic error: duplicate node-id in nodes")
		}
	} else {
		// generate new node-id if it is not specified
		for {
			nodeID = shared.NewRandomNodeID()
			if _, ok := s.nodes[*nodeID]; !ok {
				break
			}
		}
	}

	s.nodes[*nodeID] = &node{
		nodeID:    *nodeID,
		timestamp: time.Now(),
		term:      make(chan struct{}, 1),
		signals:   make([]*proto.Signal, 0),
	}

	s.mutex.Unlock()

	isAlone, err := s.isAlone(ctx, nodeID)
	if err != nil {
		return nil, ErrInternal
	}

	session.setNodeID(nodeID)
	if err = session.write(); err != nil {
		s.log(ctx).Warn("failed to write session", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	s.log(ctx).Info("node assigned", slog.String("nodeID", nodeID.String()))

	return &connect.Response[proto.AssignNodeResponse]{
		Msg: &proto.AssignNodeResponse{
			NodeId:  nodeID.Proto(),
			IsAlone: isAlone,
		},
	}, nil
}

func (s *Seed) UnassignNode(ctx context.Context, _ *connect.Request[proto.UnassignNodeRequest]) (*connect.Response[proto.UnassignNodeResponse], error) {
	session := ctx.Value(CONTEXT_KEY_SESSION).(*session)
	defer session.delete()

	nodeID := session.getNodeID()
	if nodeID == nil {
		return &connect.Response[proto.UnassignNodeResponse]{}, nil
	}

	s.mutex.Lock()
	node := s.nodes[*nodeID]
	if node != nil {
		s.unassign(node)
	}
	s.mutex.Unlock()

	return &connect.Response[proto.UnassignNodeResponse]{}, nil
}

func (s *Seed) SendSignal(ctx context.Context, request *connect.Request[proto.SendSignalRequest]) (*connect.Response[proto.SendSignalResponse], error) {
	session := ctx.Value(CONTEXT_KEY_SESSION).(*session)
	nodeID := session.getNodeID()
	if nodeID == nil {
		s.log(ctx).Warn("session error (node id is not found)")
		return nil, ErrInternal
	}

	isAlone, err := s.isAlone(ctx, nodeID)
	if err != nil {
		return nil, ErrInternal
	}

	if !isAlone {
		signal := request.Msg.GetSignal()
		if signal == nil {
			s.log(ctx).Warn("request error (signal is required)")
			return nil, ErrInternal
		}

		if err := s.validateSignal(signal, nodeID); err != nil {
			return nil, err
		}

		relayToNext := false
		if signal.GetOffer() != nil && signal.GetOffer().GetType() == proto.SignalOfferType_SIGNAL_OFFER_TYPE_NEXT {
			relayToNext = true
		}

		if s.multiSeedHandler != nil {
			if err := s.multiSeedHandler.RelaySignal(ctx, signal, relayToNext); err != nil {
				s.log(ctx).Warn("failed to relay packet via multi seed", slog.String("error", err.Error()))
				return nil, ErrInternal
			}
		} else {
			if err := s.HandleSignal(ctx, signal, relayToNext); err != nil {
				s.log(ctx).Warn("failed to relay signal", slog.String("error", err.Error()))
				return nil, ErrInternal
			}
		}
	}

	if err := session.write(); err != nil {
		s.log(ctx).Warn("failed to write session", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	return &connect.Response[proto.SendSignalResponse]{
		Msg: &proto.SendSignalResponse{
			IsAlone: isAlone,
		},
	}, nil
}

func (s *Seed) validateSignal(signal *proto.Signal, srcNodeID *shared.NodeID) error {
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

func (s *Seed) PollSignal(ctx context.Context, _ *connect.Request[proto.PollSignalRequest], stream *connect.ServerStream[proto.PollSignalResponse]) error {
	session := ctx.Value(CONTEXT_KEY_SESSION).(*session)
	nodeID := session.getNodeID()
	if nodeID == nil {
		s.log(ctx).Warn("session error (node id is not found)")
		return ErrInternal
	}

	node := s.getExplicitNode(nodeID)
	if node == nil {
		s.log(ctx).Warn("node instance may be expired")
		return ErrInternal
	}

	node.mutex.Lock()
	node.stream = stream
	defer func() {
		node.mutex.Lock()
		node.stream = nil
		node.mutex.Unlock()
	}()

	// send the first packet immediately to notify the connection
	err := node.stream.Send(&proto.PollSignalResponse{
		Signals: node.signals,
	})
	if len(node.signals) > 0 {
		node.signals = make([]*proto.Signal, 0)
	}
	node.mutex.Unlock()
	if err != nil {
		return fmt.Errorf("failed to send signal: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil

	case <-node.term:
		return nil
	}
}

func (s *Seed) log(ctx context.Context) *slog.Logger {
	requestID := ctx.Value(CONTEXT_KEY_REQUEST_ID)
	if requestID == nil {
		return s.logger
	}

	return s.logger.With(slog.String("id", requestID.(string)))
}

func (s *Seed) getExplicitNode(nodeID *shared.NodeID) *node {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.nodes[*nodeID]
}

func (s *Seed) getNextNode(nodeID *shared.NodeID) *node {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// TODO: improve performance, this logic is worst: O(n)
	minNodeID := shared.NodeIDNone
	nextNodeID := shared.NodeIDNone
	for n := range s.nodes {
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
		return s.nodes[nextNodeID]
	}
	if minNodeID != shared.NodeIDNone {
		return s.nodes[minNodeID]
	}
	return nil
}

func (s *Seed) isAlone(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
	if s.multiSeedHandler != nil {
		isAlone, err := s.multiSeedHandler.IsAlone(ctx, nodeID)
		if err != nil {
			s.log(ctx).Warn("failed on check alone", slog.String("err", err.Error()))
		}
		return isAlone, err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.nodes) == 0 || (len(s.nodes) == 1 && s.nodes[*nodeID] != nil), nil
}

func (s *Seed) cleanup() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for nodeID, node := range s.nodes {
		node.mutex.Lock()
		timestamp := node.timestamp
		node.mutex.Unlock()

		if time.Since(timestamp) > s.pollingInterval {
			s.logger.Info("node expired", slog.String("nodeID", nodeID.String()))
			s.unassign(node)
		}
	}

	return nil
}

func (s *Seed) unassign(node *node) {
	node.close()
	delete(s.nodes, node.nodeID)
	if s.assignmentHandler != nil {
		s.assignmentHandler.UnassignNode(&node.nodeID)
	}
}

func (n *node) close() {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	close(n.term)
	n.signals = nil
}
