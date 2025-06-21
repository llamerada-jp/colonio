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
package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/gateway"
	"github.com/llamerada-jp/colonio/seed/misc"
)

type Options struct {
	Logger  *slog.Logger
	Gateway gateway.Gateway
}

type Usecase struct {
	logger            *slog.Logger
	gateway           gateway.Gateway
	mtx               sync.Mutex
	signalChannels    map[shared.NodeID]chan *proto.Signal
	keepaliveChannels map[shared.NodeID]chan bool
}

var _ gateway.Handler = &Usecase{}

func NewUsecase(options *Options) *Usecase {
	return &Usecase{
		logger:         options.Logger,
		gateway:        options.Gateway,
		signalChannels: make(map[shared.NodeID]chan *proto.Signal),
	}
}

func (u *Usecase) HandleUnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	u.mtx.Lock()
	defer u.mtx.Unlock()
	if c, exists := u.signalChannels[*nodeID]; exists {
		close(c)
		delete(u.signalChannels, *nodeID)
	}
	if c, exists := u.keepaliveChannels[*nodeID]; exists {
		close(c)
		delete(u.keepaliveChannels, *nodeID)
	}
	return nil
}

func (u *Usecase) HandleKeepaliveRequest(ctx context.Context, nodeID *shared.NodeID) error {
	u.mtx.Lock()
	defer u.mtx.Lock()

	if c, exists := u.keepaliveChannels[*nodeID]; exists {
		if len(c) != 0 {
			c <- true // send a keepalive signal if channel is not full
		}
	}

	return nil
}

func (u *Usecase) HandleSignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	dstNodeID, err := shared.NewNodeIDFromProto(signal.GetDstNodeId())
	if err != nil {
		// dst node id in the signal is already checked in the validateSignal method,
		panic(fmt.Sprintf("invalid dst node id in the packet: %v", err))
	}

	u.mtx.Lock()
	defer u.mtx.Lock()

	var c chan *proto.Signal
	if relayToNext {
		_, c = misc.GetNextByMap(dstNodeID, u.signalChannels, nil)
	} else {
		c = u.signalChannels[*dstNodeID]
	}
	if c == nil {
		return fmt.Errorf("node not found: %s", dstNodeID)
	}

	c <- signal

	return nil
}

func (u *Usecase) AssignNode(ctx context.Context) (*shared.NodeID, bool, error) {
	logger := misc.NewLogger(ctx, u.logger)
	nodeID, err := u.gateway.AssignNode(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to assign node: %w", err)
	}

	isAlone, err := u.isAlone(ctx, nodeID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check if node is alone: %w", err)
	}

	logger.Info("node assigned", slog.String("nodeID", nodeID.String()))

	return nodeID, isAlone, nil
}

func (u *Usecase) UnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	logger := misc.NewLogger(ctx, u.logger)

	if err := u.gateway.UnassignNode(ctx, nodeID); err != nil {
		return fmt.Errorf("failed to unassign node %s: %w", nodeID.String(), err)
	}

	logger.Info("node unassigned", slog.String("nodeID", nodeID.String()))
	return nil
}

func (u *Usecase) Keepalive(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
	if err := u.gateway.PublishKeepaliveRequest(ctx, nodeID); err != nil {
		return false, fmt.Errorf("failed to publish keepalive request: %w", err)
	}

	c := make(chan bool, 1) // buffered channel to avoid blocking
	u.mtx.Lock()
	if _, exists := u.keepaliveChannels[*nodeID]; exists {
		u.mtx.Unlock()
		return false, fmt.Errorf("node %s already subscribed to keepalive", nodeID.String())
	}
	u.keepaliveChannels[*nodeID] = c
	u.mtx.Unlock()

	select {
	case <-ctx.Done():
		return false, nil
	case <-c:
		isAlone, err := u.isAlone(ctx, nodeID)
		if err != nil {
			return false, fmt.Errorf("failed to check if node is alone: %w", err)
		}
		return isAlone, nil
	}
}

