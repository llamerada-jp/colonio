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
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/gateway"
	"github.com/llamerada-jp/colonio/seed/misc"
)

type Options struct {
	Logger         *slog.Logger
	Gateway        gateway.Gateway
	NormalLifespan time.Duration // lifetime for everyday keepalive
	ShortLifespan  time.Duration // lifetime for reconcile to explicitly confirming
	Interval       time.Duration // interval for checking node status
}

type Controller struct {
	logger            *slog.Logger
	gateway           gateway.Gateway
	normalLifespan    time.Duration
	shortLifespan     time.Duration
	interval          time.Duration
	mtx               sync.Mutex
	signalChannels    map[shared.NodeID]*misc.Channel[*proto.Signal]
	keepaliveChannels map[shared.NodeID]*misc.Channel[bool]
}

var _ gateway.Handler = &Controller{}

func NewController(options *Options) *Controller {
	return &Controller{
		logger:            options.Logger,
		gateway:           options.Gateway,
		normalLifespan:    options.NormalLifespan,
		shortLifespan:     options.ShortLifespan,
		interval:          options.Interval,
		signalChannels:    make(map[shared.NodeID]*misc.Channel[*proto.Signal]),
		keepaliveChannels: make(map[shared.NodeID]*misc.Channel[bool]),
	}
}

func (c *Controller) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := c.cleanup(ctx); err != nil {
				c.logger.Error("failed to cleanup nodes", slog.Any("error", err))
			}
		}
	}
}

func (c *Controller) HandleUnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if ch, exists := c.signalChannels[*nodeID]; exists {
		ch.Close()
		delete(c.signalChannels, *nodeID)
	}
	if ch, exists := c.keepaliveChannels[*nodeID]; exists {
		ch.Close()
		delete(c.keepaliveChannels, *nodeID)
	}
	return nil
}

func (c *Controller) HandleKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error {
	if err := c.gateway.UpdateNodeLifespan(ctx, nodeID, time.Now().Add(c.shortLifespan)); err != nil {
		return fmt.Errorf("failed to update node lifespan: %w", err)
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	if ch, exists := c.keepaliveChannels[*nodeID]; exists {
		ch.SendWhenNotFull(true) // send a keepalive signal if channel is not full
	}

	return nil
}

func (c *Controller) HandleSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	dstNodeID, err := shared.NewNodeIDFromProto(signal.GetDstNodeId())
	if err != nil {
		// dst node id in the signal is already checked in the validateSignal method,
		panic(fmt.Sprintf("invalid dst node id in the packet: %v", err))
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	var ch *misc.Channel[*proto.Signal]
	if relayToNext {
		_, ch = misc.GetNextByMap(dstNodeID, c.signalChannels, nil)
	} else {
		ch = c.signalChannels[*dstNodeID]
	}
	if ch == nil {
		return fmt.Errorf("node not found: %s", dstNodeID)
	}

	ch.Send(signal)

	return nil
}

func (c *Controller) AssignNode(ctx context.Context) (*shared.NodeID, bool, error) {
	logger := misc.NewLogger(ctx, c.logger)
	nodeID, err := c.gateway.AssignNode(ctx, time.Now().Add(c.normalLifespan))
	if err != nil {
		return nil, false, fmt.Errorf("failed to assign node: %w", err)
	}

	isAlone, err := c.isAlone(ctx, nodeID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check if node is alone: %w", err)
	}

	logger.Info("node assigned", slog.String("nodeID", nodeID.String()))

	return nodeID, isAlone, nil
}

func (c *Controller) UnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	logger := misc.NewLogger(ctx, c.logger)

	if err := c.gateway.UnassignNode(ctx, nodeID); err != nil {
		return fmt.Errorf("failed to unassign node %s: %w", nodeID.String(), err)
	}

	logger.Info("node unassigned", slog.String("nodeID", nodeID.String()))
	return nil
}

