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
)

const (
	/**
	 * CONTEXT_KEY_REQUEST_KEY is used to describe the request id for logs.
	 */
	CONTEXT_KEY_REQUEST_ID = "requestID"
)

func NewLoggerContext(ctx context.Context) context.Context {
	// generate random request ID
	requestID := fmt.Sprintf("(%016x)", rand.Int63())
	return context.WithValue(ctx, CONTEXT_KEY_REQUEST_ID, requestID)
}

func NewLogger(ctx context.Context, logger *slog.Logger) *slog.Logger {
	requestID := ctx.Value(CONTEXT_KEY_REQUEST_ID)
	if requestID == nil {
		return logger
	}

	return logger.With(slog.String("id", requestID.(string)))
}
