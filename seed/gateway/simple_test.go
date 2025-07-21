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
	"sync"
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
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

func TestSimpleGateway_AssignNode_UnassignNode(t *testing.T) {
	var nodeID *shared.NodeID
	var err error

	called := false
	sg := NewSimpleGateway(testUtil.Logger(t),
		&HandlerHelper{
			t: t,
			HandleUnassignNodeF: func(ctx context.Context, n *shared.NodeID) error {
				t.Helper()
				assert.Equal(t, *nodeID, *n)
				called = true
				return nil
			},
		}, nil).(*SimpleGateway)

	lifespan := time.Now().Add(10 * time.Minute)
	nodeID, err = sg.AssignNode(t.Context(), lifespan)
	require.NoError(t, err)
	require.NotNil(t, nodeID)
	assert.Equal(t, lifespan, sg.nodes[*nodeID].lifespan)

	err = sg.UnassignNode(t.Context(), nodeID)
	require.NoError(t, err)
	assert.NotContains(t, sg.nodes, *nodeID)
	require.True(t, called)
}

func TestSimpleGateway_GetNodesByRange(t *testing.T) {
	sg := NewSimpleGateway(testUtil.Logger(t),
		&HandlerHelper{
			t: t,
		}, nil).(*SimpleGateway)

	nodeIDs := testUtil.UniqueNodeIDs(8)
	shared.SortNodeIDs(nodeIDs)

	for i, id := range nodeIDs {
		if i%2 == 0 {
			sg.nodes[*id] = &nodeEntry{
				lifespan:          time.Now().Add(10 * time.Minute),
				subscribingSignal: false,
				waitingSignals:    make([]signalEntry, 0),
			}
		}
	}

	tests := []struct {
		name      string
		backward  *shared.NodeID
		frontward *shared.NodeID
		expected  []*shared.NodeID
	}{
		{
			name:      "normal 1",
			backward:  nodeIDs[0],
			frontward: nodeIDs[4],
			expected:  []*shared.NodeID{nodeIDs[0], nodeIDs[2], nodeIDs[4]},
		},
		{
			name:      "normal 2",
			backward:  nodeIDs[7],
			frontward: nodeIDs[5],
			expected:  []*shared.NodeID{nodeIDs[0], nodeIDs[2], nodeIDs[4]},
		},
		{
			name:      "normal 3",
			backward:  nodeIDs[6],
			frontward: nodeIDs[2],
			expected:  []*shared.NodeID{nodeIDs[6], nodeIDs[0], nodeIDs[2]},
		},
		{
			name:      "just one node",
			backward:  nodeIDs[2],
			frontward: nodeIDs[2],
			expected:  []*shared.NodeID{nodeIDs[2]},
		},
		{
			name:      "empty range",
			backward:  nodeIDs[3],
			frontward: nodeIDs[3],
			expected:  []*shared.NodeID{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := sg.GetNodesByRange(t.Context(), tt.backward, tt.frontward)
			require.NoError(t, err)
			assert.True(t, testUtil.CompareNodeIDsUnordered(tt.expected, nodes))
		})
	}
}

func TestSimpleGateway_GetNodes(t *testing.T) {
	sg := NewSimpleGateway(testUtil.Logger(t),
		&HandlerHelper{
			t: t,
		}, nil).(*SimpleGateway)

	nodeIDs := testUtil.UniqueNodeIDs(3)

	life := time.Now().Add(10 * time.Minute)
	for _, nodeID := range nodeIDs {
		sg.nodes[*nodeID] = &nodeEntry{
			lifespan:          life,
			subscribingSignal: false,
			waitingSignals:    make([]signalEntry, 0),
		}
	}

	get, err := sg.GetNodes(t.Context())
	require.NoError(t, err)

	assert.Len(t, get, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		assert.Contains(t, get, *nodeID)
		assert.Equal(t, life, get[*nodeID])
	}
}

