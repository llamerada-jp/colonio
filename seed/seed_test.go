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
package seed

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	service "github.com/llamerada-jp/colonio/api/colonio/v1alpha/v1alphaconnect"
	"github.com/llamerada-jp/colonio/internal/shared"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startServer(t *testing.T, ctx context.Context, seed *Seed) uint16 {
	cert := os.Getenv("COLONIO_TEST_CERT")
	key := os.Getenv("COLONIO_TEST_KEY")
	if cert == "" || key == "" {
		panic("Please set COLONIO_TEST_CERT and COLONIO_TEST_KEY")
	}

	port := 8000 + uint16(rand.Uint32()%1000)
	mux := http.NewServeMux()

	seed.RegisterService(mux)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		seed.Run(ctx)
	}()

	go func() {
		err := server.ListenAndServeTLS(cert, key)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("http server start error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		err := server.Shutdown(context.Background())
		if err != nil {
			t.Logf("http server shutdown error: %v", err)
		}
	}()

	// wait for server start
	client := testUtil.NewInsecureHttpClient()
	for {
		_, err := client.Get(fmt.Sprintf("https://localhost:%d/", port))
		if err == nil {
			break
		}
	}

	return port
}

func createClient(_ *testing.T, _ context.Context, port uint16) service.SeedServiceClient {
	return service.NewSeedServiceClient(testUtil.NewInsecureHttpClient(), fmt.Sprintf("https://localhost:%d/", port))
}

func TestAssignNode(t *testing.T) {
	seed := NewSeed()
	port := startServer(t, t.Context(), seed)

	// the first node
	client1 := createClient(t, t.Context(), port)
	resAn, err := client1.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{})
	require.NoError(t, err)
	// check response
	assert.True(t, resAn.Msg.IsAlone)
	assert.NotNil(t, resAn.Msg.NodeId)
	nodeID1, err := shared.NewNodeIDFromProto(resAn.Msg.NodeId)
	require.NoError(t, err)
	assert.NotNil(t, nodeID1)
	assert.True(t, nodeID1.IsNormal())
	// check seed
	assert.Len(t, seed.nodes, 1)

	// the second node
	client2 := createClient(t, t.Context(), port)
	resAn, err = client2.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{})
	require.NoError(t, err)
	// check response
	assert.False(t, resAn.Msg.IsAlone)
	nodeID2, err := shared.NewNodeIDFromProto(resAn.Msg.NodeId)
	require.NoError(t, err)
	assert.NotNil(t, nodeID2)
	assert.True(t, nodeID2.IsNormal())
	// check seed
	assert.Len(t, seed.nodes, 2)

	assert.NotEqual(t, nodeID1, nodeID2)

	// Unassign client1
	_, err = client1.UnassignNode(t.Context(), &connect.Request[proto.UnassignNodeRequest]{})
	require.NoError(t, err)
	// check seed
	assert.Len(t, seed.nodes, 1)
}

func TestSession(t *testing.T) {
	seed := NewSeed(WithPollingInterval(1 * time.Second))
	port := startServer(t, t.Context(), seed)

	client := createClient(t, t.Context(), port)

	// deny if the session is not created yet
	_, err := client.SendSignal(t.Context(), &connect.Request[proto.SendSignalRequest]{
		Msg: &proto.SendSignalRequest{
			Signal: &proto.Signal{
				DstNodeId: shared.NewRandomNodeID().Proto(),
				SrcNodeId: shared.NewRandomNodeID().Proto(),
				Content: &proto.Signal_Offer{
					Offer: &proto.SignalOffer{},
				},
			},
		},
	})
	assert.Error(t, err)

	res, err := client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{})
	require.NoError(t, err)

	nodeID, err := shared.NewNodeIDFromProto(res.Msg.NodeId)
	require.NoError(t, err)

	// will success with valid session
	_, err = client.SendSignal(t.Context(), &connect.Request[proto.SendSignalRequest]{
		Msg: &proto.SendSignalRequest{
			Signal: &proto.Signal{
				DstNodeId: shared.NewRandomNodeID().Proto(),
				SrcNodeId: nodeID.Proto(),
				Content: &proto.Signal_Offer{
					Offer: &proto.SignalOffer{},
				},
			},
		},
	})
	assert.NoError(t, err)

	// deny if the session is outdated
	time.Sleep(2 * time.Second)
	_, err = client.SendSignal(t.Context(), &connect.Request[proto.SendSignalRequest]{
		Msg: &proto.SendSignalRequest{
			Signal: &proto.Signal{
				DstNodeId: shared.NewRandomNodeID().Proto(),
				SrcNodeId: nodeID.Proto(),
				Content: &proto.Signal_Offer{
					Offer: &proto.SignalOffer{},
				},
			},
		},
	})
	assert.Error(t, err)
}

