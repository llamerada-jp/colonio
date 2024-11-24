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
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/observation"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/test/testing_seed"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
)

type networkHandlerHelper struct {
	recvConfig             func(*config.Cluster)
	updateNextNodePosition func(map[shared.NodeID]*geometry.Coordinate)
}

func (h *networkHandlerHelper) NetworkRecvConfig(c *config.Cluster) {
	if h.recvConfig != nil {
		h.recvConfig(c)
	}
}

func (h *networkHandlerHelper) NetworkUpdateNextNodePosition(p map[shared.NodeID]*geometry.Coordinate) {
	if h.updateNextNodePosition != nil {
		h.updateNextNodePosition(p)
	}
}

func TestNetwork_Connect_alone(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(1)

	testSeed := testing_seed.NewTestingSeed()
	defer testSeed.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	configReceived := false

	net := NewNetwork(&Config{
		Ctx:         ctx,
		Logger:      slog.Default(),
		LocalNodeID: nodeIDs[0],
		Handler: &networkHandlerHelper{
			recvConfig: func(c *config.Cluster) {
				assert.NotNil(t, c)
				mtx.Lock()
				defer mtx.Unlock()
				configReceived = true
			},
		},
		Observation:      &observation.Handlers{},
		Insecure:         true,
		SeedTripInterval: 1 * time.Minute,
	})

	err := net.Connect(testSeed.URL(), nil)
	assert.NoError(t, err)

	assert.Eventually(t, func() bool {
		return net.IsOnline()
	}, 60*time.Second, 1*time.Second)

	assert.True(t, configReceived)

	net.Disconnect()
	assert.False(t, net.IsOnline())
}

func TestNetwork_Connect(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)

	testSeed := testing_seed.NewTestingSeed()
	defer testSeed.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	configReceived := 0
	net := make([]*Network, 2)

	for i := 0; i < 2; i++ {
		net[i] = NewNetwork(&Config{
			Ctx:         ctx,
			Logger:      slog.Default(),
			LocalNodeID: nodeIDs[i],
			Handler: &networkHandlerHelper{
				recvConfig: func(c *config.Cluster) {
					assert.NotNil(t, c)
					mtx.Lock()
					defer mtx.Unlock()
					configReceived += 1
				},
			},
			Observation:      &observation.Handlers{},
			Insecure:         true,
			SeedTripInterval: 1 * time.Minute,
		})

		err := net[i].Connect(testSeed.URL(), nil)
		assert.NoError(t, err)
	}

	assert.Eventually(t, func() bool {
		for _, n := range net {
			if !n.IsOnline() {
				return false
			}
		}
		return true
	}, 60*time.Second, 1*time.Second)

	assert.Equal(t, len(net), configReceived)

	for _, n := range net {
		n.Disconnect()
		assert.False(t, n.IsOnline())
	}
}

func TestNetwork_UpdateLocalPosition(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)

	testSeed := testing_seed.NewTestingSeed(
		testing_seed.WithGeometrySphere(100.0),
		testing_seed.WithSpread(time.Minute, 4096),
	)
	defer testSeed.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	net := make([]*Network, 2)
	positions := make([]map[shared.NodeID]*geometry.Coordinate, 2)

	for i := 0; i < 2; i++ {
		net[i] = NewNetwork(&Config{
			Ctx:         ctx,
			Logger:      slog.Default(),
			LocalNodeID: nodeIDs[i],
			Handler: &networkHandlerHelper{
				updateNextNodePosition: func(p map[shared.NodeID]*geometry.Coordinate) {
					assert.NotNil(t, p)
					mtx.Lock()
					defer mtx.Unlock()
					positions[i] = p
				},
			},
			Observation:      &observation.Handlers{},
			Insecure:         true,
			SeedTripInterval: 1 * time.Minute,
		})

		err := net[i].Connect(testSeed.URL(), nil)
		assert.NoError(t, err)
	}

	assert.Eventually(t, func() bool {
		return net[0].IsOnline() && net[1].IsOnline()
	}, 60*time.Second, time.Second)

	assert.Eventually(t, func() bool {
		err := net[0].UpdateLocalPosition(&geometry.Coordinate{X: 1.0, Y: 1.0})
		assert.NoError(t, err)
		err = net[1].UpdateLocalPosition(&geometry.Coordinate{X: 2.0, Y: 2.0})
		assert.NoError(t, err)

		mtx.Lock()
		defer mtx.Unlock()

		return len(positions[0]) == 1 && len(positions[1]) == 1 &&
			positions[1][*nodeIDs[0]].X == 1.0 && positions[1][*nodeIDs[0]].Y == 1.0 &&
			positions[0][*nodeIDs[1]].X == 2.0 && positions[0][*nodeIDs[1]].Y == 2.0
	}, 60*time.Second, 1*time.Second)

	for i := 0; i < 2; i++ {
		net[i].Disconnect()
	}
}