func (c *Controller) Keepalive(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
	c.mtx.Lock()
	if _, exists := c.keepaliveChannels[*nodeID]; exists {
		c.mtx.Unlock()
		return false, fmt.Errorf("node %s already subscribed to keepalive", nodeID.String())
	}
	ch := misc.NewChannel[bool](1) // buffered channel to avoid blocking
	c.keepaliveChannels[*nodeID] = ch
	c.mtx.Unlock()

	if err := c.gateway.UpdateNodeLifespan(ctx, nodeID, time.Now().Add(c.normalLifespan)); err != nil {
		return false, fmt.Errorf("failed to update node lifespan: %w", err)
	}

	if err := c.gateway.SubscribeKeepalive(ctx, nodeID); err != nil {
		return false, fmt.Errorf("failed to subscribe keepalive request: %w", err)
	}

	timer := time.NewTimer(c.normalLifespan / 2)

	defer func() {
		ch.Close()
		timer.Stop()
		c.mtx.Lock()
		delete(c.keepaliveChannels, *nodeID)
		c.mtx.Unlock()

		if err := c.gateway.UnsubscribeKeepalive(ctx, nodeID); err != nil {
			logger := misc.NewLogger(ctx, c.logger)
			logger.Error("failed to unsubscribe from keepalive", slog.String("nodeID", nodeID.String()), slog.Any("error", err))
		}
	}()

	cc := ch.C()
	if cc == nil {
		return false, nil // channel closed
	}
	select {
	case <-ctx.Done():
		return false, nil

	case ok := <-cc:
		if ok {
			isAlone, err := c.isAlone(ctx, nodeID)
			if err != nil {
				return false, fmt.Errorf("failed to check if node is alone: %w", err)
			}
			return isAlone, nil
		} else {
			return false, fmt.Errorf("keepalive channel closed unexpectedly for node %s", nodeID.String())
		}

	case <-timer.C:
		isAlone, err := c.isAlone(ctx, nodeID)
		if err != nil {
			return false, fmt.Errorf("failed to check if node is alone: %w", err)
		}
		return isAlone, nil
	}
}

func (c *Controller) ReconcileNextNodes(ctx context.Context, nodeID *shared.NodeID, nextNodeIDs, disconnectedIDs []*shared.NodeID) (bool, error) {
	for _, disconnectedID := range disconnectedIDs {
		if err := c.gateway.PublishKeepaliveRequest(ctx, disconnectedID); err != nil {
			return false, fmt.Errorf("failed to publish keepalive request for disconnected node %s: %w", disconnectedID.String(), err)
		}
	}

	if len(nextNodeIDs) == 0 {
		isAlone, err := c.isAlone(ctx, nodeID)
		if err != nil {
			return false, fmt.Errorf("failed to check if node is alone: %w", err)
		}
		return isAlone, nil
	}

	nodeIDs, err := c.gateway.GetNodesByRange(ctx, nextNodeIDs[0], nextNodeIDs[len(nextNodeIDs)-1])
	if err != nil {
		return false, fmt.Errorf("failed to get nodes by range: %w", err)
	}

	nodeIDMap := make(map[shared.NodeID]struct{})
	for _, n := range nodeIDs {
		if n.Compare(nodeID) == 0 {
			continue // skip the current node
		}
		nodeIDMap[*n] = struct{}{}
	}

	if len(nodeIDMap) != len(nextNodeIDs) {
		return false, nil
	}

	for _, nextNodeID := range nextNodeIDs {
		if _, exists := nodeIDMap[*nextNodeID]; !exists {
			return false, nil // nextNodeID not found in the map, meaning it is not connected
		}
		// Remove matched node from the map to avoid double counting
		delete(nodeIDMap, *nextNodeID)
	}

	return len(nodeIDMap) == 0, nil
}

