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
package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/sessions"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	service "github.com/llamerada-jp/colonio/api/colonio/v1alpha/v1alphaconnect"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/seed/controller"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type controllerMock struct {
	t                   *testing.T
	assignNodeF         func(ctx context.Context) (*shared.NodeID, bool, error)
	unassignNodeF       func(ctx context.Context, nodeID *shared.NodeID) error
	keepaliveF          func(ctx context.Context, nodeID *shared.NodeID) (bool, error)
	reconcileNextNodesF func(ctx context.Context, nodeID *shared.NodeID, nextNodeIDs, disconnectedIDs []*shared.NodeID) (bool, error)
	sendSignalF         func(ctx context.Context, nodeID *shared.NodeID, signal *proto.Signal) error
	pollSignalF         func(ctx context.Context, nodeID *shared.NodeID, send func(*proto.Signal) error) error
	stateKvsF           func(ctx context.Context, nodeID *shared.NodeID, active bool) (bool, error)
}

var _ controller.Controller = &controllerMock{}

func (c *controllerMock) AssignNode(ctx context.Context) (*shared.NodeID, bool, error) {
	c.t.Helper()
	require.NotNil(c.t, c.assignNodeF, "AssignNodeF must not be nil")
	return c.assignNodeF(ctx)
}

func (c *controllerMock) UnassignNode(ctx context.Context, nodeID *shared.NodeID) error {
	c.t.Helper()
	require.NotNil(c.t, c.unassignNodeF, "UnassignNodeF must not be nil")
	return c.unassignNodeF(ctx, nodeID)
}

func (c *controllerMock) Keepalive(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
	c.t.Helper()
	require.NotNil(c.t, c.keepaliveF, "KeepaliveF must not be nil")
	return c.keepaliveF(ctx, nodeID)
}

func (c *controllerMock) ReconcileNextNodes(ctx context.Context, nodeID *shared.NodeID, nextNodeIDs, disconnectedIDs []*shared.NodeID) (bool, error) {
	c.t.Helper()
	require.NotNil(c.t, c.reconcileNextNodesF, "ReconcileNextNodesF must not be nil")
	return c.reconcileNextNodesF(ctx, nodeID, nextNodeIDs, disconnectedIDs)
}

func (c *controllerMock) SendSignal(ctx context.Context, nodeID *shared.NodeID, signal *proto.Signal) error {
	c.t.Helper()
	require.NotNil(c.t, c.sendSignalF, "SendSignalF must not be nil")
	return c.sendSignalF(ctx, nodeID, signal)
}

func (c *controllerMock) PollSignal(ctx context.Context, nodeID *shared.NodeID, send func(*proto.Signal) error) error {
	c.t.Helper()
	require.NotNil(c.t, c.pollSignalF, "PollSignalF must not be nil")
	return c.pollSignalF(ctx, nodeID, send)
}

func (c *controllerMock) StateKvs(ctx context.Context, nodeID *shared.NodeID, active bool) (bool, error) {
	c.t.Helper()
	require.NotNil(c.t, c.stateKvsF, "StateKvsF must not be nil")
	return c.stateKvsF(ctx, nodeID, active)
}

func runTestServer(t *testing.T, controller *controllerMock) uint16 {
	t.Helper()

	// Create a new server instance with the provided controller mock
	srv := NewServer(Options{
		Logger:       testUtil.Logger(t),
		SessionStore: sessions.NewCookieStore([]byte("test-secret-key")),
		Controller:   controller,
	})

	// Crate a new HTTP server with the server's mux
	mux := http.NewServeMux()
	srv.RegisterService(mux)

	port := 8000 + uint16(rand.Uint32()%1000)
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Start the HTTP server in a goroutine
	cert := os.Getenv("COLONIO_TEST_CERT")
	key := os.Getenv("COLONIO_TEST_KEY")
	if cert == "" || key == "" {
		panic("Please set COLONIO_TEST_CERT and COLONIO_TEST_KEY")
	}

	go func() {
		err := httpSrv.ListenAndServeTLS(cert, key)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("HTTP server start error: %v", err)
		}
	}()

	// Shutdown the server when the test is done
	go func() {
		<-t.Context().Done()
		httpSrv.Shutdown(context.Background())
	}()

	// wait for the server to start
	require.Eventually(t, func() bool {
		client := testUtil.NewInsecureHttpClient()
		_, err := client.Get(fmt.Sprintf("https://localhost:%d", port))
		return err == nil
	}, 5*time.Second, 100*time.Millisecond)

	return port
}

func createTestClient(t *testing.T, port uint16) service.SeedServiceClient {
	t.Helper()
	return service.NewSeedServiceClient(
		testUtil.NewInsecureHttpClient(),
		fmt.Sprintf("https://localhost:%d", port),
	)
}

func TestServer_AssignNode(t *testing.T) {
	nodeID := shared.NewRandomNodeID()
	port := runTestServer(t, &controllerMock{
		t: t,
		assignNodeF: func(ctx context.Context) (*shared.NodeID, bool, error) {
			return nodeID, true, nil
		},
	})
	client := createTestClient(t, port)

	res, err := client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{})
	require.NoError(t, err)
	resNodeID, err := shared.NewNodeIDFromProto(res.Msg.GetNodeId())
	require.NoError(t, err)
	require.Equal(t, nodeID, resNodeID)
	resIsAlone := res.Msg.GetIsAlone()
	require.True(t, resIsAlone)
}

