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
package seed_accessor

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/internal/network/signal"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	testServer "github.com/llamerada-jp/colonio/test/util/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type seedAccessorHandlerHelper struct {
	errorFunc        func(error)
	recvSignalOffer  func(*shared.NodeID, *signal.Offer)
	recvSignalAnswer func(*shared.NodeID, *signal.Answer)
	recvSignalICE    func(*shared.NodeID, *signal.ICE)
}

func (h *seedAccessorHandlerHelper) SeedError(err error) {
	if h.errorFunc != nil {
		h.errorFunc(err)
	}
}

func (h *seedAccessorHandlerHelper) SeedRecvSignalOffer(srcNodeID *shared.NodeID, offer *signal.Offer) {
	if h.recvSignalOffer != nil {
		h.recvSignalOffer(srcNodeID, offer)
	}
}

func (h *seedAccessorHandlerHelper) SeedRecvSignalAnswer(srcNodeID *shared.NodeID, answer *signal.Answer) {
	if h.recvSignalAnswer != nil {
		h.recvSignalAnswer(srcNodeID, answer)
	}
}

func (h *seedAccessorHandlerHelper) SeedRecvSignalICE(srcNodeID *shared.NodeID, ice *signal.ICE) {
	if h.recvSignalICE != nil {
		h.recvSignalICE(srcNodeID, ice)
	}
}

func TestSeedAccessor_connection(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	nodeCount := 0

	seed := seed.NewSeed(
		seed.WithConnectionHandler(&testUtil.ConnectionHandlerHelper{
			T: t,
			AssignNodeIDF: func(ctx context.Context) (*shared.NodeID, error) {
				// return nodeID for 1st and 2nd node
				if nodeCount < len(nodeIDs) {
					nodeID := nodeIDs[nodeCount]
					nodeCount++
					return nodeID, nil
				}
				// return error when 3rd node is requested
				return nil, fmt.Errorf("error")
			},
			UnassignF: func(nodeID *shared.NodeID) {},
		}),
		seed.WithPollingInterval(3*time.Second),
	)
	server := testServer.NewHelper(seed)
	server.Start(t.Context())
	defer server.Stop()

	// 1st node can get nodeID and is alone
	accessor1 := newAccessor(server, nil, nil)
	ctx1, cancel1 := context.WithCancel(t.Context())
	nodeID1, err := accessor1.Start(ctx1)
	require.NoError(t, err)
	assert.True(t, nodeID1.Equal(nodeIDs[0]))
	assert.True(t, accessor1.IsAlone())

	time.Sleep(1 * time.Second)

	// 2nd node can get nodeID and is not alone
	accessor2 := newAccessor(server, nil, nil)
	nodeID2, err := accessor2.Start(t.Context())
	require.NoError(t, err)
	assert.True(t, nodeID2.Equal(nodeIDs[1]))
	assert.False(t, accessor2.IsAlone())

	// 3rd node can't start by error
	accessor3 := newAccessor(server, nil, nil)
	_, err = accessor3.Start(t.Context())
	assert.NotNil(t, err)

	// stop to renew session
	cancel1()

	// 2nd node will be alone
	assert.Eventually(t, func() bool {
		err := accessor2.SendSignalOffer(nodeID2, &signal.Offer{
			OfferID:   1,
			OfferType: signal.OfferTypeExplicit,
			Sdp:       "test",
		})
		assert.NoError(t, err)
		return accessor2.IsAlone()
	}, 5*time.Second, 500*time.Millisecond)

	// keep session by sending signal
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)
		err := accessor2.SendSignalOffer(nodeID2, &signal.Offer{
			OfferID:   1,
			OfferType: signal.OfferTypeExplicit,
			Sdp:       "test",
		})
		assert.NoError(t, err)
	}
}

func newAccessor(server *testServer.Helper, logger *slog.Logger, handler *seedAccessorHandlerHelper) *SeedAccessor {
	if logger == nil {
		logger = slog.Default()
	}

	if handler == nil {
		handler = &seedAccessorHandlerHelper{}
	}

	return NewSeedAccessor(&Config{
		Logger:     logger,
		Handler:    handler,
		URL:        server.URL(),
		HttpClient: testUtil.NewInsecureHttpClient(),
	})
}

