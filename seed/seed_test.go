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
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"slices"
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

func startServer(t *testing.T, ctx context.Context, seed *Controller) uint16 {
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

func TestSeed_AssignNode(t *testing.T) {
	seed := NewController()
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
	seed := NewController(WithPollingInterval(1 * time.Second))
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

func TestSeed_AssignNodeWithAssignmentHandler(t *testing.T) {
	nodeID := shared.NewRandomNodeID()
	var mtx sync.Mutex
	unassignCalled := false

	seed := NewController(
		WithPollingInterval(1*time.Second),
		WithAssignmentHandler(
			&AssignmentHandlerHelper{
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

func TestSeed_SendSignal(t *testing.T) {
	mtx := sync.Mutex{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	clients := make([]service.SeedServiceClient, 2)
	received := make([][]*proto.Signal, 2)

	n := 0
	seed := NewController(WithAssignmentHandler(&AssignmentHandlerHelper{
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

func TestSeed_ReconcileNextNodes_oneNode(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	slices.SortFunc(nodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})
	n := 0
	seed := NewController(WithAssignmentHandler(&AssignmentHandlerHelper{
		T: t,
		AssignNodeF: func(context.Context) (*shared.NodeID, error) {
			nodeID := nodeIDs[n]
			n++
			return nodeID, nil
		},
		UnassignNodeF: func(nodeID *shared.NodeID) {},
	}))
	port := startServer(t, t.Context(), seed)
	client := createClient(t, t.Context(), port)
	_, err := client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{
		Msg: &proto.AssignNodeRequest{},
	})
	require.NoError(t, err)

	tests := []struct {
		description     string
		nextNodeIDs     []*shared.NodeID
		disconnectedIDs []*shared.NodeID
		expectMatched   bool
	}{
		{
			description:     "no one connected",
			nextNodeIDs:     []*shared.NodeID{},
			disconnectedIDs: nil,
			expectMatched:   true,
		},
		{
			description:     "disconnected from not exist node",
			nextNodeIDs:     []*shared.NodeID{},
			disconnectedIDs: []*shared.NodeID{nodeIDs[1]},
			expectMatched:   true,
		},
		{
			description:     "next node is not exist",
			nextNodeIDs:     []*shared.NodeID{nodeIDs[1]},
			disconnectedIDs: nil,
			expectMatched:   false,
		},
	}

	for _, test := range tests {
		fmt.Println(test.description)
		res, err := client.ReconcileNextNodes(t.Context(), &connect.Request[proto.ReconcileNextNodesRequest]{
			Msg: &proto.ReconcileNextNodesRequest{
				NextNodeIds:         shared.ConvertNodeIDsToProto(test.nextNodeIDs),
				DisconnectedNodeIds: shared.ConvertNodeIDsToProto(test.disconnectedIDs),
			},
		})
		require.NoError(t, err)
		assert.Equal(t, test.expectMatched, res.Msg.Matched)
	}
}

func TestSeed_ReconcileNextNodes_twoNodes(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	slices.SortFunc(nodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})
	clients := make([]service.SeedServiceClient, len(nodeIDs))

	n := 0
	seed := NewController(WithAssignmentHandler(&AssignmentHandlerHelper{
		T: t,
		AssignNodeF: func(context.Context) (*shared.NodeID, error) {
			nodeID := nodeIDs[n]
			n++
			return nodeID, nil
		},
		UnassignNodeF: func(nodeID *shared.NodeID) {
			assert.Equal(t, nodeIDs[1], nodeID)
		},
	}))
	port := startServer(t, t.Context(), seed)

	for i := 0; i < len(nodeIDs); i++ {
		clients[i] = createClient(t, t.Context(), port)
		_, err := clients[i].AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{
			Msg: &proto.AssignNodeRequest{},
		})
		require.NoError(t, err)
	}

	tests := []struct {
		description     string
		srcNodeID       int
		nextNodeIDs     []*shared.NodeID
		disconnectedIDs []*shared.NodeID
		expectMatched   bool
	}{
		{
			description:     "all next nodes connected",
			srcNodeID:       0,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[1]},
			disconnectedIDs: nil,
			expectMatched:   true,
		},
		{
			description:     "node[0] reports disconnection of node[1]",
			srcNodeID:       0,
			nextNodeIDs:     []*shared.NodeID{},
			disconnectedIDs: []*shared.NodeID{nodeIDs[1]},
			expectMatched:   true,
		},
	}

	for _, test := range tests {
		fmt.Println(test.description)
		res, err := clients[test.srcNodeID].ReconcileNextNodes(t.Context(), &connect.Request[proto.ReconcileNextNodesRequest]{
			Msg: &proto.ReconcileNextNodesRequest{
				NextNodeIds:         shared.ConvertNodeIDsToProto(test.nextNodeIDs),
				DisconnectedNodeIds: shared.ConvertNodeIDsToProto(test.disconnectedIDs),
			},
		})
		require.NoError(t, err)
		assert.Equal(t, test.expectMatched, res.Msg.Matched)
	}
}

func TestSeed_ReconcileNextNodes_manyNodes(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(8)
	slices.SortFunc(nodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})
	clients := make([]service.SeedServiceClient, len(nodeIDs))

	n := 0
	seed := NewController(WithAssignmentHandler(&AssignmentHandlerHelper{
		T: t,
		AssignNodeF: func(context.Context) (*shared.NodeID, error) {
			nodeID := nodeIDs[n]
			n++
			return nodeID, nil
		},
		UnassignNodeF: func(nodeID *shared.NodeID) {
			assert.Equal(t, nodeIDs[4], nodeID)
		},
	}))
	port := startServer(t, t.Context(), seed)

	for i := 0; i < len(nodeIDs); i++ {
		clients[i] = createClient(t, t.Context(), port)
		_, err := clients[i].AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{
			Msg: &proto.AssignNodeRequest{},
		})
		require.NoError(t, err)
	}

	tests := []struct {
		description     string
		srcNodeID       int
		nextNodeIDs     []*shared.NodeID
		disconnectedIDs []*shared.NodeID
		expectMatched   bool
	}{
		{
			description:     "all next nodes connected",
			srcNodeID:       3,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[1], nodeIDs[2], nodeIDs[4], nodeIDs[5]},
			disconnectedIDs: nil,
			expectMatched:   true,
		},
		{
			description:     "node[2] reports disconnection of node[4]",
			srcNodeID:       2,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[3], nodeIDs[5]},
			disconnectedIDs: []*shared.NodeID{nodeIDs[4]},
			expectMatched:   false,
		},
		{
			description:     "node[2] reports disconnection of node[4] again without disconnected node",
			srcNodeID:       2,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[3], nodeIDs[5]},
			disconnectedIDs: nil,
			expectMatched:   false,
		},
		{
			description:     "node[3] reports disconnection of node[4]",
			srcNodeID:       3,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[1], nodeIDs[2], nodeIDs[5], nodeIDs[6]},
			disconnectedIDs: []*shared.NodeID{nodeIDs[4]},
			expectMatched:   true,
		},
		{
			description:     "node[2] reports disconnection of node[4] again and it should be matched",
			srcNodeID:       2,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[3], nodeIDs[5]},
			disconnectedIDs: nil,
			expectMatched:   true,
		},
		{
			description:     "node[4] become online",
			srcNodeID:       3,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[1], nodeIDs[2], nodeIDs[4], nodeIDs[5]},
			disconnectedIDs: nil,
			expectMatched:   true,
		},
		{
			description:     "node[2] reports disconnection of node[4] again and it should not be matched",
			srcNodeID:       2,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[3], nodeIDs[5]},
			disconnectedIDs: nil,
			expectMatched:   false,
		},
		{
			description:     "node[5] reports disconnection of node[4] again and it should be matched",
			srcNodeID:       5,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[2], nodeIDs[3], nodeIDs[6], nodeIDs[7]},
			disconnectedIDs: []*shared.NodeID{nodeIDs[4]},
			expectMatched:   true,
		},
		{
			description:     "node[2] reports disconnection of node[4] again and it should be matched",
			srcNodeID:       2,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[3], nodeIDs[5]},
			disconnectedIDs: nil,
			expectMatched:   true,
		},
		{
			description:     "node[3] reports disconnection of node[4] again and it should be matched",
			srcNodeID:       3,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[1], nodeIDs[2], nodeIDs[5], nodeIDs[6]},
			disconnectedIDs: nil,
			expectMatched:   true,
		},
		{
			description:     "have next node id across 0",
			srcNodeID:       7,
			nextNodeIDs:     []*shared.NodeID{nodeIDs[5], nodeIDs[6], nodeIDs[0], nodeIDs[2]},
			disconnectedIDs: []*shared.NodeID{nodeIDs[1]},
			expectMatched:   false,
		},
	}

	for _, test := range tests {
		fmt.Println(test.description)
		res, err := clients[test.srcNodeID].ReconcileNextNodes(t.Context(), &connect.Request[proto.ReconcileNextNodesRequest]{
			Msg: &proto.ReconcileNextNodesRequest{
				NextNodeIds:         shared.ConvertNodeIDsToProto(test.nextNodeIDs),
				DisconnectedNodeIds: shared.ConvertNodeIDsToProto(test.disconnectedIDs),
			},
		})
		require.NoError(t, err)
		assert.Equal(t, test.expectMatched, res.Msg.Matched)
	}
}

