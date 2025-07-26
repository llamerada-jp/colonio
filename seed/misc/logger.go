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
package misc

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"

	"github.com/llamerada-jp/colonio/internal/shared"
)

type contextKey int

const (
	contextKeyRequestID contextKey = iota
	contextKeyNodeID
)

func NewLoggerContext(ctx context.Context, nodeID *shared.NodeID) context.Context {
	// generate random request ID
	requestID := fmt.Sprintf("%016x", rand.Int63())
	ctx = context.WithValue(ctx, contextKeyRequestID, requestID)
	ctx = context.WithValue(ctx, contextKeyNodeID, nodeID)
	return ctx
}

func NewLogger(ctx context.Context, logger *slog.Logger) *slog.Logger {
	requestID, ok := ctx.Value(contextKeyRequestID).(string)
	if !ok || len(requestID) == 0 {
		panic("context should contain a request ID")
	}
	logger = logger.With(slog.String("reqID", requestID))

	nodeID, ok := ctx.Value(contextKeyNodeID).(*shared.NodeID)
	if ok && nodeID != nil {
		logger = logger.With(slog.String("reqNodeID", nodeID.String()))
	}

	return logger
}

func ErrorByContext(ctx context.Context) error {
	requestID, ok := ctx.Value(contextKeyRequestID).(string)
	if !ok || len(requestID) == 0 {
		panic("context should contain a request ID")
	}
	return fmt.Errorf("reqID: %s", requestID)
}