func (c *Controller) SendSignal(ctx context.Context, nodeID *shared.NodeID, signal *proto.Signal) error {
	isAlone, err := c.isAlone(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to check if node is alone: %w", err)
	}

	if !isAlone {
		if signal == nil {
			return fmt.Errorf("request error (signal is required)")
		}

		if err := c.validateSignal(signal, nodeID); err != nil {
			return err
		}

		relayToNext := false
		if signal.GetOffer() != nil && signal.GetOffer().GetType() == proto.SignalOfferType_SIGNAL_OFFER_TYPE_NEXT {
			relayToNext = true
		}

		if err := c.gateway.PublishSignal(ctx, signal, relayToNext); err != nil {
			return fmt.Errorf("failed to publish signal: %w", err)
		}
	}

	return nil
}

func (c *Controller) PollSignal(ctx context.Context, nodeID *shared.NodeID, send func(*proto.Signal) error) error {
	logger := misc.NewLogger(ctx, c.logger)

	if err := c.gateway.SubscribeSignal(ctx, nodeID); err != nil {
		return fmt.Errorf("failed to subscribe to signal: %w", err)
	}
	c.mtx.Lock()
	if _, exists := c.signalChannels[*nodeID]; exists {
		c.mtx.Unlock()
		return fmt.Errorf("node %s already subscribed to signal", nodeID.String())
	}
	ch := misc.NewChannel[*proto.Signal](100)
	c.signalChannels[*nodeID] = ch
	c.mtx.Unlock()
	defer func() {
		if err := c.gateway.UnsubscribeSignal(ctx, nodeID); err != nil {
			logger.Error("failed to unsubscribe from signal", slog.String("nodeID", nodeID.String()), slog.Any("error", err))
		}
		c.mtx.Lock()
		defer c.mtx.Unlock()
		ch.Close()
		delete(c.signalChannels, *nodeID)
	}()

	for {
		cc := ch.C()
		if cc == nil {
			// channel closed
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case signal, ok := <-cc:
			if !ok || signal == nil {
				return nil // channel closed, exit the loop
			}
			if err := send(signal); err != nil {
				logger.Warn("failed to send signal", slog.String("nodeID", nodeID.String()), slog.Any("error", err))
			}
		}
	}
}

func (c *Controller) cleanup(ctx context.Context) error {
	nodes, err := c.gateway.GetNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	for nodeID, lifetime := range nodes {
		if time.Now().After(lifetime) {
			c.gateway.UnassignNode(ctx, &nodeID)
		}
	}

	return nil
}

func (c *Controller) isAlone(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
	nodeCount, err := c.gateway.GetNodeCount(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get node count: %w", err)
	}
	if nodeCount == 0 {
		return true, nil
	}
	if nodeCount != 1 {
		return false, nil
	}

	nodes, err := c.gateway.GetNodesByRange(ctx, nil, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get nodes by range: %w", err)
	}
	if len(nodes) == 0 {
		return true, nil // no nodes found, consider it as alone
	}
	if len(nodes) == 1 && nodes[0].Equal(nodeID) {
		return true, nil // only this node is found, consider it as alone
	}

	return false, nil
}

func (c *Controller) validateSignal(signal *proto.Signal, srcNodeID *shared.NodeID) error {
	signalSrcNodeID, err := shared.NewNodeIDFromProto(signal.GetSrcNodeId())
	if err != nil {
		return fmt.Errorf("failed to parse src node id in signal packet: %w", err)
	}
	if !srcNodeID.Equal(signalSrcNodeID) {
		return fmt.Errorf("src node id is invalid expected:%s, got: %s", srcNodeID.String(), signalSrcNodeID.String())
	}
	_, err = shared.NewNodeIDFromProto(signal.GetDstNodeId())
	if err != nil {
		return fmt.Errorf("failed to parse dst node id in signal packet: %w", err)
	}
	if signal.GetOffer() == nil && signal.GetAnswer() == nil && signal.GetIce() == nil {
		return fmt.Errorf("signal content is required")
	}
	return nil
}