func TestSeed_ReconcileNextNodes_withMultiSeedHandler(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(5)
	slices.SortFunc(nodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})

	mtx := sync.Mutex{}
	var getNodeReportsFrom *shared.NodeID
	var getNodeReportsTo *shared.NodeID
	var getNodeReportsRes map[shared.NodeID]*NodeReport
	var reportDisconnectedTargets []*shared.NodeID
	var clearDisconnectedTargets []*shared.NodeID
	var nodeCount uint64

	seed := NewController(
		WithAssignmentHandler(&AssignmentHandlerHelper{
			T: t,
			AssignNodeF: func(ctx context.Context) (*shared.NodeID, error) {
				return nodeIDs[0], nil
			},
			UnassignNodeF: func(nodeID *shared.NodeID) {},
		}),
		WithMultiSeedHandler(&MultiSeedHandlerHelper{
			T: t,
			GetNodeReportsF: func(ctx context.Context, from, to *shared.NodeID) (map[shared.NodeID]*NodeReport, error) {
				mtx.Lock()
				defer mtx.Unlock()
				getNodeReportsFrom = from
				getNodeReportsTo = to
				return getNodeReportsRes, nil
			},
			ReportDisconnectedF: func(ctx context.Context, from *shared.NodeID, targets []*shared.NodeID) error {
				assert.Equal(t, from, nodeIDs[0])
				mtx.Lock()
				defer mtx.Unlock()
				reportDisconnectedTargets = targets
				return nil
			},
			ClearDisconnectedF: func(ctx context.Context, from *shared.NodeID, target []*shared.NodeID) error {
				assert.Equal(t, from, nodeIDs[0])
				mtx.Lock()
				defer mtx.Unlock()
				clearDisconnectedTargets = target
				return nil
			},
			GetNodeCountF: func(ctx context.Context) (uint64, error) {
				mtx.Lock()
				defer mtx.Unlock()
				return nodeCount, nil
			},
		}),
	)
	port := startServer(t, t.Context(), seed)

	// Assign node
	client := createClient(t, t.Context(), port)
	_, err := client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{
		Msg: &proto.AssignNodeRequest{},
	})
	require.NoError(t, err)

	tests := []struct {
		description                     string
		nextNodeIDs                     []*shared.NodeID
		disconnectedIDs                 []*shared.NodeID
		expectMatched                   bool
		expectNodeReportsFrom           *shared.NodeID
		expectNodeReportsTo             *shared.NodeID
		nodeReportsRes                  map[shared.NodeID]*NodeReport
		expectReportDisconnectedTargets []*shared.NodeID
		expectClearDisconnectedTargets  []*shared.NodeID
		nodeCount                       uint64
	}{
		{
			description:                     "one node, no next node, no disconnected",
			nextNodeIDs:                     []*shared.NodeID{},
			disconnectedIDs:                 nil,
			expectMatched:                   true,
			expectNodeReportsFrom:           nil,
			expectNodeReportsTo:             nil,
			nodeReportsRes:                  nil,
			expectReportDisconnectedTargets: nil,
			expectClearDisconnectedTargets:  nil,
			nodeCount:                       1,
		},
		{
			description:                     "one node, no next node, disconnected from not exist node",
			nextNodeIDs:                     []*shared.NodeID{},
			disconnectedIDs:                 []*shared.NodeID{nodeIDs[1]},
			expectMatched:                   true,
			expectNodeReportsFrom:           nil,
			expectNodeReportsTo:             nil,
			nodeReportsRes:                  nil,
			expectReportDisconnectedTargets: []*shared.NodeID{nodeIDs[1]},
			expectClearDisconnectedTargets:  nil,
			nodeCount:                       1,
		},
		{
			description:                     "one node, next node is not exist",
			nextNodeIDs:                     []*shared.NodeID{nodeIDs[1]},
			disconnectedIDs:                 nil,
			expectMatched:                   false,
			expectNodeReportsFrom:           nodeIDs[1],
			expectNodeReportsTo:             nodeIDs[1],
			nodeReportsRes:                  map[shared.NodeID]*NodeReport{},
			expectReportDisconnectedTargets: nil,
			expectClearDisconnectedTargets:  []*shared.NodeID{nodeIDs[1]},
			nodeCount:                       1,
		},
		{
			description:           "there are many nodes, disconnect some nodes",
			nextNodeIDs:           []*shared.NodeID{nodeIDs[4], nodeIDs[2], nodeIDs[3]},
			disconnectedIDs:       []*shared.NodeID{nodeIDs[1]},
			expectMatched:         false,
			expectNodeReportsFrom: nodeIDs[4],
			expectNodeReportsTo:   nodeIDs[3],
			nodeReportsRes: map[shared.NodeID]*NodeReport{
				*nodeIDs[1]: {Disconnected: map[shared.NodeID]time.Time{}},
				*nodeIDs[2]: {Disconnected: map[shared.NodeID]time.Time{}},
				*nodeIDs[3]: {Disconnected: map[shared.NodeID]time.Time{}},
				*nodeIDs[4]: {Disconnected: map[shared.NodeID]time.Time{}},
			},
			expectReportDisconnectedTargets: []*shared.NodeID{nodeIDs[1]},
			expectClearDisconnectedTargets:  []*shared.NodeID{nodeIDs[4], nodeIDs[2], nodeIDs[3]},
			nodeCount:                       5,
		},
	}

	for _, test := range tests {
		t.Log(test.description)
		mtx.Lock()
		getNodeReportsRes = test.nodeReportsRes
		nodeCount = test.nodeCount

		getNodeReportsFrom = nil
		getNodeReportsTo = nil
		reportDisconnectedTargets = nil
		clearDisconnectedTargets = nil
		mtx.Unlock()

		res, err := client.ReconcileNextNodes(t.Context(), &connect.Request[proto.ReconcileNextNodesRequest]{
			Msg: &proto.ReconcileNextNodesRequest{
				NextNodeIds:         shared.ConvertNodeIDsToProto(test.nextNodeIDs),
				DisconnectedNodeIds: shared.ConvertNodeIDsToProto(test.disconnectedIDs),
			},
		})
		require.NoError(t, err)
		assert.Equal(t, test.expectMatched, res.Msg.Matched)

		mtx.Lock()
		assert.Equal(t, test.expectNodeReportsFrom, getNodeReportsFrom)
		assert.Equal(t, test.expectNodeReportsTo, getNodeReportsTo)
		assert.Equal(t, test.expectReportDisconnectedTargets, reportDisconnectedTargets)
		assert.Equal(t, test.expectClearDisconnectedTargets, clearDisconnectedTargets)
		mtx.Unlock()
	}
}

