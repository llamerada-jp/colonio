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
	"sync"
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/misc"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/test/util/helper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestController_AssignNode(t *testing.T) {
	aNodeID := shared.NewRandomNodeID()

	tests := []struct {
		name             string
		nodeID           *shared.NodeID
		nodesByRange     []*shared.NodeID
		errorOnAssign    error
		nodeCount        uint64
		errorOnNodeCount error
		expectHasError   bool
		expectIsAlone    bool
		expectCallCount  int
	}{
		{
			name:            "Single Node",
			nodeID:          shared.NewRandomNodeID(),
			nodeCount:       0,
			expectIsAlone:   true,
			expectCallCount: 2,
		},
		{
			name:            "Single Node 2",
			nodeID:          aNodeID,
			nodesByRange:    []*shared.NodeID{aNodeID},
			nodeCount:       1,
			expectIsAlone:   true,
			expectCallCount: 3,
		},
		{
			name:            "There is an another node already",
			nodeID:          shared.NewRandomNodeID(),
			nodesByRange:    []*shared.NodeID{shared.NewRandomNodeID()},
			nodeCount:       1,
			expectIsAlone:   false,
			expectCallCount: 3,
		},
		{
			name:            "Multiple Nodes",
			nodeID:          shared.NewRandomNodeID(),
			nodeCount:       3,
			expectIsAlone:   false,
			expectCallCount: 2,
		},
		{
			name:            "Error on AssignNode",
			nodeID:          nil,
			errorOnAssign:   assert.AnError,
			expectHasError:  true,
			expectCallCount: 2,
		},
		{
			name:             "Error on GetNodeCount",
			nodeID:           shared.NewRandomNodeID(),
			errorOnNodeCount: assert.AnError,
			expectHasError:   true,
			expectCallCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalLifespan := 10 * time.Second
			callCount := 0

			c := NewController(&Options{
				Logger: testUtil.Logger(t),
				Gateway: &helper.Gateway{
					AssignNodeF: func(_ context.Context, lifespan time.Time) (*shared.NodeID, error) {
						callCount++
						assert.True(t, testUtil.NearTime(lifespan, time.Now().Add(normalLifespan)))
						return tt.nodeID, tt.errorOnAssign
					},
					GetNodeCountF: func(_ context.Context) (uint64, error) {
						callCount++
						return tt.nodeCount, tt.errorOnNodeCount
					},
					GetNodesByRangeF: func(_ context.Context, backward, frontward *shared.NodeID) ([]*shared.NodeID, error) {
						callCount++
						assert.Nil(t, backward)
						assert.Nil(t, frontward)
						return tt.nodesByRange, nil
					},
				},
				NormalLifespan: normalLifespan,
			})

			nodeID, isAlone, err := c.AssignNode(misc.NewLoggerContext(t.Context(), nil))
			if tt.expectHasError {
				assert.Error(t, err)

			} else {
				assert.NoError(t, err)
				assert.Equal(t, *tt.nodeID, *nodeID)
				assert.Equal(t, tt.expectIsAlone, isAlone)
				assert.Equal(t, tt.expectCallCount, callCount)
			}
		})
	}
}

func TestController_UnassignNode(t *testing.T) {
	ctx := misc.NewLoggerContext(t.Context(), nil)
	tests := []struct {
		name            string
		nodeID          *shared.NodeID
		errorOnUnassign error
		expectHasError  bool
	}{
		{
			name:            "Unassign Node Successfully",
			nodeID:          shared.NewRandomNodeID(),
			errorOnUnassign: nil,
			expectHasError:  false,
		},
		{
			name:            "Error on Unassign Node",
			nodeID:          shared.NewRandomNodeID(),
			errorOnUnassign: assert.AnError,
			expectHasError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0

			c := NewController(&Options{
				Logger: testUtil.Logger(t),
				Gateway: &helper.Gateway{
					UnassignNodeF: func(_ context.Context, nodeID *shared.NodeID) error {
						callCount++
						assert.Equal(t, *tt.nodeID, *nodeID)
						return tt.errorOnUnassign
					},
				},
			})

			err := c.UnassignNode(ctx, tt.nodeID)
			if tt.expectHasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, 1, callCount)
			}
		})
	}
}