func TestWithAssignmentHandler(t *testing.T) {
	nodeID := shared.NewRandomNodeID()
	var mtx sync.Mutex
	unassignCalled := false

	seed := NewSeed(
		WithPollingInterval(1*time.Second),
		WithAssignmentHandler(
			&testUtil.AssignmentHandlerHelper{
				T: t,
				AssignNodeF: func(context.Context) (*shared.NodeID, error) {
					return nodeID, nil
				},
				UnassignNodeF: func(n *shared.NodeID) {
					assert.Equal(t, nodeID, n)

					mtx.Lock()
					defer mtx.Unlock()
					unassignCalled = true
				},
			}),
	)
	port := startServer(t, t.Context(), seed)

	client := createClient(t, t.Context(), port)
	res, err := client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{
		Msg: &proto.AssignNodeRequest{},
	})
	require.NoError(t, err)
	resNodeID, err := shared.NewNodeIDFromProto(res.Msg.NodeId)
	require.NoError(t, err)
	assert.Equal(t, nodeID, resNodeID)

	// unassign should be called when the connection is closed
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return unassignCalled
	}, 10*time.Second, 100*time.Millisecond)
}

func TestRelayPoll(t *testing.T) {
	mtx := sync.Mutex{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	clients := make([]service.SeedServiceClient, 2)
	received := make([][]*proto.Signal, 2)

	n := 0
	seed := NewSeed(WithAssignmentHandler(&testUtil.AssignmentHandlerHelper{
		T: t,
		AssignNodeF: func(context.Context) (*shared.NodeID, error) {
			nodeID := nodeIDs[n]
			n++
			return nodeID, nil
		},
	}))
	port := startServer(t, t.Context(), seed)

	for i := 0; i < 2; i++ {
		clients[i] = createClient(t, t.Context(), port)
		received[i] = []*proto.Signal{}

		_, err := clients[i].AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{
			Msg: &proto.AssignNodeRequest{},
		})
		require.NoError(t, err)

		sc, err := clients[i].PollSignal(t.Context(), &connect.Request[proto.PollSignalRequest]{
			Msg: &proto.PollSignalRequest{},
		})
		require.NoError(t, err)

		go func(i int, sc *connect.ServerStreamForClient[proto.PollSignalResponse]) {
			for sc.Receive() {
				res := sc.Msg()
				mtx.Lock()
				received[i] = append(received[i], res.Signals...)
				mtx.Unlock()
			}
		}(i, sc)
	}

	errorCases := []*proto.Signal{
		{
			// src node is not matched
			DstNodeId: nodeIDs[1].Proto(),
			SrcNodeId: nodeIDs[1].Proto(),
			Content: &proto.Signal_Offer{
				Offer: &proto.SignalOffer{},
			},
		},
		{
			// dst node is not exist and mode is `explicit`
			DstNodeId: nodeIDs[2].Proto(),
			SrcNodeId: nodeIDs[0].Proto(),
			Content: &proto.Signal_Offer{
				Offer: &proto.SignalOffer{
					Type: proto.SignalOfferType_SIGNAL_OFFER_TYPE_EXPLICIT,
				},
			},
		},
	}

	for _, p := range errorCases {
		_, err := clients[0].SendSignal(t.Context(), &connect.Request[proto.SendSignalRequest]{
			Msg: &proto.SendSignalRequest{
				Signal: p,
			},
		})
		assert.Error(t, err)
	}

	mtx.Lock()
	assert.Len(t, received[0], 0)
	assert.Len(t, received[1], 0)
	mtx.Unlock()

	normalCases := []*proto.Signal{
		{
			DstNodeId: nodeIDs[1].Proto(),
			SrcNodeId: nodeIDs[0].Proto(),
			Content: &proto.Signal_Offer{
				Offer: &proto.SignalOffer{
					OfferId: 1,
					Type:    proto.SignalOfferType_SIGNAL_OFFER_TYPE_EXPLICIT,
				},
			},
		},
		{
			DstNodeId: nodeIDs[2].Proto(),
			SrcNodeId: nodeIDs[0].Proto(),
			Content: &proto.Signal_Offer{
				Offer: &proto.SignalOffer{
					OfferId: 2,
					Type:    proto.SignalOfferType_SIGNAL_OFFER_TYPE_NEXT,
				},
			},
		},
	}

	// reset received packets
	mtx.Lock()
	for i := 0; i < 2; i++ {
		received[i] = []*proto.Signal{}
	}
	mtx.Unlock()

	// send packet
	for _, p := range normalCases {
		_, err := clients[0].SendSignal(t.Context(), &connect.Request[proto.SendSignalRequest]{
			Msg: &proto.SendSignalRequest{
				Signal: p,
			},
		})
		assert.NoError(t, err)
	}
	// check received packets
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return len(received[1]) >= 1 && len(received[0])+len(received[1]) == 2
	}, 10*time.Second, 100*time.Millisecond)
}