func TestSeed_reportDisconnects_withMultiSeedHandler(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	opts := controllerOptions{
		logger: slog.Default(),
		seedHandler: &MultiSeedHandlerHelper{
			T: t,
			ReportDisconnectedF: func(ctx context.Context, target *shared.NodeID, from []*shared.NodeID) error {
				assert.True(t, target.Equal(nodeIDs[0]))
				assert.Len(t, from, 2)
				assert.True(t, from[0].Equal(nodeIDs[1]))
				assert.True(t, from[1].Equal(nodeIDs[2]))
				return nil
			},
		},
	}
	seed := &Controller{
		controllerOptions: opts,
	}
	err := seed.reportDisconnects(t.Context(), nodeIDs[0],
		[]*shared.NodeID{nodeIDs[1], nodeIDs[2]})
	require.NoError(t, err)
}

func TestSeed_reportDisconnects_withoutMultiSeedHandler(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(4)
	seed := &Controller{
		nodes: make(map[shared.NodeID]*node),
	}
	testSeed_setNode(seed, nodeIDs[0], 0, nil)
	testSeed_setNode(seed, nodeIDs[1], 0, nil)
	testSeed_setNode(seed, nodeIDs[2], 0, nil)

	baseTime := time.Now().Add(-1 * time.Second)
	// report for a node that is not exist
	seed.reportDisconnects(t.Context(), nodeIDs[0], []*shared.NodeID{nodeIDs[1], nodeIDs[3]})
	// report for a duplicate node
	seed.reportDisconnects(t.Context(), nodeIDs[0], []*shared.NodeID{nodeIDs[1], nodeIDs[2]})
	seed.reportDisconnects(t.Context(), nodeIDs[1], []*shared.NodeID{nodeIDs[2]})

	assert.Len(t, seed.nodes, 3)
	assert.Len(t, seed.nodes[*nodeIDs[0]].disconnected, 0)
	assert.Len(t, seed.nodes[*nodeIDs[1]].disconnected, 1)
	assert.Len(t, seed.nodes[*nodeIDs[2]].disconnected, 2)
	assert.True(t, seed.nodes[*nodeIDs[1]].disconnected[*nodeIDs[0]].After(baseTime))
	assert.True(t, seed.nodes[*nodeIDs[2]].disconnected[*nodeIDs[0]].After(baseTime))
	assert.True(t, seed.nodes[*nodeIDs[2]].disconnected[*nodeIDs[1]].After(baseTime))
}