func TestServer_UnassignNode(t *testing.T) {
	nodeID := shared.NewRandomNodeID()
	called := false
	port := runTestServer(t, &controllerMock{
		t: t,
		assignNodeF: func(ctx context.Context) (*shared.NodeID, bool, error) {
			return nodeID, true, nil
		},
		unassignNodeF: func(ctx context.Context, nodeID *shared.NodeID) error {
			called = true
			require.Equal(t, nodeID, nodeID)
			return nil
		},
	})
	client := createTestClient(t, port)

	// Unassign without assigning first should not call unassign
	_, err := client.UnassignNode(t.Context(), &connect.Request[proto.UnassignNodeRequest]{})
	require.NoError(t, err)
	assert.False(t, called)

	// Unassign after assigning should call unassign
	_, err = client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{})
	require.NoError(t, err)
	_, err = client.UnassignNode(t.Context(), &connect.Request[proto.UnassignNodeRequest]{})
	require.NoError(t, err)
	assert.True(t, called)

	// Unassign again should not call unassign
	called = false
	_, err = client.UnassignNode(t.Context(), &connect.Request[proto.UnassignNodeRequest]{})
	require.NoError(t, err)
	assert.False(t, called)
}

func TestServer_Keepalive(t *testing.T) {
	nodeID := shared.NewRandomNodeID()
	called := false
	port := runTestServer(t, &controllerMock{
		t: t,
		assignNodeF: func(ctx context.Context) (*shared.NodeID, bool, error) {
			return nodeID, true, nil
		},
		keepaliveF: func(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
			called = true
			assert.Equal(t, nodeID, nodeID)
			return true, nil
		},
	})
	client := createTestClient(t, port)

	// Keepalive without assigning first should return an error
	_, err := client.Keepalive(t.Context(), &connect.Request[proto.KeepaliveRequest]{})
	ce := &connect.Error{}
	assert.ErrorAs(t, err, &ce)
	assert.ErrorContains(t, err, "reqID")

	// Assign a node and then keepalive
	_, err = client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{})
	require.NoError(t, err)
	res, err := client.Keepalive(t.Context(), &connect.Request[proto.KeepaliveRequest]{})
	require.NoError(t, err)
	assert.True(t, res.Msg.IsAlone)
	assert.True(t, called)
}

func TestServer_ReconcileNextNodes(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(5)
	port := runTestServer(t, &controllerMock{
		t: t,
		assignNodeF: func(ctx context.Context) (*shared.NodeID, bool, error) {
			return nodeIDs[0], true, nil
		},
		reconcileNextNodesF: func(ctx context.Context, nodeID *shared.NodeID, nextNodeIDs, disconnectedIDs []*shared.NodeID) (bool, error) {
			assert.Equal(t, nodeID, nodeIDs[0])
			assert.Equal(t, nextNodeIDs, []*shared.NodeID{nodeIDs[1], nodeIDs[2]})
			assert.Equal(t, disconnectedIDs, []*shared.NodeID{nodeIDs[3], nodeIDs[4]})
			return true, nil
		},
	})
	client := createTestClient(t, port)

	request := &proto.ReconcileNextNodesRequest{
		NextNodeIds:         shared.ConvertNodeIDsToProto([]*shared.NodeID{nodeIDs[1], nodeIDs[2]}),
		DisconnectedNodeIds: shared.ConvertNodeIDsToProto([]*shared.NodeID{nodeIDs[3], nodeIDs[4]}),
	}

	// Reconcile next nodes without assigning first should return an error
	_, err := client.ReconcileNextNodes(t.Context(), &connect.Request[proto.ReconcileNextNodesRequest]{
		Msg: request,
	})
	ce := &connect.Error{}
	assert.ErrorAs(t, err, &ce)
	assert.ErrorContains(t, err, "reqID")

	// Assign a node and then reconcile next nodes
	_, err = client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{
		Msg: &proto.AssignNodeRequest{},
	})
	require.NoError(t, err)
	res, err := client.ReconcileNextNodes(t.Context(), &connect.Request[proto.ReconcileNextNodesRequest]{
		Msg: request,
	})
	require.NoError(t, err)
	assert.True(t, res.Msg.Matched)
}

