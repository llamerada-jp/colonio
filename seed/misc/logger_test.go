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
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/go-json-experiment/json/v1"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/stretchr/testify/require"
)

type logContent struct {
	Level   string `json:"level,omitempty"`
	Message string `json:"msg,omitempty"`
	ReqID   string `json:"reqID,omitempty"`
	NodeID  string `json:"reqNodeID,omitempty"`
}

func TestLogger_withNodeID(t *testing.T) {
	nodeID := shared.NewRandomNodeID()
	ctx := NewLoggerContext(context.Background(), nodeID)

	var byteWriter bytes.Buffer
	logger := NewLogger(ctx, slog.New(slog.NewJSONHandler(&byteWriter, nil)))
	logger.Info("test message")

	var content logContent
	err := json.Unmarshal(byteWriter.Bytes(), &content)
	require.NoError(t, err)
	require.Equal(t, "INFO", content.Level)
	require.Equal(t, "test message", content.Message)
	require.Equal(t, nodeID.String(), content.NodeID)
	require.NotEmpty(t, content.ReqID)
}

func TestLogger_withoutNodeID(t *testing.T) {
	ctx := NewLoggerContext(context.Background(), nil)

	var byteWriter bytes.Buffer
	logger := NewLogger(ctx, slog.New(slog.NewJSONHandler(&byteWriter, nil)))
	logger.Info("test message")

	var content logContent
	err := json.Unmarshal(byteWriter.Bytes(), &content)
	require.NoError(t, err)
	require.Equal(t, "INFO", content.Level)
	require.Equal(t, "test message", content.Message)
	require.Empty(t, content.NodeID)
	require.NotEmpty(t, content.ReqID)
}

func TestLogger_withWrongContext(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "Expected panic for missing request ID in context")
	}()

	logger := NewLogger(context.Background(), slog.Default())
	logger.Info("test message")
}
