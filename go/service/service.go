/*
 * Copyright 2019-2021 Yuji Ito <llamerada.jp@gmail.com>
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
package service

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"path/filepath"

	"github.com/llamerada-jp/colonio/go/seed"
	"github.com/quic-go/quic-go/http3"
)

type Service struct {
	Mux      *http.ServeMux
	config   *Config
	seed     *seed.Seed
	certFile string
	keyFile  string
}

func NewService(baseDir string, config *Config, verifier seed.TokenVerifier) (*Service, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	// setup to publish static documents
	if config.DocumentRoot != nil {
		log.Printf("publish DocumentRoot: %s", *config.DocumentRoot)
		mux.Handle("/", http.FileServer(http.Dir(calcPath(baseDir, *config.DocumentRoot))))
		for pattern, path := range config.Overrides {
			mux.Handle(pattern, http.FileServer(http.Dir(calcPath(baseDir, path))))
		}
	}

	// setup colonio-seed
	seed, err := seed.NewSeed(&config.Config, verifier)
	if err != nil {
		return nil, err
	}
	mux.Handle(config.Path+"/", http.StripPrefix(config.Path, seed.Handler))

	return &Service{
		Mux:      mux,
		config:   config,
		seed:     seed,
		certFile: calcPath(baseDir, config.CertFile),
		keyFile:  calcPath(baseDir, config.KeyFile),
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	// start seed
	s.seed.Start(ctx)

	// Start HTTP/3 service.
	if s.config.UseTCP {
		log.Println("start seed with tcp mode")
		return http3.ListenAndServe(
			fmt.Sprintf(":%d", s.config.Port),
			s.certFile,
			s.keyFile,
			&handlerWrapper{
				handler: s.Mux,
				headers: s.config.Headers,
			})
	}

	log.Println("start seed")
	server := http3.Server{
		Handler: s.Mux,
		Addr:    fmt.Sprintf(":%d", s.config.Port),
	}
	return server.ListenAndServeTLS(
		s.certFile,
		s.keyFile)
}

// warp http response writer to get http status code
type logWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (lw *logWrapper) WriteHeader(statusCode int) {
	lw.statusCode = statusCode
	lw.ResponseWriter.WriteHeader(statusCode)
}

// warp http handler to add some headers and to output access logs
type handlerWrapper struct {
	handler http.Handler
	headers map[string]string
}

func (h *handlerWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := fmt.Sprintf("(%016x)", rand.Int63())
	log.Printf("%s proto:%s, remote addr:%s, method:%s uri:%s size:%d\n", requestID, r.Proto, r.RemoteAddr, r.Method, r.RequestURI, r.ContentLength)

	writerWithLog := &logWrapper{
		ResponseWriter: w,
	}
	defer func() {
		log.Printf("%s status code:%d", requestID, writerWithLog.statusCode)
	}()

	// Set HTTP headers to enable SharedArrayBuffer for WebAssembly Threads
	header := w.Header()
	header.Add("Cross-Origin-Opener-Policy", "same-origin")
	header.Add("Cross-Origin-Embedder-Policy", "require-corp")
	for k, v := range h.headers {
		header.Add(k, v)
	}

	// set request id for the context
	ctx := context.WithValue(r.Context(), seed.CONTEXT_REQUEST_KEY, requestID)
	r = r.WithContext(ctx)

	h.handler.ServeHTTP(writerWithLog, r)
}

func calcPath(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
