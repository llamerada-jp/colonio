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
	"log/slog"
	"math/rand"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
)

func generateEmptySeed() *Seed {
	return &Seed{
		logger:         slog.Default(),
		mutex:          sync.Mutex{},
		nodes:          make(map[shared.NodeID]*node),
		sessions:       make(map[string]*shared.NodeID),
		sessionTimeout: 30 * time.Second,
		pollingTimeout: 10 * time.Second,
	}
}

type dummyAuthenticator struct {
	validToken []byte
	lastNodeID string
}

func (auth *dummyAuthenticator) Authenticate(token []byte, nodeID string) (bool, error) {
	auth.lastNodeID = nodeID
	if slices.Equal(auth.validToken, []byte("error")) {
		return false, fmt.Errorf("invalid")
	}

	if slices.Equal(auth.validToken, token) {
		return true, nil
	}
	return false, nil
}

func TestAuthenticate(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seed := generateEmptySeed()
	seed.configJS = []byte("dummy config")
	go func() {
		seed.Run(ctx)
	}()
	seed.WaitForRun()

	nodeIDs := testUtil.UniqueNodeIDs(2)

	// normal
	res, code, err := seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  nodeIDs[0].Proto(),
		Token:   nil,
	})
	sessionID1 := res.SessionId
	assert.Equal([]byte("dummy config"), res.Config)
	assert.Equal(shared.HintOnlyOne, res.Hint)
	assert.NotNil(res.SessionId)
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Len(seed.nodes, 1)
	assert.Len(seed.sessions, 1)
	assert.Equal(*nodeIDs[0], *seed.sessions[sessionID1])

	// normal2
	res, code, err = seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  nodeIDs[1].Proto(),
		Token:   nil,
	})
	sessionID2 := res.SessionId
	assert.Equal(uint32(0), res.Hint)
	assert.NotNil(res.SessionId)
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Len(seed.nodes, 2)
	assert.Len(seed.sessions, 2)
	assert.Equal(*nodeIDs[1], *seed.sessions[sessionID2])

	assert.NotEqual(nodeIDs[0], nodeIDs[1])
	assert.NotEqual(sessionID1, sessionID2)

	// duplicate connection
	res, code, err = seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  nodeIDs[0].Proto(),
		Token:   nil,
	})
	assert.Nil(res)
	assert.Equal(http.StatusInternalServerError, code)
	assert.Error(err)
	assert.Len(seed.nodes, 1)
	assert.Len(seed.sessions, 1)
	assert.Equal(*nodeIDs[1], *seed.sessions[sessionID2])

	// invalid version
	res, code, err = seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: "dummy",
	})
	assert.Nil(res)
	assert.Equal(http.StatusBadRequest, code)
	assert.Error(err)

	// verification success
	authenticator := &dummyAuthenticator{
		validToken: []byte("token!"),
	}
	seed.authenticator = authenticator
	nodeID := shared.NewRandomNodeID()
	res, code, err = seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  nodeID.Proto(),
		Token:   []byte("token!"),
	})
	assert.NotNil(res)
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Equal(nodeID.String(), authenticator.lastNodeID)
	assert.Len(seed.nodes, 2)
	assert.Len(seed.sessions, 2)

	// verification failed
	res, code, err = seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  shared.NewRandomNodeID().Proto(),
		Token:   []byte("token?"),
	})
	assert.Nil(res)
	assert.Equal(http.StatusForbidden, code)
	assert.Error(err)
	assert.Len(seed.nodes, 2)
	assert.Len(seed.sessions, 2)

	// verification error
	authenticator = &dummyAuthenticator{
		validToken: []byte("error"),
	}
	seed.authenticator = authenticator
	nodeID = shared.NewRandomNodeID()
	res, code, err = seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  nodeID.Proto(),
		Token:   nil,
	})
	assert.Nil(res)
	assert.Equal(http.StatusInternalServerError, code)
	assert.Error(err)
	assert.Equal(nodeID.String(), authenticator.lastNodeID)
	assert.Len(seed.nodes, 2)
	assert.Len(seed.sessions, 2)
}

func TestClose(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seed := generateEmptySeed()
	go func() {
		seed.Run(ctx)
	}()
	seed.WaitForRun()

	// normal
	resAuth, code, err := seed.authenticate(ctx, &proto.SeedAuthenticate{
		Version: shared.ProtocolVersion,
		NodeId:  shared.NewRandomNodeID().Proto(),
		Token:   nil,
	})
	sessionID1 := resAuth.SessionId
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Len(seed.nodes, 1)
	assert.Len(seed.sessions, 1)

	// close dummy session
	resClose, code, err := seed.close(ctx, &proto.SeedClose{
		SessionId: "dummy",
	})
	assert.Nil(resClose)
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Len(seed.nodes, 1)
	assert.Len(seed.sessions, 1)

	// close existing session
	resClose, code, err = seed.close(ctx, &proto.SeedClose{
		SessionId: sessionID1,
	})
	assert.Nil(resClose)
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Len(seed.nodes, 0)
	assert.Len(seed.sessions, 0)
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