func TestServer_SendSignal(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	called := false
	port := runTestServer(t, &controllerMock{
		t: t,
		assignNodeF: func(ctx context.Context) (*shared.NodeID, bool, error) {
			return nodeIDs[0], true, nil
		},
		sendSignalF: func(ctx context.Context, nodeID *shared.NodeID, signal *proto.Signal) error {
			called = true
			assert.Equal(t, nodeID, nodeIDs[0])
			assert.Equal(t, signal.DstNodeId, nodeIDs[1].Proto())
			assert.Equal(t, signal.SrcNodeId, nodeIDs[2].Proto())
			assert.Equal(t, signal.GetIce().OfferId, uint32(3))
			return nil
		},
	})
	client := createTestClient(t, port)

	signal := &proto.Signal{
		DstNodeId: nodeIDs[1].Proto(),
		SrcNodeId: nodeIDs[2].Proto(),
		Content: &proto.Signal_Ice{
			Ice: &proto.SignalICE{
				OfferId: 3,
			},
		},
	}

	// Send signal without assigning first should return an error
	_, err := client.SendSignal(t.Context(), &connect.Request[proto.SendSignalRequest]{
		Msg: &proto.SendSignalRequest{Signal: signal},
	})
	ce := &connect.Error{}
	assert.ErrorAs(t, err, &ce)
	assert.ErrorContains(t, err, "reqID")

	// Assign a node and then send signal
	_, err = client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{})
	require.NoError(t, err)
	_, err = client.SendSignal(t.Context(), &connect.Request[proto.SendSignalRequest]{
		Msg: &proto.SendSignalRequest{Signal: signal},
	})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestServer_PollSignal(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	port := runTestServer(t, &controllerMock{
		t: t,
		assignNodeF: func(ctx context.Context) (*shared.NodeID, bool, error) {
			return nodeIDs[0], true, nil
		},
		pollSignalF: func(ctx context.Context, nodeID *shared.NodeID, send func(*proto.Signal) error) error {
			assert.Equal(t, nodeID, nodeIDs[0])
			// Simulate sending signals
			signals := []*proto.Signal{
				{DstNodeId: nodeIDs[1].Proto(), SrcNodeId: nodeIDs[2].Proto(), Content: &proto.Signal_Ice{Ice: &proto.SignalICE{OfferId: 1}}},
				{DstNodeId: nodeIDs[2].Proto(), SrcNodeId: nodeIDs[1].Proto(), Content: &proto.Signal_Ice{Ice: &proto.SignalICE{OfferId: 2}}},
			}
			for _, signal := range signals {
				if err := send(signal); err != nil {
					return err
				}
			}
			return nil
		},
	})
	client := createTestClient(t, port)

	// Poll signal without assigning first should return an error
	stream, err := client.PollSignal(t.Context(), &connect.Request[proto.PollSignalRequest]{})
	require.NoError(t, err) // The call itself should succeed

	// The error should be available in the stream
	for stream.Receive() {
		// Should not receive any messages for this error case
	}
	err = stream.Err()
	ce := &connect.Error{}
	assert.ErrorAs(t, err, &ce)
	assert.ErrorContains(t, err, "reqID")

	// Assign a node and then poll signal
	_, err = client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{
		Msg: &proto.AssignNodeRequest{},
	})
	require.NoError(t, err)
	stream, err = client.PollSignal(t.Context(), &connect.Request[proto.PollSignalRequest]{})
	require.NoError(t, err)
	// Receive the initial empty response
	res := stream.Receive()
	assert.True(t, res)
	msg := stream.Msg()
	assert.NotNil(t, msg)
	assert.Empty(t, msg.Signals)
	// Now receive the signals sent by the server
	signals := []*proto.Signal{}
	for stream.Receive() {
		msg = stream.Msg()
		assert.NotNil(t, msg)
		signals = append(signals, msg.Signals...)
	}
	assert.Len(t, signals, 2)
	assert.Equal(t, nodeIDs[1].Proto(), signals[0].DstNodeId)
	assert.Equal(t, nodeIDs[2].Proto(), signals[0].SrcNodeId)
	assert.Equal(t, uint32(1), signals[0].GetIce().OfferId)
	assert.Equal(t, nodeIDs[2].Proto(), signals[1].DstNodeId)
	assert.Equal(t, nodeIDs[1].Proto(), signals[1].SrcNodeId)
	assert.Equal(t, uint32(2), signals[1].GetIce().OfferId)
}

func TestServer_StateKvs(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(1)
	called := false
	port := runTestServer(t, &controllerMock{
		t: t,
		assignNodeF: func(ctx context.Context) (*shared.NodeID, bool, error) {
			return nodeIDs[0], true, nil
		},
		stateKvsF: func(ctx context.Context, nodeID *shared.NodeID, active bool) (bool, error) {
			called = true
			assert.Equal(t, nodeID, nodeIDs[0])
			assert.True(t, active)
			return true, nil
		},
	})
	client := createTestClient(t, port)

	// StateKvs without assigning first should return an error
	_, err := client.StateKvs(t.Context(), &connect.Request[proto.StateKvsRequest]{
		Msg: &proto.StateKvsRequest{
			Active: true,
		},
	})
	ce := &connect.Error{}
	assert.ErrorAs(t, err, &ce)
	assert.ErrorContains(t, err, "reqID")

	// Assign a node and then StateKvs
	_, err = client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{
		Msg: &proto.AssignNodeRequest{},
	})
	require.NoError(t, err)
	_, err = client.StateKvs(t.Context(), &connect.Request[proto.StateKvsRequest]{
		Msg: &proto.StateKvsRequest{
			Active: true,
		},
	})
	require.NoError(t, err)
	assert.True(t, called)

	assert.True(t, called)
}
