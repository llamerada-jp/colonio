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
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/gateway"
	"github.com/stretchr/testify/require"
)

type GatewayHelper struct {
	T                        *testing.T
	AssignNodeF              func(ctx context.Context) (*shared.NodeID, error)
	UnassignNodeF            func(ctx context.Context, nodeID *shared.NodeID) error
	GetNodeCountF            func(ctx context.Context) (uint64, error)
	UpdateNodeTimestampF     func(ctx context.Context, nodeID *shared.NodeID) error
	GetNodesByRangeF         func(ctx context.Context, backward, frontward *shared.NodeID) ([]*shared.NodeID, error)
	GetNodesF                func(ctx context.Context) (map[shared.NodeID]time.Time, error)
	SubscribeKeepaliveF      func(ctx context.Context, nodeID *shared.NodeID) error
	UnsubscribeKeepaliveF    func(ctx context.Context, nodeID *shared.NodeID) error
	PublishKeepaliveRequestF func(ctx context.Context, nodeID *shared.NodeID) error
	SubscribeSignalF         func(ctx context.Context, nodeID *shared.NodeID) error
	UnsubscribeSignalF       func(ctx context.Context, nodeID *shared.NodeID) error
	PublishSignalF           func(ctx context.Context, signal *proto.Signal, relayToNext bool) error
}

var _ gateway.Gateway = (*GatewayHelper)(nil)

func (h *GatewayHelper) AssignNode(ctx context.Context) (*shared.NodeID, error) {
	require.NotNil(h.T, h.AssignNodeF, "AssignNodeF must not be nil")
	return h.AssignNodeF(ctx)
}

func (h *GatewayHelper) UnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	require.NotNil(h.T, h.UnassignNodeF, "UnassignNodeF must not be nil")
	return h.UnassignNodeF(ctx, nodeID)
}

func (h *GatewayHelper) GetNodeCount(ctx context.Context) (uint64, error) {
	require.NotNil(h.T, h.GetNodeCountF, "GetNodeCountF must not be nil")
	return h.GetNodeCountF(ctx)
}

func (h *GatewayHelper) UpdateNodeTimestamp(ctx context.Context, nodeID *shared.NodeID) error {
	require.NotNil(h.T, h.UpdateNodeTimestampF, "UpdateNodeTimestampF must not be nil")
	return h.UpdateNodeTimestampF(ctx, nodeID)
}

func (h *GatewayHelper) GetNodesByRange(ctx context.Context, backward, frontward *shared.NodeID) ([]*shared.NodeID, error) {
	require.NotNil(h.T, h.GetNodesByRangeF, "GetNodesByRangeF must not be nil")
	return h.GetNodesByRangeF(ctx, backward, frontward)
}

func (h *GatewayHelper) GetNodes(ctx context.Context) (map[shared.NodeID]time.Time, error) {
	require.NotNil(h.T, h.GetNodesF, "GetNodesF must not be nil")
	return h.GetNodesF(ctx)
}

func (h *GatewayHelper) SubscribeKeepalive(ctx context.Context, nodeID *shared.NodeID) error {
	require.NotNil(h.T, h.SubscribeKeepaliveF, "SubscribeKeepaliveF must not be nil")
	return h.SubscribeKeepaliveF(ctx, nodeID)
}

func (h *GatewayHelper) UnsubscribeKeepalive(ctx context.Context, nodeID *shared.NodeID) error {
	require.NotNil(h.T, h.UnsubscribeKeepaliveF, "UnsubscribeKeepaliveF must not be nil")
	return h.UnsubscribeKeepaliveF(ctx, nodeID)
}

func (h *GatewayHelper) PublishKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error {
	require.NotNil(h.T, h.PublishKeepaliveRequestF, "PublishKeepaliveRequestF must not be nil")
	return h.PublishKeepaliveRequestF(ctx, nodeID)
}

func (h *GatewayHelper) SubscribeSignal(ctx context.Context, nodeID *shared.NodeID) error {
	require.NotNil(h.T, h.SubscribeSignalF, "SubscribeSignalF must not be nil")
	return h.SubscribeSignalF(ctx, nodeID)
}

func (h *GatewayHelper) UnsubscribeSignal(ctx context.Context, nodeID *shared.NodeID) error {
	require.NotNil(h.T, h.UnsubscribeSignalF, "UnsubscribeSignalF must not be nil")
	return h.UnsubscribeSignalF(ctx, nodeID)
}

func (h *GatewayHelper) PublishSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	require.NotNil(h.T, h.PublishSignalF, "PublishSignalF must not be nil")
	return h.PublishSignalF(ctx, signal, relayToNext)
}
