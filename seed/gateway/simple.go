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
	"fmt"
	"maps"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type SimpleGateway struct {
	mtx             sync.Mutex
	handler         Handler
	nodeIDGenerator func(exists map[shared.NodeID]time.Time) (*shared.NodeID, error)
	nodes           map[shared.NodeID]time.Time
}

func NewSimpleGateway(handler Handler, nodeIDGenerator func(exists map[shared.NodeID]time.Time) (*shared.NodeID, error)) Gateway {
	if nodeIDGenerator == nil {
		nodeIDGenerator = func(exists map[shared.NodeID]time.Time) (*shared.NodeID, error) {
			for {
				nodeID := shared.NewRandomNodeID()
				if _, exists := exists[*nodeID]; !exists {
					return nodeID, nil
				}
			}
		}
	}

	return &SimpleGateway{
		handler:         handler,
		nodeIDGenerator: nodeIDGenerator,
		nodes:           make(map[shared.NodeID]time.Time),
	}
}

func (h *SimpleGateway) AssignNode(_ context.Context, lifespan time.Time) (*shared.NodeID, error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	nodeID, err := h.nodeIDGenerator(h.nodes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate node ID: %w", err)
	}

	if _, exists := h.nodes[*nodeID]; exists {
		panic(fmt.Sprintf("node ID %s already exists", nodeID.String()))
	}

	h.nodes[*nodeID] = lifespan
	return nodeID, nil
}

func (h *SimpleGateway) UnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	h.mtx.Lock()
	delete(h.nodes, *nodeID)
	h.mtx.Unlock()

	return h.handler.HandleUnassignNode(ctx, nodeID)
}

func (h *SimpleGateway) GetNodeCount(_ context.Context) (uint64, error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	return uint64(len(h.nodes)), nil
}

func (h *SimpleGateway) UpdateNodeLifespan(_ context.Context, nodeID *shared.NodeID, lifespan time.Time) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	if _, exists := h.nodes[*nodeID]; !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}

	h.nodes[*nodeID] = lifespan
	return nil
}

func (h *SimpleGateway) GetNodesByRange(_ context.Context, backward, frontward *shared.NodeID) ([]*shared.NodeID, error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	var nodes []*shared.NodeID
	if backward == nil && frontward == nil {
		// if both are nil, return all nodes
		for nodeID := range h.nodes {
			nodes = append(nodes, &nodeID)
		}
		return nodes, nil
	}

	if backward == nil || frontward == nil {
		return nil, fmt.Errorf("both backward and frontward must be specified")
	}

	if backward.Compare(frontward) <= 0 {
		// backward <= nodeID <= frontward
		for nodeID := range h.nodes {
			if nodeID.Compare(backward) >= 0 && nodeID.Compare(frontward) <= 0 {
				nodes = append(nodes, &nodeID)
			}
		}

	} else {
		// if backward > frontward
		// nodeID <= frontward || nodeID >= backward
		for nodeID := range h.nodes {
			if nodeID.Compare(frontward) <= 0 || nodeID.Compare(backward) >= 0 {
				nodes = append(nodes, &nodeID)
			}
		}
	}

	return nodes, nil
}

func (h *SimpleGateway) GetNodes(ctx context.Context) (map[shared.NodeID]time.Time, error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	return maps.Clone(h.nodes), nil
}

func (h *SimpleGateway) SubscribeKeepalive(_ context.Context, nodeID *shared.NodeID) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	if _, exists := h.nodes[*nodeID]; !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}

	// Here you would typically set up a subscription mechanism
	// For simplicity, we just return nil
	return nil
}

func (h *SimpleGateway) UnsubscribeKeepalive(_ context.Context, nodeID *shared.NodeID) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	if _, exists := h.nodes[*nodeID]; !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}

	// Here you would typically remove the subscription
	// For simplicity, we just return nil
	return nil
}

func (h *SimpleGateway) PublishKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error {
	h.mtx.Lock()
	_, exists := h.nodes[*nodeID]
	h.mtx.Unlock()

	if !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}

	// Here you would typically publish a keepalive request
	// For simplicity, we just call the handler directly
	return h.handler.HandleKeepaliveRequest(ctx, nodeID)
}

func (h *SimpleGateway) SubscribeSignal(_ context.Context, nodeID *shared.NodeID) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	if _, exists := h.nodes[*nodeID]; !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}

	// Here you would typically set up a subscription mechanism
	// For simplicity, we just return nil
	return nil
}

func (h *SimpleGateway) UnsubscribeSignal(_ context.Context, nodeID *shared.NodeID) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	if _, exists := h.nodes[*nodeID]; !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}

	// Here you would typically remove the subscription
	// For simplicity, we just return nil
	return nil
}

func (h *SimpleGateway) PublishSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	nodeIDProto := signal.GetSrcNodeId()
	nodeID, err := shared.NewNodeIDFromProto(nodeIDProto)
	if err != nil {
		// the signal is already checked the node ID before here
		panic(fmt.Sprintf("invalid source node ID in signal: %s", err.Error()))
	}

	h.mtx.Lock()
	_, exists := h.nodes[*nodeID]
	h.mtx.Unlock()

	if !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}

	return h.handler.HandleSignal(ctx, signal, relayToNext)
}