func TestSeed_getNodeReports_withMultiSeedHandler(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	opts := controllerOptions{
		logger: slog.Default(),
		seedHandler: &MultiSeedHandlerHelper{
			T: t,
			GetNodeReportsF: func(ctx context.Context, from, to *shared.NodeID) (map[shared.NodeID]*NodeReport, error) {
				require.True(t, from.Equal(nodeIDs[0]))
				require.True(t, to.Equal(nodeIDs[2]))
				return map[shared.NodeID]*NodeReport{
					*nodeIDs[0]: {
						Disconnected: nil,
					},
					*nodeIDs[1]: {
						Disconnected: map[shared.NodeID]time.Time{
							*nodeIDs[0]: time.Now(),
							*nodeIDs[2]: time.Now(),
						},
					},
					*nodeIDs[2]: {
						Disconnected: map[shared.NodeID]time.Time{
							*nodeIDs[0]: time.Now(),
							*nodeIDs[1]: time.Now().Add(-1*REPORT_TIMEOUT - 10*time.Second),
						},
					},
				}, nil
			},
		},
	}
	seed := &Controller{
		controllerOptions: opts,
	}
	reports, err := seed.getNodeReports(t.Context(), nodeIDs[0], nodeIDs[2])
	require.NoError(t, err)
	require.Len(t, reports, 3)
	require.Len(t, reports[*nodeIDs[0]].Disconnected, 0)
	require.Len(t, reports[*nodeIDs[1]].Disconnected, 2)
	require.Len(t, reports[*nodeIDs[2]].Disconnected, 1)
}