func TestSimpleGateway_SubscribeKeepalive(t *testing.T) {
	sg := NewSimpleGateway(testUtil.Logger(t),
		&HandlerHelper{
			t: t,
		}, nil).(*SimpleGateway)

	nodeIDs := testUtil.UniqueNodeIDs(2)
	sg.nodes[*nodeIDs[0]] = &nodeEntry{
		lifespan:          time.Now().Add(10 * time.Minute),
		subscribingSignal: false,
		waitingSignals:    make([]signalEntry, 0),
	}

	err := sg.SubscribeKeepalive(t.Context(), nodeIDs[0])
	require.NoError(t, err)

	err = sg.SubscribeKeepalive(t.Context(), nodeIDs[1])
	require.Error(t, err)
}

func TestSimpleGateway_UnsubscribeKeepalive(t *testing.T) {
	sg := NewSimpleGateway(testUtil.Logger(t),
		&HandlerHelper{
			t: t,
		}, nil).(*SimpleGateway)

	nodeIDs := testUtil.UniqueNodeIDs(2)
	sg.nodes[*nodeIDs[0]] = &nodeEntry{
		lifespan:          time.Now().Add(10 * time.Minute),
		subscribingSignal: false,
		waitingSignals:    make([]signalEntry, 0),
	}

	err := sg.UnsubscribeKeepalive(t.Context(), nodeIDs[0])
	require.NoError(t, err)

	err = sg.UnsubscribeKeepalive(t.Context(), nodeIDs[1])
	require.Error(t, err)
}

func TestSimpleGateway_PublishKeepaliveRequest(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	callCount := 0

	sg := NewSimpleGateway(testUtil.Logger(t),
		&HandlerHelper{
			t: t,
			HandleKeepaliveRequestF: func(ctx context.Context, nodeID *shared.NodeID) error {
				t.Helper()
				callCount += 1
				assert.Equal(t, *nodeIDs[0], *nodeID)
				return nil
			},
		}, nil).(*SimpleGateway)
	sg.nodes[*nodeIDs[0]] = &nodeEntry{
		lifespan:          time.Now().Add(10 * time.Minute),
		subscribingSignal: false,
		waitingSignals:    make([]signalEntry, 0),
	}

	err := sg.PublishKeepaliveRequest(t.Context(), nodeIDs[0])
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Will not call handler if node does not exist without error
	err = sg.PublishKeepaliveRequest(t.Context(), nodeIDs[1])
	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestSimpleGateway_SubscribeSignal(t *testing.T) {
	mtx := &sync.Mutex{}
	callCount := 0

	sg := NewSimpleGateway(testUtil.Logger(t),
		&HandlerHelper{
			t: t,
			HandleSignalF: func(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
				mtx.Lock()
				defer mtx.Unlock()
				callCount += 1

				assert.Equal(t, uint32(callCount), signal.GetOffer().OfferId)
				if callCount == 1 {
					assert.True(t, relayToNext)
				} else {
					assert.False(t, relayToNext)
				}

				return nil
			},
		}, nil).(*SimpleGateway)

	nodeIDs := testUtil.UniqueNodeIDs(2)
	sg.nodes[*nodeIDs[0]] = &nodeEntry{
		lifespan:          time.Now().Add(10 * time.Minute),
		subscribingSignal: false,
		waitingSignals: []signalEntry{
			{
				timestamp: time.Now(),
				signal: &proto.Signal{
					Content: &proto.Signal_Offer{
						Offer: &proto.SignalOffer{
							OfferId: 1,
						},
					},
				},
				relayToNext: true,
			},
			{
				timestamp: time.Now(),
				signal: &proto.Signal{
					Content: &proto.Signal_Offer{
						Offer: &proto.SignalOffer{
							OfferId: 2,
						},
					},
				},
				relayToNext: false,
			},
		},
	}

	mtx.Lock()
	assert.Equal(t, 0, callCount)
	mtx.Unlock()

	err := sg.SubscribeSignal(t.Context(), nodeIDs[0])
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return callCount == 2
	}, 5*time.Second, 100*time.Millisecond)
	sg.mtx.Lock()
	assert.Len(t, sg.nodes[*nodeIDs[0]].waitingSignals, 0)
	sg.mtx.Unlock()

	err = sg.SubscribeSignal(t.Context(), nodeIDs[1])
	require.Error(t, err)
}

