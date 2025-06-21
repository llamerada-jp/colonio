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
package gateway

import (
	"context"
	"testing"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/stretchr/testify/require"
)

type HandlerHelper struct {
	t                       *testing.T
	HandleUnassignNodeF     func(ctx context.Context, nodeID *shared.NodeID) error
	HandleKeepaliveRequestF func(ctx context.Context, nodeID *shared.NodeID) error
	HandleSignalF           func(ctx context.Context, signal *proto.Signal, relayToNext bool) error
}

var _ Handler = &HandlerHelper{}

func (h *HandlerHelper) HandleUnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	h.t.Helper()
	require.NotNil(h.t, h.HandleUnassignNodeF)
	return h.HandleUnassignNodeF(ctx, nodeID)
}

func (h *HandlerHelper) HandleKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error {
	h.t.Helper()
	require.NotNil(h.t, h.HandleKeepaliveRequestF)
	return h.HandleKeepaliveRequestF(ctx, nodeID)
}

func (h *HandlerHelper) HandleSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	h.t.Helper()
	require.NotNil(h.t, h.HandleSignalF)
	return h.HandleSignalF(ctx, signal, relayToNext)
}

func TestSimpleGateway_AssignNode(t *testing.T) {
	sg := NewSimpleGateway(nil)
}
