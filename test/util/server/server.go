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
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"time"

	"github.com/llamerada-jp/colonio/seed"
	"github.com/llamerada-jp/colonio/test/util"
)

type options struct {
	logger *slog.Logger
	port   uint16
	mux    *http.ServeMux
}

type OptionSetter func(*options)

func WithLogger(logger *slog.Logger) OptionSetter {
	return func(o *options) {
		o.logger = logger
	}
}

func WithPort(port uint16) OptionSetter {
	return func(o *options) {
		o.port = port
	}
}

func WithDocument(path string) OptionSetter {
	return func(o *options) {
		o.mux.Handle("/", http.FileServer(http.Dir(path)))
	}
}

func WithDocumentOverride(pattern, path string) OptionSetter {
	return func(o *options) {
		o.mux.Handle(pattern, http.FileServer(http.Dir(path)))
	}
}

func WithHttpHandlerFunc(pattern string, handlerFunc http.HandlerFunc) OptionSetter {
	return func(o *options) {
		o.mux.HandleFunc(pattern, handlerFunc)
	}
}

type Helper struct {
	options
	seed   *seed.Seed
	server *http.Server
	cancel context.CancelFunc
}

func NewHelper(seed *seed.Seed, optionsSetters ...OptionSetter) *Helper {
	opts := options{
		logger: slog.Default(),
		port:   8000 + uint16(rand.Uint32()%1000),
		mux:    http.NewServeMux(),
	}

	for _, setter := range optionsSetters {
		setter(&opts)
	}

	seed.RegisterService(opts.mux)

	return &Helper{
		options: opts,
		seed:    seed,
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", opts.port),
			Handler: opts.mux,
		},
	}
}

func (h *Helper) URL() string {
	return fmt.Sprintf("https://localhost:%d", h.port)
}

func (h *Helper) Start(ctx context.Context) {
	cert := os.Getenv("COLONIO_TEST_CERT")
	key := os.Getenv("COLONIO_TEST_KEY")
	if cert == "" || key == "" {
		panic("Please set COLONIO_TEST_CERT and COLONIO_TEST_KEY")
	}

	if h.cancel != nil {
		panic("already started")
	}

	cc, cancel := context.WithCancel(ctx)
	h.cancel = cancel

	go func() {
		h.seed.Run(cc)
	}()

	go func() {
		err := h.server.ListenAndServeTLS(cert, key)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			h.logger.Error("http server start error", slog.String("error", err.Error()))
		}
	}()

	go func() {
		<-cc.Done()
		h.server.Shutdown(context.Background())
		h.cancel()
	}()

	// wait for the server to start
	client := util.NewInsecureHttpClient()
	for i := 0; i < 100; i++ {
		_, err := client.Get(h.URL())
		if err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	panic("server start timeout")
}

func (h *Helper) Stop() {
	h.cancel()
}