func TestController_Keepalive(t *testing.T) {
	ctx := misc.NewLoggerContext(t.Context(), nil)
	nodeIDs := testUtil.UniqueNodeIDs(2)
	normalLifespan := 3 * time.Second
	shortLifespan := 1 * time.Second
	mtx := sync.Mutex{}
	finished := false
	callCount := 0

	c := NewController(&Options{
		Logger: testUtil.Logger(t),
		Gateway: &helper.Gateway{
			GetNodeCountF: func(ctx context.Context) (uint64, error) {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++
				// there are multiple nodes
				return 3, nil
			},
			UpdateNodeLifespanF: func(_ context.Context, nodeID *shared.NodeID, lifespan time.Time) error {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++
				switch callCount {
				case 1:
					assert.Equal(t, *nodeIDs[0], *nodeID)
					assert.True(t, testUtil.NearTime(lifespan, time.Now().Add(normalLifespan)))
				case 3:
					assert.Equal(t, *nodeIDs[1], *nodeID)
					assert.True(t, testUtil.NearTime(lifespan, time.Now().Add(shortLifespan)))
				case 4:
					assert.Equal(t, *nodeIDs[0], *nodeID)
					assert.True(t, testUtil.NearTime(lifespan, time.Now().Add(shortLifespan)))
				case 7:
					assert.Equal(t, *nodeIDs[0], *nodeID)
				case 10:
					assert.Equal(t, *nodeIDs[0], *nodeID)
				default:
					require.FailNow(t, "Unexpected call count", "callCount", callCount)
				}
				return nil
			},
			SubscribeKeepaliveF: func(_ context.Context, nodeID *shared.NodeID) error {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++
				assert.Equal(t, *nodeIDs[0], *nodeID)
				return nil
			},
			UnsubscribeKeepaliveF: func(_ context.Context, nodeID *shared.NodeID) error {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++
				assert.Equal(t, *nodeIDs[0], *nodeID)
				return nil
			},
		},
		NormalLifespan: normalLifespan,
		ShortLifespan:  shortLifespan,
	})

	go func() {
		isAlone, err := c.Keepalive(ctx, nodeIDs[0])
		require.NoError(t, err)
		assert.False(t, isAlone)

		mtx.Lock()
		defer mtx.Unlock()
		finished = true
	}()

	// wait for start to Keepalive
	assert.Eventually(t, func() bool {
		c.mtx.Lock()
		defer c.mtx.Unlock()
		return c.keepaliveChannels[*nodeIDs[0]] != nil
	}, 3*time.Second, 100*time.Millisecond)

	// If you request keepalive for the same node at the same time, an error will be returned.
	_, err := c.Keepalive(ctx, nodeIDs[0])
	assert.Error(t, err)

	// ignore the keepalive request for the unexpected node
	err = c.HandleKeepaliveRequest(ctx, nodeIDs[1])
	require.NoError(t, err)
	mtx.Lock()
	assert.False(t, finished)
	mtx.Unlock()

	// send the keepalive request for the expected node then should finish
	err = c.HandleKeepaliveRequest(ctx, nodeIDs[0])
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return finished
	}, 3*time.Second, 100*time.Millisecond)
	assert.NotContains(t, c.keepaliveChannels, *nodeIDs[0])

	// wait for the keepalive to finish again
	finished = false
	go func() {
		isAlone, err := c.Keepalive(ctx, nodeIDs[0])
		// occur error because the keepalive channel is closed
		require.Error(t, err)
		assert.False(t, isAlone)

		mtx.Lock()
		defer mtx.Unlock()
		finished = true
	}()

	// wait for start to Keepalive
	assert.Eventually(t, func() bool {
		c.mtx.Lock()
		defer c.mtx.Unlock()
		return c.keepaliveChannels[*nodeIDs[0]] != nil
	}, 3*time.Second, 100*time.Millisecond)

	// un assign another node
	err = c.HandleUnassignNode(ctx, nodeIDs[1])
	require.NoError(t, err)
	mtx.Lock()
	assert.False(t, finished)
	mtx.Unlock()

	// un assign target node then should finish
	err = c.HandleUnassignNode(ctx, nodeIDs[0])
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return finished
	}, 3*time.Second, 100*time.Millisecond)
	assert.NotContains(t, c.keepaliveChannels, *nodeIDs[0])

	// keepalive has a timeout
	finished = false
	go func() {
		_, err := c.Keepalive(ctx, nodeIDs[0])
		assert.NoError(t, err)

		mtx.Lock()
		defer mtx.Unlock()
		finished = true
	}()
	assert.Eventually(t, func() bool {
		c.mtx.Lock()
		mtx.Lock()
		defer func() {
			c.mtx.Unlock()
			mtx.Unlock()
		}()
		return finished && len(c.keepaliveChannels) == 0
	}, 5*time.Second, 100*time.Millisecond)

	mtx.Lock()
	defer mtx.Unlock()
	assert.Equal(t, 13, callCount)
}