func TestSeed_getNodeReports_withoutMultiSeedHandler(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(5)
	slices.SortFunc(nodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})

	seed := &Controller{
		nodes: make(map[shared.NodeID]*node),
	}
	testSeed_setNode(seed, nodeIDs[0], 0, nil)
	testSeed_setNode(seed, nodeIDs[1], 0, nil)
	testSeed_setNode(seed, nodeIDs[2], 0, map[shared.NodeID]time.Duration{
		*nodeIDs[0]: 0,
		*nodeIDs[1]: REPORT_TIMEOUT / 2,
	})
	testSeed_setNode(seed, nodeIDs[3], 0, map[shared.NodeID]time.Duration{
		*nodeIDs[0]: 0,
		*nodeIDs[1]: REPORT_TIMEOUT + 10*time.Second,
	})
	testSeed_setNode(seed, nodeIDs[4], 0, nil)

	reports, err := seed.getNodeReports(t.Context(), nodeIDs[1], nodeIDs[3])
	require.NoError(t, err)
	require.Len(t, reports, 3)
	require.Len(t, reports[*nodeIDs[1]].Disconnected, 0)
	require.Len(t, reports[*nodeIDs[2]].Disconnected, 2)
	require.Len(t, reports[*nodeIDs[3]].Disconnected, 1)
}

