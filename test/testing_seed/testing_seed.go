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
package testing_seed

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/seed"
	"github.com/llamerada-jp/colonio/seed/util"
)

type TestingSeed struct {
	cancel        context.CancelFunc
	authenticator seed.Authenticator
	service       *util.Service
}

type ServiceOption func(*TestingSeed)

// NewTestingSeed creates and run a new testing seed.
func NewTestingSeed(opts ...ServiceOption) *TestingSeed {
	ctx, cancel := context.WithCancel(context.Background())

	ts := &TestingSeed{
		cancel: cancel,
	}

	// create server
	cert := os.Getenv("COLONIO_TEST_CERT")
	key := os.Getenv("COLONIO_TEST_KEY")
	if cert == "" || key == "" {
		panic("Please set COLONIO_TEST_CERT and COLONIO_TEST_KEY")
	}

	// default config
	serviceConfig := &util.ServiceConfig{
		Headers:  map[string]string{},
		Port:     8080,
		CertFile: cert,
		KeyFile:  key,

		Cluster: &config.Cluster{
			SessionTimeout: 30 * time.Second,
			PollingTimeout: 10 * time.Second,
			Revision:       1,
			IceServers: []config.IceServer{
				{
					Urls: []string{"stun:stun.l.google.com:19302"},
				},
			},
			Routing: config.Routing{
				ForceUpdateCount: toPtr[uint32](10),
				UpdatePeriod:     toPtr[uint32](500), // 500ms
			},
		},
		SeedPath: "/test",
	}

	// create service
	service := util.NewService(serviceConfig, slog.Default())
	ts.service = service

	// apply options
	for _, opt := range opts {
		opt(ts)
	}

	// create and run seed
	seedRunner, seedHandler := seed.NewSeed(serviceConfig.Cluster, slog.Default(), ts.authenticator)
	service.SetHandler(seedHandler)
	go func() {
		seedRunner.Run(ctx)
	}()

	// run service
	go func() {
		if err := service.Run(); err != nil {
			slog.Error("failed to run service", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	service.WaitForRun()
	slog.Info("testing seed is running", slog.Int("port", int(serviceConfig.Port)))

	return ts
}

func WithHandleTestingFunc(t *testing.T, path string, handler func(*testing.T, http.ResponseWriter, *http.Request)) ServiceOption {
	return func(ts *TestingSeed) {
		ts.service.RootMux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			handler(t, w, r)
		})
	}
}

func WithRevision(rev float64) ServiceOption {
	return func(ts *TestingSeed) {
		ts.service.Config.Cluster.Revision = rev
	}
}

func WithAuthenticator(auth seed.Authenticator) ServiceOption {
	return func(ts *TestingSeed) {
		ts.authenticator = auth
	}
}

func WithSessionTimeout(d time.Duration) ServiceOption {
	return func(ts *TestingSeed) {
		ts.service.Config.Cluster.SessionTimeout = d
	}
}

func WithPollingTimeout(d time.Duration) ServiceOption {
	return func(ts *TestingSeed) {
		ts.service.Config.Cluster.PollingTimeout = d
	}
}

func (s *TestingSeed) Port() uint16 {
	return s.service.Config.Port
}

func (s *TestingSeed) Stop() {
	s.service.Stop()
	s.cancel()
}

func toPtr[T any](v T) *T {
	return &v
}