func TestController_ReconcileNextNodes(t *testing.T) {
	ctx := misc.NewLoggerContext(t.Context(), nil)
	nodeIDs := testUtil.UniqueNodeIDs(10)

	tests := []struct {
		name                        string
		nodeID                      *shared.NodeID
		nextNodeIDs                 []*shared.NodeID
		disconnectedIDs             []*shared.NodeID
		nodeCount                   uint64
		expectNodesByRangeBackward  *shared.NodeID
		expectNodesByRangeFrontward *shared.NodeID
		nodesByRange                []*shared.NodeID
		expectCallCount             int
		expectResult                bool
	}{
		{
			name:            "alone node",
			nodeID:          nodeIDs[0],
			nextNodeIDs:     []*shared.NodeID{},
			disconnectedIDs: []*shared.NodeID{},
			nodeCount:       1,
			nodesByRange:    []*shared.NodeID{nodeIDs[0]},
			expectCallCount: 2,
			expectResult:    true,
		},
		{
			name:            "multiple nodes, no next nodes",
			nodeID:          nodeIDs[0],
			nextNodeIDs:     []*shared.NodeID{},
			disconnectedIDs: []*shared.NodeID{},
			nodeCount:       3,
			expectCallCount: 1,
			expectResult:    false,
		},
		{
			name:                        "multiple nodes, next nodes are all connected",
			nodeID:                      nodeIDs[1],
			nextNodeIDs:                 []*shared.NodeID{nodeIDs[0], nodeIDs[2]},
			disconnectedIDs:             []*shared.NodeID{},
			expectNodesByRangeBackward:  nodeIDs[0],
			expectNodesByRangeFrontward: nodeIDs[2],
			nodesByRange:                []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[2]},
			expectCallCount:             1,
			expectResult:                true,
		},
		{
			name:                        "multiple nodes, next nodes are not all connected 1",
			nodeID:                      nodeIDs[1],
			nextNodeIDs:                 []*shared.NodeID{nodeIDs[0], nodeIDs[2], nodeIDs[3]},
			expectNodesByRangeBackward:  nodeIDs[0],
			expectNodesByRangeFrontward: nodeIDs[3],
			nodesByRange:                []*shared.NodeID{nodeIDs[0], nodeIDs[1]},
			expectCallCount:             1,
			expectResult:                false,
		},
		{
			name:                        "multiple nodes, next nodes are not all connected 2",
			nodeID:                      nodeIDs[1],
			nextNodeIDs:                 []*shared.NodeID{nodeIDs[0], nodeIDs[2]},
			expectNodesByRangeBackward:  nodeIDs[0],
			expectNodesByRangeFrontward: nodeIDs[2],
			nodesByRange:                []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[2], nodeIDs[3]},
			expectCallCount:             1,
			expectResult:                false,
		},
		{
			name:                        "multiple nodes, have disconnected nodes",
			nodeID:                      nodeIDs[1],
			nextNodeIDs:                 []*shared.NodeID{nodeIDs[0], nodeIDs[2], nodeIDs[3]},
			disconnectedIDs:             []*shared.NodeID{nodeIDs[4], nodeIDs[5]},
			expectNodesByRangeBackward:  nodeIDs[0],
			expectNodesByRangeFrontward: nodeIDs[3],
			nodesByRange:                []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[2], nodeIDs[3]},
			expectCallCount:             3,
			expectResult:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mtx := sync.Mutex{}
			callCount := 0
			disconnectedCount := 0

			c := NewController(&Options{
				Logger: testUtil.Logger(t),
				Gateway: &helper.Gateway{
					GetNodeCountF: func(ctx context.Context) (uint64, error) {
						mtx.Lock()
						defer mtx.Unlock()
						callCount++
						return tt.nodeCount, nil
					},
					GetNodesByRangeF: func(_ context.Context, backward, frontward *shared.NodeID) ([]*shared.NodeID, error) {
						mtx.Lock()
						defer mtx.Unlock()
						callCount++
						assert.Equal(t, tt.expectNodesByRangeBackward, backward)
						assert.Equal(t, tt.expectNodesByRangeFrontward, frontward)
						return tt.nodesByRange, nil
					},
					PublishKeepaliveRequestF: func(ctx context.Context, nodeID *shared.NodeID) error {
						mtx.Lock()
						defer mtx.Unlock()
						callCount++
						assert.Equal(t, *tt.disconnectedIDs[disconnectedCount], *nodeID)
						disconnectedCount++
						return nil
					},
				},
			})

			res, err := c.ReconcileNextNodes(ctx, tt.nodeID, tt.nextNodeIDs, tt.disconnectedIDs)
			require.NoError(t, err)
			assert.Equal(t, tt.expectResult, res)
			c.mtx.Lock()
			defer c.mtx.Unlock()
			assert.Equal(t, tt.expectCallCount, callCount)
		})
	}
}

