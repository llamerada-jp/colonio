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
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/sessions"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/controller"
	"github.com/llamerada-jp/colonio/seed/gateway"
	"github.com/llamerada-jp/colonio/seed/server"
)

type options struct {
	logger         *slog.Logger
	sessionStore   sessions.Store
	gateway        gateway.Gateway
	normalLifespan time.Duration
	shortLifespan  time.Duration
}

type optionSetter func(*options)

func WithLogger(logger *slog.Logger) optionSetter {
	return func(o *options) {
		o.logger = logger
	}
}

func WithSessionStore(store sessions.Store) optionSetter {
	return func(o *options) {
		o.sessionStore = store
	}
}

func WithGateway(gateway gateway.Gateway) optionSetter {
	return func(o *options) {
		o.gateway = gateway
	}
}

func WithLifespan(normal, short time.Duration) optionSetter {
	return func(o *options) {
		o.normalLifespan = normal
		o.shortLifespan = short
	}
}

type Seed struct {
	options
	server     *server.Server
	controller *controller.Controller
}

func NewSeed(optionSetters ...optionSetter) *Seed {
	options := &options{
		logger:         slog.Default(),
		normalLifespan: 30 * time.Minute,
		shortLifespan:  10 * time.Second,
	}

	for _, setter := range optionSetters {
		setter(options)
	}

	if options.sessionStore == nil {
		cookieKeyPair := os.Getenv("COLONIO_COOKIE_SECRET_KEY_PAIR")
		if cookieKeyPair == "" {
			panic("please set COLONIO_COOKIE_SECRET_KEY_PAIR")
		}
		cookieStore := sessions.NewCookieStore([]byte(cookieKeyPair))
		options.sessionStore = cookieStore
	}

	s := &Seed{}

	if options.gateway != nil {
		s.gateway = options.gateway
	} else {
		s.gateway = gateway.NewSimpleGateway(s, nil)
	}

	s.controller = controller.NewController(&controller.Options{
		Logger:         options.logger,
		Gateway:        s.gateway,
		NormalLifespan: options.normalLifespan,
		ShortLifespan:  options.shortLifespan,
		Interval:       options.shortLifespan / 2,
	})

	s.server = server.NewServer(server.Options{
		Logger:       options.logger,
		SessionStore: options.sessionStore,
		Controller:   s.controller,
	})

	return s
}

func (s *Seed) HandleUnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	return s.controller.HandleUnassignNode(ctx, nodeID)
}

func (s *Seed) HandleKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error {
	return s.controller.HandleKeepaliveRequest(ctx, nodeID)
}

func (s *Seed) HandleSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	return s.controller.HandleSignal(ctx, signal, relayToNext)
}

func (s *Seed) RegisterService(mux *http.ServeMux) {
	s.server.RegisterService(mux)
}

func (s *Seed) Run(ctx context.Context) error {
	return s.controller.Run(ctx)
}
