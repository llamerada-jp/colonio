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
package routing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routingSeedHandlerHelper struct {
	setActivated func(bool)
}

func (h *routingSeedHandlerHelper) seedSetActivated(activated bool) {
	if h.setActivated != nil {
		h.setActivated(activated)
	}
}

func TestRoutingSeed_handler(t *testing.T) {
	tests := []struct {
		connectRate      uint
		neighborDistance uint32
		expectActivated  bool
	}{
		{
			connectRate:      0,
			neighborDistance: 1,
			expectActivated:  true,
		},
		{
			connectRate:      5,
			neighborDistance: 1,
			expectActivated:  false,
		},
		{
			connectRate:      5,
			neighborDistance: 10,
			expectActivated:  true,
		},
	}

	for i, test := range tests {
		func() {
			t.Logf("test %d", i)

			mtx := sync.Mutex{}
			activated := !test.expectActivated
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			rs := newRoutingSeed(&routingSeedConfig{
				ctx: ctx,
				handler: &routingSeedHandlerHelper{
					setActivated: func(a bool) {
						mtx.Lock()
						defer mtx.Unlock()
						activated = a
					},
				},
				routingExchangeInterval: 100 * time.Millisecond,
				seedConnectRate:         test.connectRate,
				seedReconnectDuration:   100 * time.Millisecond,
			})

			rs.updateSeedState(!test.expectActivated)

			// wait for change activated to expected value
			require.Eventually(t, func() bool {
				rs.recvRoutingPacket(shared.NewRandomNodeID(), &proto.Routing{
					SeedDistance: test.neighborDistance,
				})

				mtx.Lock()
				defer mtx.Unlock()
				return activated == test.expectActivated
			}, 10*time.Second, 30*time.Millisecond)

			// check activated value is kept
			for range 10 {
				time.Sleep(100 * time.Millisecond)
				mtx.Lock()
				assert.Equal(t, test.expectActivated, activated)
				mtx.Unlock()
			}
		}()
	}
}

func TestRoutingSeed_updateSeedState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rs := newRoutingSeed(&routingSeedConfig{
		ctx: ctx,
		handler: &routingSeedHandlerHelper{
			setActivated: func(bool) {
				assert.Fail(t, "unexpected call")
			},
		},
		routingExchangeInterval: 100 * time.Millisecond,
		seedConnectRate:         5,
		seedReconnectDuration:   100 * time.Millisecond,
	})

	rs.updateSeedState(true)
	assert.Contains(t, rs.seedRouteInfos, shared.NodeIDThis)
	assert.True(t, rs.getNextStep().Equal(&shared.NodeIDThis))
	content := &proto.Routing{}
	rs.setupRoutingPacket(content)
	assert.Equal(t, uint32(1), content.SeedDistance)

	assert.False(t, rs.updateSeedState(true))
	assert.True(t, rs.updateSeedState(false))
	assert.NotContains(t, rs.seedRouteInfos, shared.NodeIDThis)
	assert.True(t, rs.getNextStep().Equal(&shared.NodeIDNone))
	rs.setupRoutingPacket(content)
	assert.Equal(t, uint32(MAX_SEED_DISTANCE), content.SeedDistance)
	assert.True(t, rs.updateSeedState(true))
}

func TestRoutingSeed_updateNodeConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rs := newRoutingSeed(&routingSeedConfig{
		ctx: ctx,
		handler: &routingSeedHandlerHelper{
			setActivated: func(bool) {},
		},
		routingExchangeInterval: 1 * time.Second,
		seedConnectRate:         10,
		seedReconnectDuration:   1 * time.Second,
	})

	nodeIDs := util.UniqueNodeIDs(3)

	tests := []struct {
		neighbors      map[shared.NodeID]uint32
		connections    []*shared.NodeID
		expectNextStep *shared.NodeID
		expectDistance uint32
	}{
		{
			neighbors: map[shared.NodeID]uint32{
				*nodeIDs[0]: 7,
				*nodeIDs[1]: 6,
				*nodeIDs[2]: 5,
			},
			connections: []*shared.NodeID{
				nodeIDs[0],
				nodeIDs[1],
				nodeIDs[2],
			},
			expectNextStep: nodeIDs[2],
			expectDistance: 6,
		},
		{
			neighbors: map[shared.NodeID]uint32{
				*nodeIDs[1]: 4,
			},
			connections: []*shared.NodeID{
				nodeIDs[0],
				nodeIDs[1],
				nodeIDs[2],
			},
			expectNextStep: nodeIDs[1],
			expectDistance: 5,
		},
		{
			neighbors: map[shared.NodeID]uint32{},
			connections: []*shared.NodeID{
				nodeIDs[0],
				nodeIDs[2],
			},
			expectNextStep: nodeIDs[2],
			expectDistance: 6,
		},
		{
			neighbors:      map[shared.NodeID]uint32{},
			connections:    []*shared.NodeID{},
			expectNextStep: &shared.NodeIDNone,
			expectDistance: MAX_SEED_DISTANCE,
		},
	}

	for i, test := range tests {
		t.Log("test", i)

		for nodeID, distance := range test.neighbors {
			nodeID := nodeID
			rs.recvRoutingPacket(&nodeID, &proto.Routing{
				SeedDistance: distance,
			})
		}

		rs.updateNodeConnections(test.connections)

		assert.True(t, rs.getNextStep().Equal(test.expectNextStep))
		content := &proto.Routing{}
		rs.setupRoutingPacket(content)
		assert.Equal(t, test.expectDistance, content.SeedDistance)
	}
}

func TestRoutingSeed_cleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rs := newRoutingSeed(&routingSeedConfig{
		ctx:                     ctx,
		handler:                 &routingSeedHandlerHelper{},
		routingExchangeInterval: 200 * time.Millisecond,
		seedConnectRate:         10,
		seedReconnectDuration:   1 * time.Second,
	})

	nodeIDs := util.UniqueNodeIDs(3)

	rs.recvRoutingPacket(nodeIDs[0], &proto.Routing{
		SeedDistance: 8,
	})
	rs.recvRoutingPacket(nodeIDs[1], &proto.Routing{
		SeedDistance: 7,
	})
	rs.recvRoutingPacket(nodeIDs[2], &proto.Routing{
		SeedDistance: 6,
	})
	assert.False(t, rs.cleanup())
	assert.True(t, rs.getNextStep().Equal(nodeIDs[2]))
	assert.Eventually(t, func() bool {
		rs.recvRoutingPacket(nodeIDs[1], &proto.Routing{
			SeedDistance: 7,
		})
		return rs.cleanup() && rs.getNextStep().Equal(nodeIDs[1])
	}, 5*time.Second, 100*time.Millisecond)
	assert.False(t, rs.cleanup())
	assert.Eventually(t, func() bool {
		return rs.cleanup() && rs.getNextStep().Equal(&shared.NodeIDNone)
	}, 5*time.Second, 100*time.Millisecond)
}