func TestController_Signal(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	mtx := sync.Mutex{}
	received := make([]*proto.Signal, 0)
	callCount := 0

	c := NewController(&Options{
		Logger: testUtil.Logger(t),
		Gateway: &helper.Gateway{
			SubscribeSignalF: func(_ context.Context, nodeID *shared.NodeID) error {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++
				assert.Equal(t, *nodeIDs[0], *nodeID)
				return nil
			},
			UnsubscribeSignalF: func(_ context.Context, nodeID *shared.NodeID) error {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++
				assert.Equal(t, *nodeIDs[0], *nodeID)
				return nil
			},
		},
	})

	ctx1 := misc.NewLoggerContext(t.Context(), nil)
	ctx2, cancel := context.WithCancel(ctx1)
	go func() {
		err := c.PollSignal(ctx2, nodeIDs[0], func(signal *proto.Signal) error {
			mtx.Lock()
			defer mtx.Unlock()
			received = append(received, signal)
			return nil
		})
		require.NoError(t, err)
	}()

	// wait for start to PollSignal
	assert.Eventually(t, func() bool {
		c.mtx.Lock()
		defer c.mtx.Unlock()
		return c.signalChannels[*nodeIDs[0]] != nil
	}, 3*time.Second, 100*time.Millisecond)

	// If you poll signal for the same node at the same time, an error will be returned.
	err := c.PollSignal(ctx1, nodeIDs[0], func(signal *proto.Signal) error {
		assert.FailNow(t, "Should not be called", "signal", signal)
		return nil
	})
	assert.Error(t, err)

	// ignore the signal for the unexpected node
	err = c.HandleSignal(ctx1, &proto.Signal{
		SrcNodeId: nodeIDs[0].Proto(),
		DstNodeId: nodeIDs[1].Proto(),
		Content: &proto.Signal_Ice{
			Ice: &proto.SignalICE{},
		},
	}, false)
	assert.Error(t, err)

	mtx.Lock()
	assert.Empty(t, received)
	mtx.Unlock()

	// acceptable signal
	err = c.HandleSignal(ctx1, &proto.Signal{
		SrcNodeId: nodeIDs[1].Proto(),
		DstNodeId: nodeIDs[0].Proto(),
		Content: &proto.Signal_Ice{
			Ice: &proto.SignalICE{
				OfferId: 1,
			},
		},
	}, false)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(received) == 1 && received[0].GetIce().GetOfferId() == 1
	}, 3*time.Second, 100*time.Millisecond)

	// acceptable signal with relayToNext
	err = c.HandleSignal(ctx1, &proto.Signal{
		SrcNodeId: nodeIDs[0].Proto(),
		DstNodeId: nodeIDs[1].Proto(),
		Content: &proto.Signal_Offer{
			Offer: &proto.SignalOffer{
				OfferId: 2,
			},
		},
	}, true)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(received) == 2 && received[1].GetOffer().GetOfferId() == 2
	}, 3*time.Second, 100*time.Millisecond)

	// stop the PollSignal
	cancel()
	assert.Eventually(t, func() bool {
		c.mtx.Lock()
		defer c.mtx.Unlock()
		return callCount == 2 && len(c.signalChannels) == 0
	}, 3*time.Second, 100*time.Millisecond)

}

