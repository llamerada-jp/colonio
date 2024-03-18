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
package util

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/llamerada-jp/colonio/seed"
)

type Service struct {
	Handler http.Handler
	RootMux *http.ServeMux
	Config  *ServiceConfig

	logger *slog.Logger
	server *http.Server
}

// NewServiceHandler creates a new service instance.
func NewService(config *ServiceConfig, logger *slog.Logger) *Service {
	return &Service{
		RootMux: http.NewServeMux(),
		Config:  config,
		logger:  logger,
	}
}

func (s *Service) SetHandler(seedHandler http.Handler) {
	// setup to publish static documents
	if s.Config.DocumentRoot != nil {
		s.logger.Info("publish DocumentRoot", slog.String("path", *s.Config.DocumentRoot))
		s.RootMux.Handle("/", http.FileServer(http.Dir(s.Config.ToAbsPath(*s.Config.DocumentRoot))))
		for pattern, path := range s.Config.DocOverrides {
			s.RootMux.Handle(pattern, http.FileServer(http.Dir(s.Config.ToAbsPath(path))))
		}
	}

	s.RootMux.Handle(s.Config.SeedPath+"/", http.StripPrefix(s.Config.SeedPath, seedHandler))

	headers := maps.Clone(s.Config.Headers)
	if headers == nil {
		headers = make(map[string]string)
	}
	headers["Cross-Origin-Opener-Policy"] = "same-origin"
	headers["Cross-Origin-Embedder-Policy"] = "require-corp"

	s.Handler = NewLogHandler(s.logger, seed.CONTEXT_REQUEST_KEY, NewHeaderHandler(headers, s.RootMux))
}

func (s *Service) WaitForRun() {
	for {
		conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", s.Config.Port))
		if err == nil {
			conn.Close()
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Service) Run() error {
	// Start HTTP/2 server
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.Config.Port),
		Handler: s.Handler,
	}

	err := s.server.ListenAndServeTLS(
		s.Config.ToAbsPath(s.Config.CertFile),
		s.Config.ToAbsPath(s.Config.KeyFile))
	if err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Service) Stop() {
	err := s.server.Shutdown(context.Background())
	if err != nil {
		s.logger.Error("failed to stop server", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
