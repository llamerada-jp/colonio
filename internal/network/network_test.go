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
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/config"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/network/node_accessor"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/observation"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed"
	testUtil "github.com/llamerada-jp/colonio/test/util"
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

func newTestConfigBase(seedURL string, i int) *Config {
	return &Config{
		Logger:           slog.Default().With(slog.String("node", fmt.Sprintf("#%d", i))),
		Handler:          &networkHandlerHelper{},
		Observation:      &observation.Handlers{},
		CoordinateSystem: geometry.NewPlaneCoordinateSystem(-1.0, 1.0, -1.0, 1.0),
		HttpClient:       testUtil.NewInsecureHttpClient(),
		SeedURL:          seedURL,
		ICEServers: []config.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
		NLC: &node_accessor.NodeLinkConfig{
			SessionTimeout:    5 * time.Second,
			KeepaliveInterval: 1 * time.Second,
			BufferInterval:    10 * time.Millisecond,
			PacketBaseBytes:   1024,
		},
		RoutingExchangeInterval: 1 * time.Second,
		PacketHopLimit:          10,
		NextConnectionInterval:  5 * time.Second,
	}
}

func TestNetwork(t *testing.T) {
	mtx := sync.Mutex{}
	nodeIDs := testUtil.UniqueNodeIDs(5)
	networks := make([]*Network, len(nodeIDs))
	positionMaps := make([]map[shared.NodeID]*geometry.Coordinate, len(nodeIDs))

	// start seed
	nodeCount := 0
	seed := seed.NewSeed(
		seed.WithConnectionHandler(&testUtil.ConnectionHandlerHelper{
			T: t,
			AssignNodeIDF: func(ctx context.Context) (*shared.NodeID, error) {
				if nodeCount >= len(nodeIDs) {
					t.FailNow()
				}
				nodeID := nodeIDs[nodeCount]
				nodeCount++
				return nodeID, nil
			},
			UnassignF: func(nodeID *shared.NodeID) {},
		}),
	)
	server := server.NewHelper(seed)
	server.Start(t.Context())
	defer server.Stop()

	// make the first node
	config := newTestConfigBase(server.URL(), 0)
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

	nodeID, err := networks[0].Start(t.Context())
	require.NoError(t, err)
	require.True(t, nodeID.Equal(nodeIDs[0]))

	// can be online only one node
	require.Eventually(t, func() bool {
		return networks[0].IsOnline()
	}, 60*time.Second, 1*time.Second)

	// make other nodes online
	for i := 1; i < len(nodeIDs); i++ {
		i := i
		config := newTestConfigBase(server.URL(), i)
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

		nodeID, err = networks[i].Start(t.Context())
		require.NoError(t, err)
		require.True(t, nodeID.Equal(nodeIDs[i]))
	}

	// all nodes should be online
	require.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		for _, network := range networks {
			if !network.IsOnline() {
				return false
			}
		}
		return true
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

	// position should be tolled to each node
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()

		for _, pMap := range positionMaps {
			for _, nodeID := range nodeIDs {
				expected := expectedPositionMap[*nodeID]
				if pos, ok := pMap[*nodeID]; ok {
					if pos.X != expected.X || pos.Y != expected.Y {
						return false
					}
				}
			}
		}

		return true
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

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()

		for _, network := range networks {
			assert.True(t, network.IsOnline())
		}

		return len(receivedPackets) == 2
	}, 60*time.Second, 1*time.Second)

	for _, n := range networks {
		assert.True(t, n.IsOnline())
	}
}