func TestSeed_cleanup(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(6)

	opts := controllerOptions{
		logger:          slog.Default(),
		pollingInterval: 10 * time.Minute,
	}
	seed := &Controller{
		controllerOptions: opts,
		nodes:             make(map[shared.NodeID]*node),
	}

	// the first node timeout
	testSeed_setNode(seed, nodeIDs[0], 15*time.Minute, nil)
	seed.cleanup()
	assert.Len(t, seed.nodes, 0)

	// disconnect by reports in small cluster
	testSeed_setNode(seed, nodeIDs[0], 5*time.Minute, map[shared.NodeID]time.Duration{
		*nodeIDs[2]: 10 * time.Second,
	})
	testSeed_setNode(seed, nodeIDs[1], 5*time.Minute, map[shared.NodeID]time.Duration{
		*nodeIDs[0]: 10 * time.Second,
		*nodeIDs[2]: 10 * time.Second,
	})
	testSeed_setNode(seed, nodeIDs[2], 5*time.Minute, map[shared.NodeID]time.Duration{
		*nodeIDs[0]: 10 * time.Second,
		*nodeIDs[1]: 10 * time.Second,
	})
	require.Len(t, seed.nodes, 3)
	seed.cleanup()
	assert.Len(t, seed.nodes, 1)
	assert.Contains(t, seed.nodes, *nodeIDs[0])

	// normal cluster
	testSeed_setNode(seed, nodeIDs[0], 5*time.Minute, map[shared.NodeID]time.Duration{
		*nodeIDs[2]: 10 * time.Second,
	})
	testSeed_setNode(seed, nodeIDs[1], 5*time.Minute, map[shared.NodeID]time.Duration{
		*nodeIDs[0]: 10 * time.Second,
		*nodeIDs[2]: 10 * time.Second,
	})
	testSeed_setNode(seed, nodeIDs[2], 5*time.Minute, map[shared.NodeID]time.Duration{
		*nodeIDs[0]: 10 * time.Second,
		*nodeIDs[1]: 10 * time.Second,
	})
	testSeed_setNode(seed, nodeIDs[3], 5*time.Minute, nil)
	testSeed_setNode(seed, nodeIDs[4], 15*time.Minute, nil)
	testSeed_setNode(seed, nodeIDs[5], 5*time.Minute, map[shared.NodeID]time.Duration{
		*nodeIDs[0]: 10 * time.Second,
		*nodeIDs[1]: 10 * time.Second,
		*nodeIDs[2]: 10 * time.Second,
	})
}

