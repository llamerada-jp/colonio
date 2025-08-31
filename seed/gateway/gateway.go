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
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Handler interface {
	HandleUnassignNode(ctx context.Context, nodeID *shared.NodeID) error
	HandleKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error
	HandleSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error
}

type Gateway interface {
	AssignNode(ctx context.Context, lifespan time.Time) (*shared.NodeID, error)
	UnassignNode(ctx context.Context, nodeID *shared.NodeID) error
	GetNodeCount(ctx context.Context) (uint64, error)
	UpdateNodeLifespan(ctx context.Context, nodeID *shared.NodeID, lifespan time.Time) error
	GetNodesByRange(ctx context.Context, backward, frontward *shared.NodeID) ([]*shared.NodeID, error)
	GetNodes(ctx context.Context) (map[shared.NodeID]time.Time, error)
	SubscribeKeepalive(ctx context.Context, nodeID *shared.NodeID) error
	UnsubscribeKeepalive(ctx context.Context, nodeID *shared.NodeID) error
	PublishKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error
	SubscribeSignal(ctx context.Context, nodeID *shared.NodeID) error
	UnsubscribeSignal(ctx context.Context, nodeID *shared.NodeID) error
	PublishSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error
	SetKvsState(ctx context.Context, nodeID *shared.NodeID, active bool) error
	ExistsKvsActiveNode(ctx context.Context) (bool, error)
}
