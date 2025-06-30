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

	"github.com/gorilla/sessions"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/controller"
	"github.com/llamerada-jp/colonio/seed/gateway"
	"github.com/llamerada-jp/colonio/seed/usecase"
)

type options struct {
	logger       *slog.Logger
	sessionStore sessions.Store
	gateway      gateway.Gateway
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

type Seed interface {
	RegisterService(mux *http.ServeMux)
	Run(ctx context.Context) error
}

type seed struct {
	options
	controller *controller.Controller
	usecase    *usecase.Usecase
}

func NewSeed(optionSetters ...optionSetter) Seed {
	options := &options{}

	for _, setter := range optionSetters {
		setter(options)
	}

	if options.logger == nil {
		options.logger = slog.Default()
	}

	if options.sessionStore == nil {
		cookieKeyPair := os.Getenv("COLONIO_COOKIE_SECRET_KEY_PAIR")
		if cookieKeyPair == "" {
			panic("please set COLONIO_COOKIE_SECRET_KEY_PAIR")
		}
		cookieStore := sessions.NewCookieStore([]byte(cookieKeyPair))
		options.sessionStore = cookieStore
	}

	s := &seed{}

	if options.gateway != nil {
		s.gateway = options.gateway
	} else {
		s.gateway = gateway.NewSimpleGateway(s)
	}

	s.usecase = usecase.NewUsecase(&usecase.Options{
		Logger:  options.logger,
		Gateway: s.gateway,
	})

	s.controller = controller.NewController(controller.Options{
		Logger:       options.logger,
		SessionStore: options.sessionStore,
		Usecase:      s.usecase,
	})

	return s
}

func (s *seed) HandleUnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	return s.usecase.HandleUnassignNode(ctx, nodeID)
}

func (s *seed) HandleKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error {
	return s.usecase.HandleKeepaliveRequest(ctx, nodeID)
}

func (s *seed) HandleSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	return s.usecase.HandleSignal(ctx, signal, relayToNext)
}

func (s *seed) RegisterService(mux *http.ServeMux) {
	s.controller.RegisterService(mux)
}

func (s *seed) Run(ctx context.Context) error {
	panic("todo: implement seed.Run")
}
