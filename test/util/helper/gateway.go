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
package helper

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/seed/gateway"
	"github.com/llamerada-jp/colonio/types"
)

type Gateway struct {
	Logger       *slog.Logger
	Seed         gateway.Handler
	NodeIDs      []*types.NodeID
	superOnce    sync.Once
	superGateway gateway.Gateway

	AssignNodeF                   func(ctx context.Context, lifespan time.Time) (*types.NodeID, error)
	UnassignNodeF                 func(ctx context.Context, nodeID *types.NodeID) error
	GetNodeCountF                 func(ctx context.Context) (uint64, error)
	UpdateNodeLifespanF           func(ctx context.Context, nodeID *types.NodeID, lifespan time.Time) error
	GetNodesByRangeF              func(ctx context.Context, backward, frontward *types.NodeID) ([]*types.NodeID, error)
	GetNodesF                     func(ctx context.Context) (map[types.NodeID]time.Time, error)
	SubscribeKeepaliveF           func(ctx context.Context, nodeID *types.NodeID) error
	UnsubscribeKeepaliveF         func(ctx context.Context, nodeID *types.NodeID) error
	PublishKeepaliveRequestF      func(ctx context.Context, nodeID *types.NodeID) error
	SubscribeSignalF              func(ctx context.Context, nodeID *types.NodeID) error
	UnsubscribeSignalF            func(ctx context.Context, nodeID *types.NodeID) error
	PublishSignalF                func(ctx context.Context, signal *proto.Signal, relayToNext bool) error
	SetKvsSectorStateF            func(ctx context.Context, nodeID *types.NodeID, active bool) error
	ExistsKvsActiveNodeF          func(ctx context.Context) (bool, error)
	SetKvsFirstActiveCandidateF   func(ctx context.Context, nodeID *types.NodeID) error
	UnsetKvsFirstActiveCandidateF func(ctx context.Context) error
}

var _ gateway.Gateway = (*Gateway)(nil)

func (h *Gateway) GetSuper() gateway.Gateway {
	if h.Seed == nil {
		panic("Seed must not be nil")
	}

	// If NodeIDs is provided, use it to generate node IDs.
	var nodeIDGenerator func(exists map[types.NodeID]any) (*types.NodeID, error)
	if h.NodeIDs != nil {
		nodeIDGenerator = func(exists map[types.NodeID]any) (*types.NodeID, error) {
			if len(h.NodeIDs) == len(exists) {
				return nil, fmt.Errorf("no more node IDs available")
			}
			return h.NodeIDs[len(exists)], nil
		}
	}

	h.superOnce.Do(func() {
		h.superGateway = gateway.NewSimpleGateway(h.Logger, h.Seed, nodeIDGenerator)
	})
	return h.superGateway
}

func (h *Gateway) AssignNode(ctx context.Context, lifespan time.Time) (*types.NodeID, error) {
	if h.AssignNodeF != nil {
		return h.AssignNodeF(ctx, lifespan)
	}
	return h.GetSuper().AssignNode(ctx, lifespan)
}

func (h *Gateway) UnassignNode(ctx context.Context, nodeID *types.NodeID) error {
	if h.UnassignNodeF != nil {
		return h.UnassignNodeF(ctx, nodeID)
	}
	return h.GetSuper().UnassignNode(ctx, nodeID)
}

func (h *Gateway) GetNodeCount(ctx context.Context) (uint64, error) {
	if h.GetNodeCountF != nil {
		return h.GetNodeCountF(ctx)
	}
	return h.GetSuper().GetNodeCount(ctx)
}

func (h *Gateway) UpdateNodeLifespan(ctx context.Context, nodeID *types.NodeID, lifespan time.Time) error {
	if h.UpdateNodeLifespanF != nil {
		return h.UpdateNodeLifespanF(ctx, nodeID, lifespan)
	}
	return h.GetSuper().UpdateNodeLifespan(ctx, nodeID, lifespan)
}

func (h *Gateway) GetNodesByRange(ctx context.Context, backward, frontward *types.NodeID) ([]*types.NodeID, error) {
	if h.GetNodesByRangeF != nil {
		return h.GetNodesByRangeF(ctx, backward, frontward)
	}
	return h.GetSuper().GetNodesByRange(ctx, backward, frontward)
}

func (h *Gateway) GetNodes(ctx context.Context) (map[types.NodeID]time.Time, error) {
	if h.GetNodesF != nil {
		return h.GetNodesF(ctx)
	}
	return h.GetSuper().GetNodes(ctx)
}

func (h *Gateway) SubscribeKeepalive(ctx context.Context, nodeID *types.NodeID) error {
	if h.SubscribeKeepaliveF != nil {
		return h.SubscribeKeepaliveF(ctx, nodeID)
	}
	return h.GetSuper().SubscribeKeepalive(ctx, nodeID)
}

func (h *Gateway) UnsubscribeKeepalive(ctx context.Context, nodeID *types.NodeID) error {
	if h.UnsubscribeKeepaliveF != nil {
		return h.UnsubscribeKeepaliveF(ctx, nodeID)
	}
	return h.GetSuper().UnsubscribeKeepalive(ctx, nodeID)
}

func (h *Gateway) PublishKeepaliveRequest(ctx context.Context, nodeID *types.NodeID) error {
	if h.PublishKeepaliveRequestF != nil {
		return h.PublishKeepaliveRequestF(ctx, nodeID)
	}
	return h.GetSuper().PublishKeepaliveRequest(ctx, nodeID)
}

func (h *Gateway) SubscribeSignal(ctx context.Context, nodeID *types.NodeID) error {
	if h.SubscribeSignalF != nil {
		return h.SubscribeSignalF(ctx, nodeID)
	}
	return h.GetSuper().SubscribeSignal(ctx, nodeID)
}

func (h *Gateway) UnsubscribeSignal(ctx context.Context, nodeID *types.NodeID) error {
	if h.UnsubscribeSignalF != nil {
		return h.UnsubscribeSignalF(ctx, nodeID)
	}
	return h.GetSuper().UnsubscribeSignal(ctx, nodeID)
}

func (h *Gateway) PublishSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	if h.PublishSignalF != nil {
		return h.PublishSignalF(ctx, signal, relayToNext)
	}
	return h.GetSuper().PublishSignal(ctx, signal, relayToNext)
}

func (h *Gateway) SetKvsSectorState(ctx context.Context, nodeID *types.NodeID, active bool) error {
	if h.SetKvsSectorStateF != nil {
		return h.SetKvsSectorStateF(ctx, nodeID, active)
	}
	return h.GetSuper().SetKvsSectorState(ctx, nodeID, active)
}

func (h *Gateway) ExistsKvsActiveNode(ctx context.Context) (bool, error) {
	if h.ExistsKvsActiveNodeF != nil {
		return h.ExistsKvsActiveNodeF(ctx)
	}
	return h.GetSuper().ExistsKvsActiveNode(ctx)
}

func (h *Gateway) SetKvsFirstActiveCandidate(ctx context.Context, nodeID *types.NodeID) error {
	if h.SetKvsFirstActiveCandidateF != nil {
		return h.SetKvsFirstActiveCandidateF(ctx, nodeID)
	}
	return h.GetSuper().SetKvsFirstActiveCandidate(ctx, nodeID)
}

func (h *Gateway) UnsetKvsFirstActiveCandidate(ctx context.Context) error {
	if h.UnsetKvsFirstActiveCandidateF != nil {
		return h.UnsetKvsFirstActiveCandidateF(ctx)
	}
	return h.GetSuper().UnsetKvsFirstActiveCandidate(ctx)
}
