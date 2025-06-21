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
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/gateway"
)

type GatewayHelper struct {
	Seed         gateway.Handler
	superGateway gateway.Gateway

	AssignNodeF              func(ctx context.Context) (*shared.NodeID, error)
	UnassignNodeF            func(ctx context.Context, nodeID *shared.NodeID) error
	GetNodeCountF            func(ctx context.Context) (uint64, error)
	UpdateNodeLifespanF      func(ctx context.Context, nodeID *shared.NodeID) error
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

func (h *GatewayHelper) GetSuper() gateway.Gateway {
	if h.Seed == nil {
		panic("Seed must not be nil")
	}
	if h.superGateway == nil {
		h.superGateway = gateway.NewSimpleGateway(h.Seed)
	}
	return h.superGateway
}

func (h *GatewayHelper) AssignNode(ctx context.Context) (*shared.NodeID, error) {
	if h.AssignNodeF != nil {
		return h.AssignNodeF(ctx)
	}
	return h.GetSuper().AssignNode(ctx)
}

func (h *GatewayHelper) UnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	if h.UnassignNodeF != nil {
		return h.UnassignNodeF(ctx, nodeID)
	}
	return h.GetSuper().UnassignNode(ctx, nodeID)
}

func (h *GatewayHelper) GetNodeCount(ctx context.Context) (uint64, error) {
	if h.GetNodeCountF != nil {
		return h.GetNodeCountF(ctx)
	}
	return h.GetSuper().GetNodeCount(ctx)
}

func (h *GatewayHelper) UpdateNodeLifespan(ctx context.Context, nodeID *shared.NodeID, lifespan time.Time) error {
	if h.UpdateNodeLifespanF != nil {
		return h.UpdateNodeLifespanF(ctx, nodeID)
	}
	return h.GetSuper().UpdateNodeLifespan(ctx, nodeID, lifespan)
}

func (h *GatewayHelper) GetNodesByRange(ctx context.Context, backward, frontward *shared.NodeID) ([]*shared.NodeID, error) {
	if h.GetNodesByRangeF != nil {
		return h.GetNodesByRangeF(ctx, backward, frontward)
	}
	return h.GetSuper().GetNodesByRange(ctx, backward, frontward)
}

func (h *GatewayHelper) GetNodes(ctx context.Context) (map[shared.NodeID]time.Time, error) {
	if h.GetNodesF != nil {
		return h.GetNodesF(ctx)
	}
	return h.GetSuper().GetNodes(ctx)
}

func (h *GatewayHelper) SubscribeKeepalive(ctx context.Context, nodeID *shared.NodeID) error {
	if h.SubscribeKeepaliveF != nil {
		return h.SubscribeKeepaliveF(ctx, nodeID)
	}
	return h.GetSuper().SubscribeKeepalive(ctx, nodeID)
}

func (h *GatewayHelper) UnsubscribeKeepalive(ctx context.Context, nodeID *shared.NodeID) error {
	if h.UnsubscribeKeepaliveF != nil {
		return h.UnsubscribeKeepaliveF(ctx, nodeID)
	}
	return h.GetSuper().UnsubscribeKeepalive(ctx, nodeID)
}

func (h *GatewayHelper) PublishKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error {
	if h.PublishKeepaliveRequestF != nil {
		return h.PublishKeepaliveRequestF(ctx, nodeID)
	}
	return h.GetSuper().PublishKeepaliveRequest(ctx, nodeID)
}

func (h *GatewayHelper) SubscribeSignal(ctx context.Context, nodeID *shared.NodeID) error {
	if h.SubscribeSignalF != nil {
		return h.SubscribeSignalF(ctx, nodeID)
	}
	return h.GetSuper().SubscribeSignal(ctx, nodeID)
}

func (h *GatewayHelper) UnsubscribeSignal(ctx context.Context, nodeID *shared.NodeID) error {
	if h.UnsubscribeSignalF != nil {
		return h.UnsubscribeSignalF(ctx, nodeID)
	}
	return h.GetSuper().UnsubscribeSignal(ctx, nodeID)
}

func (h *GatewayHelper) PublishSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	if h.PublishSignalF != nil {
		return h.PublishSignalF(ctx, signal, relayToNext)
	}
	return h.GetSuper().PublishSignal(ctx, signal, relayToNext)
}