func TestNode_isExpired(t *testing.T) {
	tests := []struct {
		node            *node
		pollingInterval time.Duration
		reportThreshold int
		expect          bool
	}{
		{
			// not expired
			node: &node{
				timestamp: time.Now().Add(-5 * time.Second),
				disconnected: map[shared.NodeID]time.Time{
					*shared.NewRandomNodeID(): time.Now(),
					*shared.NewRandomNodeID(): time.Now(),
				},
			},
			pollingInterval: 10 * time.Second,
			reportThreshold: 3,
			expect:          false,
		},
		{
			// timeout
			node: &node{
				timestamp: time.Now().Add(-15 * time.Second),
				disconnected: map[shared.NodeID]time.Time{
					*shared.NewRandomNodeID(): time.Now(),
					*shared.NewRandomNodeID(): time.Now(),
				},
			},
			pollingInterval: 10 * time.Second,
			reportThreshold: 3,
			expect:          true,
		},
		{
			// reportThreshold is small
			node: &node{
				timestamp: time.Now().Add(-5 * time.Second),
				disconnected: map[shared.NodeID]time.Time{
					*shared.NewRandomNodeID(): time.Now(),
				},
			},
			pollingInterval: 10 * time.Second,
			reportThreshold: 1,
			expect:          true,
		},
		{
			// there are more reports than the threshold
			node: &node{
				timestamp: time.Now().Add(-5 * time.Second),
				disconnected: map[shared.NodeID]time.Time{
					*shared.NewRandomNodeID(): time.Now(),
					*shared.NewRandomNodeID(): time.Now(),
					*shared.NewRandomNodeID(): time.Now(),
					*shared.NewRandomNodeID(): time.Now(),
				},
			},
			pollingInterval: 10 * time.Second,
			reportThreshold: 3,
			expect:          true,
		},
		{
			// report expired
			node: &node{
				timestamp: time.Now().Add(-5 * time.Second),
				disconnected: map[shared.NodeID]time.Time{
					*shared.NewRandomNodeID(): time.Now(),
					*shared.NewRandomNodeID(): time.Now(),
					*shared.NewRandomNodeID(): time.Now().Add(-60 * time.Second),
					*shared.NewRandomNodeID(): time.Now().Add(-60 * time.Second),
				},
			},
			pollingInterval: 10 * time.Second,
			reportThreshold: 3,
			expect:          false,
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expect, test.node.isExpired(test.pollingInterval, test.reportThreshold))
	}
}

func Test_nodeIDBetween(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	slices.SortFunc(nodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})

	for i := 0; i < len(nodeIDs); i++ {
		assert.True(t, nodeIDBetween(nodeIDs[i], nodeIDs[(i+2)%3], nodeIDs[(i+1)%3]))
		assert.False(t, nodeIDBetween(nodeIDs[i], nodeIDs[(i+1)%3], nodeIDs[(i+2)%3]))
	}
}

func testSeed_setNode(seed *Controller, nodeID *shared.NodeID, age time.Duration, disconnectedSrc map[shared.NodeID]time.Duration) {
	disconnected := make(map[shared.NodeID]time.Time)
	for nodeID, passed := range disconnectedSrc {
		disconnected[nodeID] = time.Now().Add(-1 * passed)
	}
	seed.nodes[*nodeID] = &node{
		nodeID:       *nodeID,
		timestamp:    time.Now().Add(-1 * age),
		term:         make(chan struct{}),
		disconnected: disconnected,
	}
}
