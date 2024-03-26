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
	"math/rand"
	"net/http"
)

type logHandler struct {
	logger  *slog.Logger
	key     any
	handler http.Handler
}

// NewLogHandler creates a new http.Handler that output access Info logs using log/slog.
// `key` is a key to pass request ID to request context.
func NewLogHandler(logger *slog.Logger, key any, handler http.Handler) http.Handler {
	return &logHandler{
		logger:  logger,
		key:     key,
		handler: handler,
	}
}

// ServeHTTP is the implementation of http.Handler to output logs when get requests and after write the responses.
func (h *logHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// generate random request ID
	requestID := fmt.Sprintf("(%016x)", rand.Int63())
	h.logger.Info("request",
		slog.String("id", requestID),
		slog.String("from", r.RemoteAddr),
		slog.String("method", r.Method),
		slog.String("uri", r.RequestURI),
		slog.Int("contentLength", int(r.ContentLength)))

	bypass := &bypass{ResponseWriter: w}

	defer func() {
		h.logger.Info("response",
			slog.String("id", requestID),
			slog.Int("statusCode", bypass.statusCode),
			slog.Int("bodySize", bypass.bodySize))
	}()

	// set request id for the context
	ctx := context.WithValue(r.Context(), h.key, requestID)
	r = r.WithContext(ctx)

	h.handler.ServeHTTP(bypass, r)
}

// bypass is a wrapper for http.ResponseWriter to bypass status code of HTTP and body size of the response.
type bypass struct {
	http.ResponseWriter
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
