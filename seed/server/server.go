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
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/gorilla/sessions"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	service "github.com/llamerada-jp/colonio/api/colonio/v1alpha/v1alphaconnect"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/controller"
	"github.com/llamerada-jp/colonio/seed/misc"
)

type ContextKey int

const (
	ContextKeySession ContextKey = iota
)

var (
	ErrSession  = errors.New("session error")
	ErrInternal = errors.New("internal error")
)

type Options struct {
	Logger       *slog.Logger
	SessionStore sessions.Store
	Controller   *controller.Controller
}

type Server struct {
	logger         *slog.Logger
	sessionStore   sessions.Store
	controller     *controller.Controller
	connectHandler http.Handler
	service.UnimplementedSeedServiceHandler
}

/**
 * NewServer creates an http handler to provide colonio's seed.
 */
func NewServer(options Options) *Server {
	return &Server{
		logger:       options.Logger,
		sessionStore: options.SessionStore,
		controller:   options.Controller,
	}
}

func (c *Server) RegisterService(mux *http.ServeMux) {
	path, handler := service.NewSeedServiceHandler(c)
	c.connectHandler = handler
	mux.Handle(path, c)
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

func (c *Server) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	// create session
	session, err := newSession(c.sessionStore, request, response)
	if err != nil {
		c.logger.Error("failed to create session", slog.String("error", err.Error()))
		http.Error(response, "session error", http.StatusInternalServerError)
		return
	}
	ctxWithSession := context.WithValue(request.Context(), ContextKeySession, session)
	nodeID := session.getNodeID()
	ctx := misc.NewLoggerContext(ctxWithSession, nodeID)
	logger := misc.NewLogger(ctx, c.logger)

	logger.Info("request",
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
		logger.Info("response",
			slog.Int("statusCode", bypass.statusCode),
			slog.Int("bodySize", bypass.bodySize))
	}()

	c.connectHandler.ServeHTTP(bypass, request.WithContext(ctx))
}