func TestSeedAccessor_SignalingKind(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	nodeCount := 0

	// create seed
	seed := seed.NewSeed(
		seed.WithConnectionHandler(&testUtil.ConnectionHandlerHelper{
			T: t,
			AssignNodeIDF: func(ctx context.Context) (*shared.NodeID, error) {
				if nodeCount < len(nodeIDs) {
					nodeID := nodeIDs[nodeCount]
					nodeCount++
					return nodeID, nil
				}
				return nil, fmt.Errorf("error")
			},
			UnassignF: func(nodeID *shared.NodeID) {},
		}),
	)
	server := testServer.NewHelper(seed)
	server.Start(t.Context())
	defer server.Stop()

	// src accessor
	srcAccessor := newAccessor(server, nil, nil)
	_, err := srcAccessor.Start(t.Context())
	require.NoError(t, err)

	// dst accessor
	mtx := sync.Mutex{}
	receivedCounts := make([]int, 3)
	dstAccessor := newAccessor(server, nil, &seedAccessorHandlerHelper{
		errorFunc: func(err error) {
			assert.Fail(t, "error", err)
		},
		// receive signals and check
		recvSignalOffer: func(srcNodeID *shared.NodeID, offer *signal.Offer) {
			mtx.Lock()
			defer mtx.Unlock()
			receivedCounts[0]++
			assert.True(t, srcNodeID.Equal(nodeIDs[0]))
			assert.Equal(t, uint32(1), offer.OfferID)
			assert.Equal(t, signal.OfferTypeExplicit, offer.OfferType)
			assert.Equal(t, "offer", offer.Sdp)
		},
		recvSignalAnswer: func(srcNodeID *shared.NodeID, answer *signal.Answer) {
			mtx.Lock()
			defer mtx.Unlock()
			receivedCounts[1]++
			assert.True(t, srcNodeID.Equal(nodeIDs[0]))
			assert.Equal(t, uint32(2), answer.OfferID)
			assert.Equal(t, signal.AnswerStatus(3), answer.Status)
			assert.Equal(t, "answer", answer.Sdp)
		},
		recvSignalICE: func(srcNodeID *shared.NodeID, ice *signal.ICE) {
			mtx.Lock()
			defer mtx.Unlock()
			receivedCounts[2]++
			assert.True(t, srcNodeID.Equal(nodeIDs[0]))
			assert.Equal(t, uint32(3), ice.OfferID)
			assert.Equal(t, []string{"ice"}, ice.Ices)
		},
	})
	_, err = dstAccessor.Start(t.Context())
	require.NoError(t, err)

	// send signals
	srcAccessor.SendSignalOffer(nodeIDs[1], &signal.Offer{
		OfferID:   1,
		OfferType: signal.OfferTypeExplicit,
		Sdp:       "offer",
	})
	srcAccessor.SendSignalAnswer(nodeIDs[1], &signal.Answer{
		OfferID: 2,
		Status:  signal.AnswerStatus(3),
		Sdp:     "answer",
	})
	srcAccessor.SendSignalICE(nodeIDs[1], &signal.ICE{
		OfferID: 3,
		Ices:    []string{"ice"},
	})

	// wait for receiving signals
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return receivedCounts[0] == 1 && receivedCounts[1] == 1 && receivedCounts[2] == 1
	}, 5*time.Second, 500*time.Millisecond)
}

func TestSeedAccessor_SignalingTarget(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	slices.SortFunc(nodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})
	nodeCount := 0

	// setup seed
	seed := seed.NewSeed(
		seed.WithConnectionHandler(&testUtil.ConnectionHandlerHelper{
			T: t,
			AssignNodeIDF: func(ctx context.Context) (*shared.NodeID, error) {
				if nodeCount < len(nodeIDs) {
					nodeID := nodeIDs[nodeCount]
					nodeCount++
					return nodeID, nil
				}
				return nil, fmt.Errorf("error")
			},
			UnassignF: func(nodeID *shared.NodeID) {},
		}),
	)
	server := testServer.NewHelper(seed)
	server.Start(t.Context())
	defer server.Stop()

	accessors := make([]*SeedAccessor, 0, len(nodeIDs))
	mtx := sync.Mutex{}
	receivedFrom := make([][]*shared.NodeID, len(nodeIDs))
	for i := range nodeIDs {
		accessor := newAccessor(server, nil,
			&seedAccessorHandlerHelper{
				recvSignalOffer: func(srcNodeID *shared.NodeID, offer *signal.Offer) {
					mtx.Lock()
					defer mtx.Unlock()
					receivedFrom[i] = append(receivedFrom[i], srcNodeID)
				},
			},
		)
		_, err := accessor.Start(t.Context())
		require.NoError(t, err)
		accessors = append(accessors, accessor)
	}

	// send signal to explicit destination
	for i := range nodeIDs {
		err := accessors[i].SendSignalOffer(nodeIDs[mod(i+1, len(nodeIDs))], &signal.Offer{
			OfferID:   uint32(i),
			OfferType: signal.OfferTypeExplicit,
		})
		assert.NoError(t, err)
	}

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		for i := range receivedFrom {
			if len(receivedFrom[i]) != 1 {
				return false
			}
			if !receivedFrom[i][0].Equal(nodeIDs[mod(i+len(nodeIDs)-1, len(nodeIDs))]) {
				return false
			}
		}
		return true
	}, 5*time.Second, 500*time.Millisecond)

	// send signal to next destination
	mtx.Lock()
	receivedFrom = make([][]*shared.NodeID, len(nodeIDs))
	mtx.Unlock()

	for i, dstNodeID := range nodeIDs {
		err := accessors[i].SendSignalOffer(dstNodeID, &signal.Offer{
			OfferID:   uint32(i),
			OfferType: signal.OfferTypeNext,
		})
		assert.NoError(t, err)
	}

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		for i := range receivedFrom {
			if len(receivedFrom[i]) != 1 {
				return false
			}
			if !receivedFrom[i][0].Equal(nodeIDs[mod(i+len(nodeIDs)-1, len(nodeIDs))]) {
				return false
			}
		}
		return true
	}, 5*time.Second, 500*time.Millisecond)
}

func mod(a, b int) int {
	return int(math.Mod(float64(a), float64(b)))
}