func (u *Usecase) ReconcileNextNodes(ctx context.Context, nodeID *shared.NodeID, nextNodeIDs, disconnectedIDs []*shared.NodeID) (bool, error) {
	if len(disconnectedIDs) != 0 {
		for _, disconnectedID := range disconnectedIDs {
			if err := u.gateway.PublishKeepaliveRequest(ctx, disconnectedID); err != nil {
				return false, fmt.Errorf("failed to publish keepalive request for disconnected node %s: %w", disconnectedID.String(), err)
			}
		}
	}

	if len(nextNodeIDs) == 0 {
		isAlone, err := u.isAlone(ctx, nodeID)
		if err != nil {
			return false, fmt.Errorf("failed to check if node is alone: %w", err)
		}
		return isAlone, nil
	}

	nodeIDs, err := u.gateway.GetNodesByRange(ctx, nextNodeIDs[0], nextNodeIDs[len(nextNodeIDs)-1])
	if err != nil {
		return false, fmt.Errorf("failed to get nodes by range: %w", err)
	}

	matched := len(nodeIDs) == len(nextNodeIDs)
	if matched {
		for i, nextNodeID := range nextNodeIDs {
			if !nextNodeID.Equal(nodeIDs[i]) {
				matched = false
				break
			}
		}
	}

	return matched, nil
}

func (u *Usecase) SendSignal(ctx context.Context, nodeID *shared.NodeID, signal *proto.Signal) error {
	isAlone, err := u.isAlone(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to check if node is alone: %w", err)
	}

	if !isAlone {
		if signal == nil {
			return fmt.Errorf("request error (signal is required)")
		}

		if err := u.validateSignal(signal, nodeID); err != nil {
			return err
		}

		relayToNext := false
		if signal.GetOffer() != nil && signal.GetOffer().GetType() == proto.SignalOfferType_SIGNAL_OFFER_TYPE_NEXT {
			relayToNext = true
		}

		if err := u.gateway.PublishSignal(ctx, signal, relayToNext); err != nil {
			return fmt.Errorf("failed to publish signal: %w", err)
		}
	}

	return nil
}

func (u *Usecase) PollSignal(ctx context.Context, nodeID *shared.NodeID, send func(*proto.Signal) error) error {
	logger := misc.NewLogger(ctx, u.logger)

	if err := u.gateway.SubscribeSignal(ctx, nodeID); err != nil {
		return fmt.Errorf("failed to subscribe to signal: %w", err)
	}
	c := make(chan *proto.Signal, 100)
	u.mtx.Lock()
	if _, exists := u.signalChannels[*nodeID]; exists {
		u.mtx.Unlock()
		return fmt.Errorf("node %s already subscribed to signal", nodeID.String())
	}
	u.signalChannels[*nodeID] = c
	u.mtx.Unlock()
	defer func() {
		if err := u.gateway.UnsubscribeSignal(ctx, nodeID); err != nil {
			logger.Error("failed to unsubscribe from signal", slog.String("nodeID", nodeID.String()), slog.Any("error", err))
		}
		u.mtx.Lock()
		defer u.mtx.Unlock()
		close(c)
		delete(u.signalChannels, *nodeID)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal, ok := <-c:
			if !ok || signal == nil {
				return nil // channel closed, exit the loop
			}
			if err := send(signal); err != nil {
				logger.Warn("failed to send signal", slog.String("nodeID", nodeID.String()), slog.Any("error", err))
			}
		}
	}
}

func (u *Usecase) isAlone(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
	nodeCount, err := u.gateway.GetNodeCount(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get node count: %w", err)
	}
	if nodeCount == 0 {
		return true, nil
	}
	if nodeCount != 1 {
		return false, nil
	}

	return false, nil
}

func (u *Usecase) validateSignal(signal *proto.Signal, srcNodeID *shared.NodeID) error {
	signalSrcNodeID, err := shared.NewNodeIDFromProto(signal.GetSrcNodeId())
	if err != nil {
		return fmt.Errorf("failed to parse src node id in signal packet: %w", err)
	}
	if !srcNodeID.Equal(signalSrcNodeID) {
		return fmt.Errorf("src node id is invalid")
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
