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
	"log/slog"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/misc"
)

type signalEntry struct {
	timestamp   time.Time
	signal      *proto.Signal
	relayToNext bool
}

type nodeEntry struct {
	lifespan          time.Time
	subscribingSignal bool
	waitingSignals    []signalEntry
	kvsActive         bool
}

type SimpleGateway struct {
	logger          *slog.Logger
	mtx             sync.Mutex
	handler         Handler
	nodeIDGenerator func(exists map[shared.NodeID]any) (*shared.NodeID, error)
	nodes           map[shared.NodeID]*nodeEntry
}

func NewSimpleGateway(logger *slog.Logger, handler Handler, nodeIDGenerator func(exists map[shared.NodeID]any) (*shared.NodeID, error)) Gateway {
	if nodeIDGenerator == nil {
		nodeIDGenerator = func(exists map[shared.NodeID]any) (*shared.NodeID, error) {
			for {
				nodeID := shared.NewRandomNodeID()
				if _, exists := exists[*nodeID]; !exists {
					return nodeID, nil
				}
			}
		}
	}

	return &SimpleGateway{
		logger:          logger,
		handler:         handler,
		nodeIDGenerator: nodeIDGenerator,
		nodes:           make(map[shared.NodeID]*nodeEntry),
	}
}

func (h *SimpleGateway) AssignNode(_ context.Context, lifespan time.Time) (*shared.NodeID, error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	exists := make(map[shared.NodeID]any, len(h.nodes))
	for nodeID := range h.nodes {
		exists[nodeID] = struct{}{}
	}
	nodeID, err := h.nodeIDGenerator(exists)
	if err != nil {
		return nil, fmt.Errorf("failed to generate node ID: %w", err)
	}

	if _, exists := h.nodes[*nodeID]; exists {
		panic(fmt.Sprintf("node ID %s already exists", nodeID.String()))
	}

	h.nodes[*nodeID] = &nodeEntry{
		lifespan:          lifespan,
		subscribingSignal: false,
		waitingSignals:    make([]signalEntry, 0),
	}
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

	h.nodes[*nodeID].lifespan = lifespan
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

	nodes := make(map[shared.NodeID]time.Time, len(h.nodes))
	for nodeID, entry := range h.nodes {
		nodes[nodeID] = entry.lifespan
	}
	return nodes, nil
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

	// If the node does node exist, no meaningful keepalive can be sent
	if !exists {
		return nil
	}

	// Here you would typically publish a keepalive request
	// For simplicity, we just call the handler directly
	return h.handler.HandleKeepaliveRequest(ctx, nodeID)
}

func (h *SimpleGateway) SubscribeSignal(ctx context.Context, nodeID *shared.NodeID) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	nodeEntry, exists := h.nodes[*nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}
	nodeEntry.subscribingSignal = true
	if len(nodeEntry.waitingSignals) > 0 {
		// If there are waiting signals, process them
		go func() {
			logger := misc.NewLogger(ctx, h.logger)
			h.mtx.Lock()
			defer h.mtx.Unlock()
			for _, entry := range nodeEntry.waitingSignals {
				if err := h.handler.HandleSignal(context.Background(), entry.signal, entry.relayToNext); err != nil {
					logger.Warn("failed to handle waiting signal", "nodeID", nodeID.String(), "error", err)
				}
			}
			nodeEntry.waitingSignals = make([]signalEntry, 0)
		}()
	}

	// Here you would typically set up a subscription mechanism
	// For simplicity, we just return nil
	return nil
}

func (h *SimpleGateway) UnsubscribeSignal(_ context.Context, nodeID *shared.NodeID) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	nodeEntry, exists := h.nodes[*nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}
	nodeEntry.subscribingSignal = false

	// Here you would typically remove the subscription
	// For simplicity, we just return nil
	return nil
}

func (h *SimpleGateway) PublishSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	nodeIDProto := signal.GetDstNodeId()
	nodeID, err := shared.NewNodeIDFromProto(nodeIDProto)
	if err != nil {
		// the signal is already checked the node ID before here
		panic(fmt.Sprintf("invalid source node ID in signal: %s", err.Error()))
	}

	h.mtx.Lock()
	var nodeEntry *nodeEntry
	if relayToNext {
		_, nodeEntry = misc.GetNextByMap(nodeID, h.nodes, nil)
	} else {
		nodeEntry = h.nodes[*nodeID]
	}

	// If the node does node exist, no meaningful signal can be sent
	if nodeEntry == nil {
		h.mtx.Unlock()
		return nil
	}

	if nodeEntry.subscribingSignal {
		h.mtx.Unlock()
		return h.handler.HandleSignal(ctx, signal, relayToNext)
	}

	nodeEntry.waitingSignals = append(nodeEntry.waitingSignals, signalEntry{
		timestamp:   time.Now(),
		signal:      signal,
		relayToNext: relayToNext,
	})
	h.mtx.Unlock()
	return nil
}

func (h *SimpleGateway) SetKVSState(ctx context.Context, nodeID *shared.NodeID, active bool) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	node, exists := h.nodes[*nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID.String())
	}

	node.kvsActive = active
	return nil
}

func (h *SimpleGateway) ExistsKVSActiveNode(ctx context.Context) (bool, error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	for _, node := range h.nodes {
		if node.kvsActive {
			return true, nil
		}
	}

	return false, nil
}
