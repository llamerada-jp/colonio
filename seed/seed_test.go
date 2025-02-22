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
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/proto"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func runGRPCServer(ctx context.Context, seed *Seed) uint16 {
	port := 8000 + uint16(rand.Uint32()%1000)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}

	sv := grpc.NewServer()
	seed.RegisterGRPCServer(sv)

	go func() {
		seed.Run(ctx)
	}()

	go func() {
		if err := sv.Serve(listener); err != nil {
			panic(err)
		}
	}()

	go func() {
		<-ctx.Done()
		sv.GracefulStop()
	}()

	return port
}

func makeGRPCClient(ctx context.Context, port uint16) proto.SeedClient {
	conn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	return proto.NewSeedClient(conn)
}

type connectionHandlerHelper struct {
	assignNodeID func(context.Context) (*shared.NodeID, error)
	unassign     func(*shared.NodeID)
}

func (h *connectionHandlerHelper) AssignNodeID(ctx context.Context) (*shared.NodeID, error) {
	if h.assignNodeID != nil {
		return h.assignNodeID(ctx)
	}
	return nil, nil
}

func (h *connectionHandlerHelper) Unassign(nodeID *shared.NodeID) {
	if h.unassign != nil {
		h.unassign(nodeID)
	}
}

func TestAssignNodeID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seed := NewSeed(ctx)
	port := runGRPCServer(ctx, seed)

	// the first node
	client1 := makeGRPCClient(ctx, port)
	var md1 metadata.MD
	res, err := client1.AssignNodeID(ctx, &proto.AssignNodeIDRequest{}, grpc.Header(&md1))
	require.NoError(t, err)
	// check response
	assert.True(t, res.IsAlone)
	assert.NotNil(t, res.NodeId)
	nodeID1 := shared.NewNodeIDFromProto(res.NodeId)
	assert.NotNil(t, nodeID1)
	assert.True(t, nodeID1.IsNormal())
	// check session
	assert.Len(t, md1.Get(METADATA_SESSION_NAME), 1)
	session1 := md1.Get(METADATA_SESSION_NAME)[0]
	// check seed
	assert.Len(t, seed.nodes, 1)

	// the second node
	client2 := makeGRPCClient(ctx, port)
	var md2 metadata.MD
	res, err = client2.AssignNodeID(ctx, &proto.AssignNodeIDRequest{}, grpc.Header(&md2))
	require.NoError(t, err)
	// check response
	assert.False(t, res.IsAlone)
	nodeID2 := shared.NewNodeIDFromProto(res.NodeId)
	assert.NotNil(t, nodeID2)
	assert.True(t, nodeID2.IsNormal())
	// get session
	assert.Len(t, md2.Get(METADATA_SESSION_NAME), 1)
	session2 := md2.Get(METADATA_SESSION_NAME)[0]
	// check seed
	assert.Len(t, seed.nodes, 2)

	assert.NotEqual(t, nodeID1, nodeID2)
	assert.NotEqual(t, session1, session2)
}

func TestSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seed := NewSeed(ctx, WithPollingInterval(1*time.Second))
	port := runGRPCServer(ctx, seed)
	client := makeGRPCClient(ctx, port)
	var md metadata.MD
	_, err := client.AssignNodeID(ctx, &proto.AssignNodeIDRequest{}, grpc.Header(&md))
	require.NoError(t, err)

	// will success with valid session
	ctxWithMD := metadata.NewIncomingContext(ctx, md)
	_, err = client.Relay(ctxWithMD, &proto.RelayRequest{
		Packets: []*proto.SeedPacket{},
	})
	assert.NoError(t, err)

	// deny if the session is invalid
	_, err = client.Relay(ctx, &proto.RelayRequest{
		Packets: []*proto.SeedPacket{},
	})
	assert.Error(t, err)

	// deny if the session is outdated
	time.Sleep(12 * time.Second)
	_, err = client.Relay(ctxWithMD, &proto.RelayRequest{
		Packets: []*proto.SeedPacket{},
	})
	assert.Error(t, err)
}

func TestWithConnectionHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeID := shared.NewRandomNodeID()
	var mtx sync.Mutex
	unassignCalled := false

	seed := NewSeed(ctx,
		WithPollingInterval(1*time.Second),
		WithConnectionHandler(
			&connectionHandlerHelper{
				assignNodeID: func(context.Context) (*shared.NodeID, error) {
					return nodeID, nil
				},
				unassign: func(n *shared.NodeID) {
					assert.Equal(t, nodeID, n)

					mtx.Lock()
					defer mtx.Unlock()
					unassignCalled = true
				},
			}),
	)
	port := runGRPCServer(ctx, seed)
	client := makeGRPCClient(ctx, port)
	res, err := client.AssignNodeID(ctx, &proto.AssignNodeIDRequest{})
	require.NoError(t, err)
	resNodeID := shared.NewNodeIDFromProto(res.NodeId)
	assert.Equal(t, nodeID, resNodeID)

	// unassign should be called when the connection is closed
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return unassignCalled
	}, 15*time.Second, 1*time.Second)
}

func TestRelayPoll(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seed := generateEmptySeed()
	go func() {
		seed.Run(ctx)
	}()
	seed.WaitForRun()

	// connection
	srcNodeID := shared.NewRandomNodeID()
	resAuth, code, err := seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  srcNodeID.Proto(),
		Token:   nil,
	})
	sessionID1 := resAuth.SessionId
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)

	resAuth, code, err = seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  shared.NewRandomNodeID().Proto(),
		Token:   nil,
	})
	sessionID2 := resAuth.SessionId
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)

	// normal poll
	chPoll := make(chan *proto.SeedPollResponse, 1)
	go func() {
		resPoll, code, err := seed.poll(ctx, &proto.SeedPoll{
			SessionId: sessionID2,
			Online:    true,
		})
		assert.Equal(http.StatusOK, code)
		assert.NoError(err)
		chPoll <- resPoll
	}()

	// normal relay
	dstNodeID := shared.NewRandomNodeID()
	packetID := rand.Uint32()
	resRelay, code, err := seed.relay(ctx, &proto.SeedRelay{
		SessionId: sessionID1,
		Packets: []*proto.SeedPacket{
			{
				DstNodeId: dstNodeID.Proto(),
				SrcNodeId: srcNodeID.Proto(),
				Id:        packetID,
				Mode:      uint32(shared.PacketModeOneWay),
			},
		},
	})
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Equal(uint32(0), resRelay.Hint)

	resPoll := <-chPoll
	assert.Equal(uint32(0), resPoll.Hint)
	assert.Equal(sessionID2, resPoll.SessionId) // temporary
	assert.Len(resPoll.Packets, 1)
	assert.Equal(packetID, resPoll.Packets[0].Id)

	// get empty result if the sub process finished
	ctxPoll, cancelPoll := context.WithCancel(ctx)
	go func() {
		resPoll, code, err := seed.poll(ctxPoll, &proto.SeedPoll{
			SessionId: sessionID2,
			Online:    true,
		})
		assert.NotNil(resPoll)
		assert.Equal(http.StatusOK, code)
		assert.NoError(err)
		chPoll <- resPoll
	}()

	cancelPoll()
	<-chPoll

	// get empty result if the context done
	go func() {
		resPoll, code, err = seed.poll(context.Background(), &proto.SeedPoll{
			SessionId: sessionID2,
			Online:    true,
		})
		assert.NotNil(resPoll)
		assert.Equal(http.StatusOK, code)
		assert.NoError(err)
		chPoll <- resPoll
	}()

	cancel()
	<-chPoll
}

