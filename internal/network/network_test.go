//go:build !js

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
package network

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"slices"
	"sync"
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/network/node_accessor"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/test/util/helper"
	"github.com/llamerada-jp/colonio/test/util/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type networkHandlerHelper struct {
	updateNextNodePosition func(map[shared.NodeID]*geometry.Coordinate)
}

func (h *networkHandlerHelper) NetworkUpdateNextNodePosition(p map[shared.NodeID]*geometry.Coordinate) {
	if h.updateNextNodePosition != nil {
		h.updateNextNodePosition(p)
	}
}

func newTestConfigBase(t *testing.T, seedURL string, i int) *Config {
	return &Config{
		Logger:           testUtil.Logger(t).With(slog.String("node", fmt.Sprintf("#%d", i))),
		Handler:          &networkHandlerHelper{},
		Observation:      &config.ObservationHandler{},
		CoordinateSystem: geometry.NewPlaneCoordinateSystem(-1.0, 1.0, -1.0, 1.0),
		HttpClient:       testUtil.NewInsecureHttpClient(),
		SeedURL:          seedURL,
		NLC: &node_accessor.NodeLinkConfig{
			ICEServers:        constants.TestingICEServers,
			SessionTimeout:    60 * time.Second,
			KeepaliveInterval: 5 * time.Second,
			BufferInterval:    10 * time.Millisecond,
			PacketBaseBytes:   1024,
		},
		PacketHopLimit: 10,
	}
}

func TestNetwork(t *testing.T) {
	mtx := sync.Mutex{}
	nodeIDs := testUtil.UniqueNodeIDs(10)
	networks := make([]*Network, len(nodeIDs))
	positionMaps := make([]map[shared.NodeID]*geometry.Coordinate, len(nodeIDs))
	ctx, cancel := context.WithCancel(t.Context())

	// start seed
	gateway := &helper.Gateway{
		NodeIDs: nodeIDs,
	}
	seed := seed.NewSeed(
		seed.WithGateway(gateway),
	)
	gateway.Seed = seed
	server := server.NewHelper(seed)
	server.Start(t.Context())
	defer server.Stop()

	// make the first node
	config := newTestConfigBase(t, server.URL(), 0)
	config.Handler = &networkHandlerHelper{
		updateNextNodePosition: func(m map[shared.NodeID]*geometry.Coordinate) {
			mtx.Lock()
			defer mtx.Unlock()
			positionMaps[0] = m
		},
	}
	var err error
	networks[0], err = NewNetwork(config)
	require.NoError(t, err)

	// only the first node can receive packet
	receivedPackets := make([]*shared.Packet, 0)
	transferer.SetRequestHandler[proto.PacketContent_Error](networks[0].GetTransferer(), func(p *shared.Packet) {
		mtx.Lock()
		defer mtx.Unlock()
		receivedPackets = append(receivedPackets, p)
	})

	nodeID, err := networks[0].Start(ctx)
	require.NoError(t, err)
	require.True(t, nodeID.Equal(nodeIDs[0]))

	// can be online only one node
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		st, _ := networks[0].GetStability()
		assert.True(c, networks[0].IsOnline())
		assert.True(c, st)
	}, 60*time.Second, 1*time.Second)

	// make other nodes online
	for i := 1; i < len(nodeIDs); i++ {
		i := i
		config := newTestConfigBase(t, server.URL(), i)
		config.Handler = &networkHandlerHelper{
			updateNextNodePosition: func(m map[shared.NodeID]*geometry.Coordinate) {
				mtx.Lock()
				defer mtx.Unlock()
				positionMaps[i] = m
			},
		}
		networks[i], err = NewNetwork(config)
		require.NoError(t, err)

		transferer.SetRequestHandler[proto.PacketContent_Error](networks[i].GetTransferer(), func(p *shared.Packet) {
			assert.Fail(t, "should not receive packet")
		})

		nodeID, err = networks[i].Start(ctx)
		require.NoError(t, err)
		require.True(t, nodeID.Equal(nodeIDs[i]))
	}

	// all nodes should be online
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		for _, network := range networks {
			st, _ := network.GetStability()
			assert.True(c, network.IsOnline())
			assert.True(c, st)
		}
	}, 60*time.Second, 1*time.Second)

	expectedPositionMap := make(map[shared.NodeID]*geometry.Coordinate)
	for i, network := range networks {
		x := rand.Float64() - 0.5
		y := rand.Float64() - 0.5
		expectedPositionMap[*nodeIDs[i]] = &geometry.Coordinate{X: x, Y: y}
		t.Logf("node %d: %f, %f\n", i, x, y)
		err = network.UpdateLocalPosition(&geometry.Coordinate{X: x, Y: y})
		require.NoError(t, err)
	}

	// position should be told to each node
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()

		for _, pMap := range positionMaps {
			for _, nodeID := range nodeIDs {
				expected := expectedPositionMap[*nodeID]
				if pos, ok := pMap[*nodeID]; ok {
					assert.Equal(c, expected.X, pos.X)
					assert.Equal(c, expected.Y, pos.Y)
				}
			}
		}
	}, 60*time.Second, 1*time.Second)

	// send packet
	networks[1].GetTransferer().RequestOneWay(nodeIDs[0], shared.PacketModeExplicit, &proto.PacketContent{
		Content: &proto.PacketContent_Error{
			Error: &proto.Error{
				Code:    1,
				Message: "test",
			},
		},
	})

	networks[2].GetTransferer().RequestOneWay(nodeIDs[0].Add(shared.NewNormalNodeID(0, 1)), shared.PacketModeNone, &proto.PacketContent{
		Content: &proto.PacketContent_Error{
			Error: &proto.Error{
				Code:    2,
				Message: "test",
			},
		},
	})

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		mtx.Lock()
		defer mtx.Unlock()

		for _, network := range networks {
			assert.True(t, network.IsOnline())
			st, _ := network.GetStability()
			assert.True(t, st)
		}
		assert.Len(c, receivedPackets, 2)
	}, 60*time.Second, 1*time.Second)

	sortedNodeIDs := slices.Clone(nodeIDs)
	slices.SortFunc(sortedNodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})
	for i, network := range networks {
		nodeID := nodeIDs[i]
		_, nextNodeIDs := network.GetStability()
		assert.Len(t, nextNodeIDs, 4)

		j := slices.IndexFunc(sortedNodeIDs, func(id *shared.NodeID) bool {
			return nodeID.Equal(id)
		})
		for k, l := range []int{-2, -1, 1, 2} {
			nextNodeID := nextNodeIDs[k]
			assert.Contains(t, nodeIDs, nextNodeID)
			expectedIdx := (j + l + len(nodeIDs)) % len(nodeIDs)
			assert.Equal(t, sortedNodeIDs[expectedIdx], nextNodeID)
		}
	}

	cancel()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		for _, network := range networks {
			assert.False(c, network.IsOnline())
		}
	}, 60*time.Second, 1*time.Second)
}
