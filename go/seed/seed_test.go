/*
 * Copyright 2019-2021 Yuji Ito <llamerada.jp@gmail.com>
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
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/go/proto"
	"github.com/stretchr/testify/assert"
)

func generateEmptySeed(ctx context.Context) *seed {
	return &seed{
		ctx:              ctx,
		mutex:            sync.Mutex{},
		nodes:            make(map[string]*node),
		sessions:         make(map[string]string),
		keepAliveTimeout: 30 * time.Second,
		pollingTimeout:   10 * time.Second,
	}
}

// make unique nids
func uniqueNids(count int) []*proto.NodeID {
	nids := make([]*proto.NodeID, count)
	exists := make(map[string]bool)

	for i := range nids {
		for {
			nid := randomNid()
			_, ok := exists[nidToString(nid)]
			if !ok {
				nids[i] = nid
				exists[nidToString(nid)] = true
				break
			}
		}
	}

	return nids
}

func randomNid() *proto.NodeID {
	return &proto.NodeID{
		Type: NidTypeNormal,
		Id0:  rand.Uint64(),
		Id1:  rand.Uint64(),
	}
}

type dummyVerifier struct {
	validToken string
}

func (dv *dummyVerifier) Verify(token string) (bool, error) {
	if dv.validToken == "error" {
		return false, fmt.Errorf("invalid")
	}

	if dv.validToken == token {
		return true, nil
	}
	return false, nil
}

func TestAuthenticate(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seed := generateEmptySeed(ctx)
	seed.config = "dummy config"
	seed.start()

	nids := uniqueNids(2)

	// normal
	res, code, err := seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     nids[0],
		Token:   "",
	})
	sessionID1 := res.SessionId
	assert.Equal("dummy config", res.Config)
	assert.Equal(HintOnlyOne, res.Hint)
	assert.NotNil(res.SessionId)
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Len(seed.nodes, 1)
	assert.Len(seed.sessions, 1)
	assert.Equal(nidToString(nids[0]), seed.sessions[sessionID1])

	// normal2
	res, code, err = seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     nids[1],
		Token:   "",
	})
	sessionID2 := res.SessionId
	assert.Equal(uint32(0), res.Hint)
	assert.NotNil(res.SessionId)
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Len(seed.nodes, 2)
	assert.Len(seed.sessions, 2)
	assert.Equal(nidToString(nids[1]), seed.sessions[sessionID2])

	assert.NotEqual(nidToString(nids[0]), nidToString(nids[1]))
	assert.NotEqual(sessionID1, sessionID2)

	// duplicate connection
	res, code, err = seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     nids[0],
		Token:   "",
	})
	assert.Nil(res)
	assert.Equal(http.StatusInternalServerError, code)
	assert.Error(err)
	assert.Len(seed.nodes, 1)
	assert.Len(seed.sessions, 1)
	assert.Equal(nidToString(nids[1]), seed.sessions[sessionID2])

	// invalid version
	res, code, err = seed.authenticate(&proto.SeedAuthenticate{
		Version: "dummy",
	})
	assert.Nil(res)
	assert.Equal(http.StatusBadRequest, code)
	assert.Error(err)

	// verification success
	seed.verifier = &dummyVerifier{
		validToken: "token!",
	}
	res, code, err = seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     randomNid(),
		Token:   "token!",
	})
	assert.NotNil(res)
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Len(seed.nodes, 2)
	assert.Len(seed.sessions, 2)

	// verification failed
	res, code, err = seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     randomNid(),
		Token:   "token?",
	})
	assert.Nil(res)
	assert.Equal(http.StatusUnauthorized, code)
	assert.Error(err)
	assert.Len(seed.nodes, 2)
	assert.Len(seed.sessions, 2)

	// verification error
	seed.verifier = &dummyVerifier{
		validToken: "error",
	}
	res, code, err = seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     randomNid(),
		Token:   "",
	})
	assert.Nil(res)
	assert.Equal(http.StatusInternalServerError, code)
	assert.Error(err)
	assert.Len(seed.nodes, 2)
	assert.Len(seed.sessions, 2)
}

func TestClose(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seed := generateEmptySeed(ctx)
	seed.start()

	// normal
	res, code, err := seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     randomNid(),
		Token:   "",
	})
	sessionID1 := res.SessionId
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)
	assert.Len(seed.nodes, 1)
	assert.Len(seed.sessions, 1)

	// close dummy session
	seed.close(&proto.SeedClose{
		SessionId: "dummy",
	})
	assert.Len(seed.nodes, 1)
	assert.Len(seed.sessions, 1)

	// close existing session
	seed.close(&proto.SeedClose{
		SessionId: sessionID1,
	})
	assert.Len(seed.nodes, 0)
	assert.Len(seed.sessions, 0)
}

func TestRelayPoll(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seed := generateEmptySeed(ctx)
	seed.start()

	// connection
	srcNid := randomNid()
	resAuth, code, err := seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     srcNid,
		Token:   "",
	})
	sessionID1 := resAuth.SessionId
	assert.Equal(http.StatusOK, code)
	assert.NoError(err)

	resAuth, code, err = seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     randomNid(),
		Token:   "",
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
	dstNid := randomNid()
	packetID := rand.Uint32()
	resRelay, code, err := seed.relay(&proto.SeedRelay{
		SessionId: sessionID1,
		Packets: []*proto.SeedPacket{
			{
				DstNid: dstNid,
				SrcNid: srcNid,
				Id:     packetID,
				Mode:   ModeOneWay,
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

	// get nil if the sub process finished
	ctxPoll, cancelPoll := context.WithCancel(ctx)
	go func() {
		resPoll, code, err := seed.poll(ctxPoll, &proto.SeedPoll{
			SessionId: sessionID2,
			Online:    true,
		})
		assert.Nil(resPoll)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seed := generateEmptySeed(ctx)
	nids := uniqueNids(7)

	packets := []packet{
		// general packet
		{
			stepNid: nidToString(nids[1]),
			p: &proto.SeedPacket{
				Id:     0,
				DstNid: nids[2],
				SrcNid: nids[3],
				Mode:   0,
			},
		},
		// response packet
		{
			stepNid: nidToString(nids[4]),
			p: &proto.SeedPacket{
				Id:     1,
				DstNid: nids[5],
				SrcNid: nids[6],
				Mode:   ModeResponse,
			},
		},
		// random request packet
		{
			stepNid: nidToString(nids[1]),
			p: &proto.SeedPacket{
				Id:     2,
				DstNid: nids[1],
				SrcNid: nids[1],
				Mode:   0,
			},
		},
	}

	testCases := []struct {
		title       string
		nid         string
		online      bool
		anotherNode bool
		expect      []uint32
	}{
		{
			title:       "can get general packet by online normal node",
			nid:         nidToString(nids[0]),
			online:      true,
			anotherNode: true,
			expect:      []uint32{0, 2},
		},
		{
			title:       "can't get any packet if the packet posted by it self",
			nid:         nidToString(nids[1]),
			online:      true,
			anotherNode: true,
			expect:      []uint32{},
		},
		{
			title:       "can't get any packet if offline",
			nid:         nidToString(nids[0]),
			online:      false,
			anotherNode: true,
			expect:      []uint32{},
		},
		{
			title:       "can get general packet when all node are offline",
			nid:         nidToString(nids[0]),
			online:      false,
			anotherNode: false,
			expect:      []uint32{0, 2},
		},
		{
			title:       "can get general packet when destination is me with offline",
			nid:         nidToString(nids[2]),
			online:      false,
			anotherNode: true,
			expect:      []uint32{0},
		},
		{
			title:       "can get response packet if destination is me",
			nid:         nidToString(nids[5]),
			online:      false,
			anotherNode: true,
			expect:      []uint32{1},
		},
		{
			title:       "can get more than two packets",
			nid:         nidToString(nids[5]),
			online:      true,
			anotherNode: true,
			expect:      []uint32{0, 1, 2},
		},
	}

	for _, testCase := range testCases {
		seed.packets = make([]packet, len(packets))
		copy(seed.packets, packets)

		if testCase.anotherNode {
			seed.nodes = map[string]*node{
				"dummy": {
					timestamp:   time.Now(),
					sessionID:   "dummy",
					online:      true,
					chTimestamp: time.Now(),
					c:           make(chan uint32),
				},
			}
		}

		res := seed.getPackets(testCase.nid, testCase.online, false)
		assert.Len(res, len(testCase.expect), testCase.title)
		for _, r := range res {
			assert.Contains(testCase.expect, r.Id, testCase.title)
		}
		assert.Len(seed.packets, len(packets)-len(testCase.expect), testCase.title)
		for _, p := range seed.packets {
			assert.NotContains(testCase.expect, p.p.Id, testCase.title)
		}

		if testCase.anotherNode {
			close(seed.nodes["dummy"].c)
			delete(seed.nodes, "dummy")
		}
	}
}

func TestRandomConnect(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seed := generateEmptySeed(ctx)
	seed.start()
	nids := uniqueNids(2)

	// not request random-connect when only one node
	resAuth, _, err := seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     nids[0],
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
	resAuth, _, err = seed.authenticate(&proto.SeedAuthenticate{
		Version: ProtocolVersion,
		Nid:     nids[1],
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
		fmt.Println("wait", seed.countChannels(false), seed.countOnline(false))
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
	assert.Equal(HintRequireRandom, resPoll.Hint)
	assert.Len(resPoll.Packets, 0)
}