func TestSimpleGateway_UnsubscribeSignal(t *testing.T) {
	sg := NewSimpleGateway(testUtil.Logger(t),
		&HandlerHelper{
			t: t,
		}, nil).(*SimpleGateway)

	nodeIDs := testUtil.UniqueNodeIDs(2)
	sg.nodes[*nodeIDs[0]] = &nodeEntry{
		lifespan:          time.Now().Add(10 * time.Minute),
		subscribingSignal: true,
		waitingSignals:    make([]signalEntry, 0),
	}

	err := sg.UnsubscribeSignal(t.Context(), nodeIDs[0])
	require.NoError(t, err)

	err = sg.UnsubscribeSignal(t.Context(), nodeIDs[1])
	require.Error(t, err)
}

func TestSimpleGateway_PublishSignal(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	shared.SortNodeIDs(nodeIDs)

	tests := []struct {
		name              string
		dstNodeID         *shared.NodeID
		relayToNext       bool
		expectedCallCount int
		expectedWaiting   []int
	}{
		{
			name:              "to subscribing node without relayToNext",
			dstNodeID:         nodeIDs[0],
			relayToNext:       false,
			expectedCallCount: 1,
			expectedWaiting:   []int{0, 0},
		},
		{
			name:              "to subscribing node with relayToNext",
			dstNodeID:         nodeIDs[0],
			relayToNext:       true,
			expectedCallCount: 0,
			expectedWaiting:   []int{0, 1},
		},
		{
			name:              "to non-subscribing node without relayToNext",
			dstNodeID:         nodeIDs[1],
			relayToNext:       false,
			expectedCallCount: 0,
			expectedWaiting:   []int{0, 1},
		},
		{
			name:              "to non-subscribing node with relayToNext",
			dstNodeID:         nodeIDs[1],
			relayToNext:       true,
			expectedCallCount: 1,
			expectedWaiting:   []int{0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			sg := NewSimpleGateway(testUtil.Logger(t),
				&HandlerHelper{
					t: t,
					HandleSignalF: func(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
						t.Helper()
						callCount += 1
						dstNodeID, err := shared.NewNodeIDFromProto(signal.GetDstNodeId())
						require.NoError(t, err)
						assert.Equal(t, tt.dstNodeID, dstNodeID)
						assert.Equal(t, tt.relayToNext, relayToNext)
						return nil
					},
				}, nil).(*SimpleGateway)

			sg.nodes[*nodeIDs[0]] = &nodeEntry{
				lifespan:          time.Now().Add(10 * time.Minute),
				subscribingSignal: true, // node[0] is subscribing
				waitingSignals:    make([]signalEntry, 0),
			}
			sg.nodes[*nodeIDs[1]] = &nodeEntry{
				lifespan:          time.Now().Add(10 * time.Minute),
				subscribingSignal: false, // node[1] is not subscribing
				waitingSignals:    make([]signalEntry, 0),
			}

			err := sg.PublishSignal(t.Context(), &proto.Signal{
				DstNodeId: tt.dstNodeID.Proto(),
				SrcNodeId: shared.NewRandomNodeID().Proto(),
				Content:   &proto.Signal_Answer{},
			}, tt.relayToNext)
			require.NoError(t, err)
			waiting := make([]int, len(nodeIDs))
			for i, nodeID := range nodeIDs {
				if entry, exists := sg.nodes[*nodeID]; exists {
					waiting[i] = len(entry.waitingSignals)
				}
			}
			assert.Equal(t, tt.expectedCallCount, callCount)
			assert.Equal(t, tt.expectedWaiting, waiting)
		})
	}
}