func (c *Server) AssignNode(ctx context.Context, _ *connect.Request[proto.AssignNodeRequest]) (*connect.Response[proto.AssignNodeResponse], error) {
	logger := misc.NewLogger(ctx, c.logger)
	session := ctx.Value(ContextKeySession).(*session)

	// check if the session is already assigned
	if nodeID := session.getNodeID(); nodeID != nil {
		if err := c.controller.UnassignNode(ctx, nodeID); err != nil {
			logger.Warn("failed to unassign node", slog.String("error", err.Error()))
			return nil, ErrInternal
		}
	}

	// assign a new node
	nodeID, isAlone, err := c.controller.AssignNode(ctx)
	if err != nil {
		logger.Warn("failed to assign node", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	session.setNodeID(nodeID)
	if err = session.write(); err != nil {
		logger.Warn("failed to write session", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	return &connect.Response[proto.AssignNodeResponse]{
		Msg: &proto.AssignNodeResponse{
			NodeId:  nodeID.Proto(),
			IsAlone: isAlone,
		},
	}, nil
}

func (c *Server) UnassignNode(ctx context.Context, _ *connect.Request[proto.UnassignNodeRequest]) (*connect.Response[proto.UnassignNodeResponse], error) {
	logger := misc.NewLogger(ctx, c.logger)
	session := ctx.Value(ContextKeySession).(*session)
	defer func() {
		if session != nil {
			session.delete()
		}
	}()

	nodeID := session.getNodeID()
	if nodeID == nil {
		return &connect.Response[proto.UnassignNodeResponse]{}, nil
	}

	if err := c.controller.UnassignNode(ctx, nodeID); err != nil {
		logger.Warn("failed to unassign node", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	return &connect.Response[proto.UnassignNodeResponse]{}, nil
}

func (c *Server) Keepalive(ctx context.Context, _ *connect.Request[proto.KeepaliveRequest]) (*connect.Response[proto.KeepaliveResponse], error) {
	logger := misc.NewLogger(ctx, c.logger)
	session := ctx.Value(ContextKeySession).(*session)
	nodeID := session.getNodeID()
	if nodeID == nil {
		logger.Warn("session error (node id is not found)")
		return nil, ErrInternal
	}

	isAlone, err := c.controller.Keepalive(ctx, nodeID)
	if err != nil {
		logger.Warn("failed to keepalive", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	if err = session.write(); err != nil {
		logger.Warn("failed to write session", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	return &connect.Response[proto.KeepaliveResponse]{
		Msg: &proto.KeepaliveResponse{
			IsAlone: isAlone,
		},
	}, nil
}

func (c *Server) SendSignal(ctx context.Context, request *connect.Request[proto.SendSignalRequest]) (*connect.Response[proto.SendSignalResponse], error) {
	logger := misc.NewLogger(ctx, c.logger)
	session := ctx.Value(ContextKeySession).(*session)

	nodeID := session.getNodeID()
	if nodeID == nil {
		logger.Warn("session error (node id is not found)")
		return nil, ErrInternal
	}

	if err := c.controller.SendSignal(ctx, nodeID, request.Msg.GetSignal()); err != nil {
		logger.Warn("failed to send signal", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	if err := session.write(); err != nil {
		logger.Warn("failed to write session", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	return &connect.Response[proto.SendSignalResponse]{
		Msg: &proto.SendSignalResponse{},
	}, nil
}

func (c *Server) PollSignal(ctx context.Context, _ *connect.Request[proto.PollSignalRequest], stream *connect.ServerStream[proto.PollSignalResponse]) error {
	logger := misc.NewLogger(ctx, c.logger)
	session := ctx.Value(ContextKeySession).(*session)
	nodeID := session.getNodeID()
	if nodeID == nil {
		logger.Warn("session error (node id is not found)")
		return ErrInternal
	}

	// send the first packet immediately to notify the connection
	if err := stream.Send(&proto.PollSignalResponse{
		Signals: []*proto.Signal{},
	}); err != nil {
		logger.Warn("failed to send initial response", slog.String("error", err.Error()))
		return ErrInternal
	}

	if err := c.controller.PollSignal(ctx, nodeID, func(s *proto.Signal) error {
		return stream.Send(&proto.PollSignalResponse{
			Signals: []*proto.Signal{s},
		})
	}); err != nil {
		logger.Warn("failed to poll signal", slog.String("error", err.Error()))
		return ErrInternal
	}

	return nil
}

func (c *Server) ReconcileNextNodes(ctx context.Context, request *connect.Request[proto.ReconcileNextNodesRequest]) (*connect.Response[proto.ReconcileNextNodesResponse], error) {
	logger := misc.NewLogger(ctx, c.logger)
	session := ctx.Value(ContextKeySession).(*session)
	nodeID := session.getNodeID()
	if nodeID == nil {
		logger.Warn("session error (node id is not found)")
		return nil, ErrInternal
	}

	nextNodeIDs, err := shared.ConvertNodeIDsFromProto(request.Msg.GetNextNodeIds())
	if err != nil {
		logger.Warn("next_node_ids contains invalid", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	disconnectedIDs, err := shared.ConvertNodeIDsFromProto(request.Msg.GetDisconnectedNodeIds())
	if err != nil {
		logger.Warn("disconnected_node_ids contains invalid", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	matched, err := c.controller.ReconcileNextNodes(ctx, nodeID, nextNodeIDs, disconnectedIDs)
	if err != nil {
		logger.Warn("failed to reconcile next nodes", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	if err := session.write(); err != nil {
		logger.Warn("failed to write session", slog.String("error", err.Error()))
		return nil, ErrInternal
	}

	return &connect.Response[proto.ReconcileNextNodesResponse]{
		Msg: &proto.ReconcileNextNodesResponse{
			Matched: matched,
		},
	}, nil
}