func TestController_SendSignal(t *testing.T) {
	ctx := misc.NewLoggerContext(t.Context(), nil)
	nodeIDs := testUtil.UniqueNodeIDs(2)
	mtx := sync.Mutex{}
	callCount := 0

	c := NewController(&Options{
		Logger: testUtil.Logger(t),
		Gateway: &helper.Gateway{
			GetNodeCountF: func(ctx context.Context) (uint64, error) {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++

				switch callCount {
				case 1:
					return 2, nil // there are multiple nodes
				case 3:
					return 2, nil //there are multiple nodes
				case 5:
					return 1, nil // there is a single node
				default:
					require.FailNow(t, "Unexpected call count", "callCount", callCount)
					return 0, nil // unreachable
				}
			},
			GetNodesByRangeF: func(_ context.Context, backward, frontward *shared.NodeID) ([]*shared.NodeID, error) {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++
				assert.Nil(t, backward)
				assert.Nil(t, frontward)
				assert.Equal(t, 6, callCount) // called 6 times in total
				return []*shared.NodeID{nodeIDs[1]}, nil
			},
			PublishSignalF: func(_ context.Context, signal *proto.Signal, relayToNext bool) error {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++

				switch callCount {
				case 2:
					assert.Equal(t, nodeIDs[1].Proto(), signal.GetSrcNodeId())
					assert.Equal(t, nodeIDs[0].Proto(), signal.GetDstNodeId())
					assert.False(t, relayToNext)
				case 4:
					assert.True(t, relayToNext)
				default:
					require.FailNow(t, "Unexpected call count", "callCount", callCount)
				}
				return nil
			},
		},
	})

	// send signal when multiple nodes
	err := c.SendSignal(ctx, nodeIDs[1], &proto.Signal{
		SrcNodeId: nodeIDs[1].Proto(),
		DstNodeId: nodeIDs[0].Proto(),
		Content: &proto.Signal_Ice{
			Ice: &proto.SignalICE{},
		},
	})
	require.NoError(t, err)

	// send offer signal
	err = c.SendSignal(ctx, nodeIDs[1], &proto.Signal{
		SrcNodeId: nodeIDs[1].Proto(),
		DstNodeId: nodeIDs[0].Proto(),
		Content: &proto.Signal_Offer{
			Offer: &proto.SignalOffer{
				OfferId: 1,
				Type:    proto.SignalOfferType_SIGNAL_OFFER_TYPE_NEXT,
			},
		},
	})
	require.NoError(t, err)

	// send signal when single node
	err = c.SendSignal(ctx, nodeIDs[1], &proto.Signal{
		SrcNodeId: nodeIDs[1].Proto(),
		DstNodeId: nodeIDs[0].Proto(),
		Content: &proto.Signal_Ice{
			Ice: &proto.SignalICE{},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 6, callCount)
}

func TestController_cleanup(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	mtx := sync.Mutex{}
	callCount := 0

	c := NewController(&Options{
		Logger: testUtil.Logger(t),
		Gateway: &helper.Gateway{
			GetNodesF: func(_ context.Context) (map[shared.NodeID]time.Time, error) {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++
				assert.Equal(t, callCount%2, 1)
				return map[shared.NodeID]time.Time{
					*nodeIDs[0]: time.Now().Add(10 * time.Second),
					*nodeIDs[1]: time.Now().Add(-10 * time.Second),
				}, nil
			},
			UnassignNodeF: func(_ context.Context, nodeID *shared.NodeID) error {
				mtx.Lock()
				defer mtx.Unlock()
				callCount++
				assert.Equal(t, callCount%2, 0)
				assert.Equal(t, *nodeIDs[1], *nodeID)
				return nil
			},
		},
		Interval: 10 * time.Millisecond,
	})

	go func() {
		c.Run(t.Context())
	}()

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return callCount > 2
	}, 3*time.Second, 100*time.Millisecond)
}