func TestGetPacket(t *testing.T) {
	assert := assert.New(t)

	seed := generateEmptySeed()
	nodeIDs := testUtil.UniqueNodeIDs(7)

	packets := []packet{
		// general packet
		{
			stepNodeID: nodeIDs[1],
			p: &proto.SeedPacket{
				Id:        0,
				DstNodeId: nodeIDs[2].Proto(),
				SrcNodeId: nodeIDs[3].Proto(),
				Mode:      0,
			},
		},
		// response packet
		{
			stepNodeID: nodeIDs[4],
			p: &proto.SeedPacket{
				Id:        1,
				DstNodeId: nodeIDs[5].Proto(),
				SrcNodeId: nodeIDs[6].Proto(),
				Mode:      uint32(shared.PacketModeResponse),
			},
		},
		// random request packet
		{
			stepNodeID: nodeIDs[1],
			p: &proto.SeedPacket{
				Id:        2,
				DstNodeId: nodeIDs[1].Proto(),
				SrcNodeId: nodeIDs[1].Proto(),
				Mode:      0,
			},
		},
	}

	testCases := []struct {
		title       string
		nodeID      *shared.NodeID
		online      bool
		anotherNode bool
		expect      []uint32
	}{
		{
			title:       "can get general packet by online normal node",
			nodeID:      nodeIDs[0],
			online:      true,
			anotherNode: true,
			expect:      []uint32{0, 2},
		},
		{
			title:       "can't get any packet if the packet posted by it self",
			nodeID:      nodeIDs[1],
			online:      true,
			anotherNode: true,
			expect:      []uint32{},
		},
		{
			title:       "can't get any packet if offline",
			nodeID:      nodeIDs[0],
			online:      false,
			anotherNode: true,
			expect:      []uint32{},
		},
		{
			title:       "can get general packet when all node are offline",
			nodeID:      nodeIDs[0],
			online:      false,
			anotherNode: false,
			expect:      []uint32{0, 2},
		},
		{
			title:       "can get general packet when destination is me with offline",
			nodeID:      nodeIDs[2],
			online:      false,
			anotherNode: true,
			expect:      []uint32{0},
		},
		{
			title:       "can get response packet if destination is me",
			nodeID:      nodeIDs[5],
			online:      false,
			anotherNode: true,
			expect:      []uint32{1},
		},
		{
			title:       "can get more than two packets",
			nodeID:      nodeIDs[5],
			online:      true,
			anotherNode: true,
			expect:      []uint32{0, 1, 2},
		},
	}

	dummyNodeID := shared.NewRandomNodeID()

	for _, testCase := range testCases {
		seed.packets = make([]packet, len(packets))
		copy(seed.packets, packets)

		if testCase.anotherNode {
			seed.nodes = map[shared.NodeID]*node{
				*dummyNodeID: {
					timestamp:   time.Now(),
					sessionID:   "dummy",
					online:      true,
					chTimestamp: time.Now(),
					c:           make(chan uint32),
				},
			}
		}

		res := seed.getPackets(testCase.nodeID, testCase.online, false)
		assert.Len(res, len(testCase.expect), testCase.title)
		for _, r := range res {
			assert.Contains(testCase.expect, r.Id, testCase.title)
		}
		assert.Len(seed.packets, len(packets)-len(testCase.expect), testCase.title)
		for _, p := range seed.packets {
			assert.NotContains(testCase.expect, p.p.Id, testCase.title)
		}

		if testCase.anotherNode {
			close(seed.nodes[*dummyNodeID].c)
			delete(seed.nodes, *dummyNodeID)
		}
	}
}

func TestRandomConnect(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seed := generateEmptySeed()
	go func() {
		seed.Run(ctx)
	}()
	seed.WaitForRun()

	nodeIDs := testUtil.UniqueNodeIDs(2)

	// not request random-connect when only one node
	resAuth, _, err := seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  nodeIDs[0].Proto(),
	})
	assert.NoError(err)
	ch1 := make(chan *proto.SeedPollResponse)
	go func(sessionID string) {
		resPoll, _, err := seed.poll(ctx, &proto.SeedPoll{
			SessionId: sessionID,
			Online:    true,
		})
		assert.NoError(err)
		ch1 <- resPoll
	}(resAuth.SessionId)

	// waiting for starting to poll
	assert.Eventually(func() bool {
		return seed.countOnline(false) == 1 &&
			seed.countChannels(false) == 1
	}, time.Second, time.Millisecond)

	seed.randomConnect()
	// polling ch exists
	assert.Equal(1, seed.countChannels(false))
	assert.Equal(1, seed.countOnline(false))

	timer := time.NewTimer(1 * time.Second)
	select {
	case <-timer.C:
	case <-ch1:
		assert.Fail("")
	}

	// generate random-connect request when more then two nodes
	resAuth, _, err = seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  nodeIDs[1].Proto(),
	})
	assert.NoError(err)
	ch2 := make(chan *proto.SeedPollResponse)
	go func(sessionID string) {
		resPoll, _, err := seed.poll(ctx, &proto.SeedPoll{
			SessionId: sessionID,
			Online:    true,
		})
		assert.NoError(err)
		ch2 <- resPoll
	}(resAuth.SessionId)

	// waiting for starting to poll
	assert.Eventually(func() bool {
		return seed.countOnline(false) == 2 &&
			seed.countChannels(false) == 2
	}, time.Second, 100*time.Millisecond)

	seed.randomConnect()

	// a channel will be close after call wakeUp method by randomConnect
	assert.Eventually(func() bool {
		return seed.countChannels(false) == 1
	}, time.Second, 100*time.Millisecond)

	var resPoll *proto.SeedPollResponse
	select {
	case resPoll = <-ch1:
	case resPoll = <-ch2:
	}
	assert.Equal(shared.HintRequireRandom, resPoll.Hint)
	assert.Len(resPoll.Packets, 0)
}
