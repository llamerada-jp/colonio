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
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/test/testing_seed"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type authenticator struct {
	expectedAccount map[string][]byte
}

func (auth *authenticator) Authenticate(token []byte, nodeID string) (bool, error) {
	expectedToken, ok := auth.expectedAccount[nodeID]
	if !ok {
		return false, nil
	}
	return slices.Equal(expectedToken, token), nil
}

type seedEventHandlerHelper struct {
	authorizeFailed      func()
	changeState          func(bool)
	recvConfig           func(*config.Cluster)
	recvPacket           func(*shared.Packet)
	requireRandomConnect func()
}

func (h *seedEventHandlerHelper) SeedAuthorizeFailed() {
	if h.authorizeFailed != nil {
		h.authorizeFailed()
	}
}

func (h *seedEventHandlerHelper) SeedChangeState(online bool) {
	if h.changeState != nil {
		h.changeState(online)
	}
}

func (h *seedEventHandlerHelper) SeedRecvConfig(conf *config.Cluster) {
	if h.recvConfig != nil {
		h.recvConfig(conf)
	}
}

func (h *seedEventHandlerHelper) SeedRecvPacket(packet *shared.Packet) {
	if h.recvPacket != nil {
		h.recvPacket(packet)
	}
}

func (h *seedEventHandlerHelper) SeedRequireRandomConnect() {
	if h.requireRandomConnect != nil {
		h.requireRandomConnect()
	}
}

func TestSeedAccessor_authSuccess(t *testing.T) {
	nodeID := shared.NewRandomNodeID()

	auth := &authenticator{
		expectedAccount: map[string][]byte{
			nodeID.String(): []byte("token"),
		},
	}

	testSeed := testing_seed.NewTestingSeed(
		testing_seed.WithAuthenticator(auth),
		testing_seed.WithRevision(3.14),
		testing_seed.WithSessionTimeout(35*time.Second),
		testing_seed.WithPollingTimeout(12*time.Second),
	)
	defer testSeed.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	events := ""

	config := &Config{
		Ctx:    ctx,
		Logger: slog.Default(),
		Transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		Handler: &seedEventHandlerHelper{
			authorizeFailed: func() {
				assert.FailNow(t, "authentication should be success")
			},
			changeState: func(online bool) {
				mtx.Lock()
				defer mtx.Unlock()
				if online {
					events += "1"
				} else {
					events += "0"
				}
			},
			recvConfig: func(conf *config.Cluster) {
				mtx.Lock()
				defer mtx.Unlock()
				events += "c"

				assert.Equal(t, 3.14, conf.Revision)
				assert.Equal(t, 35*time.Second, conf.SessionTimeout)
				assert.Equal(t, 12*time.Second, conf.PollingTimeout)
			},
			recvPacket: func(_ *shared.Packet) {
				assert.FailNow(t, "no one should send packet on this test")
			},
			requireRandomConnect: func() {
				assert.FailNow(t, "random connect event should not be happened when only one node")
			},
		},
		URL:          testSeed.URL(),
		LocalNodeID:  nodeID,
		Token:        []byte("token"),
		TripInterval: 10 * time.Second,
	}

	accessor := NewSeedAccessor(config)
	// defer accessor.destruct()
	assert.Eventually(t, func() bool {
		return accessor.IsAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	// change state to online, recv config
	assert.Equal(t, "c1", events)
	mtx.Unlock()

	accessor.Destruct()
	assert.True(t, assert.Eventually(t, func() bool {
		return !accessor.IsAuthenticated()
	}, 3*time.Second, 100*time.Millisecond))

	mtx.Lock()
	// change state to online, recv config, change state to offline
	assert.Equal(t, "c10", events)
	mtx.Unlock()
}

func TestSeedAccessor_authFailure(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	nodeID, differentID := nodeIDs[0], nodeIDs[1]

	auth := &authenticator{
		expectedAccount: map[string][]byte{
			nodeID.String(): []byte("token"),
		},
	}

	testSeed := testing_seed.NewTestingSeed(testing_seed.WithAuthenticator(auth))
	defer testSeed.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	events := ""

	config := &Config{
		Ctx:    ctx,
		Logger: slog.Default(),
		Transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		Handler: &seedEventHandlerHelper{
			authorizeFailed: func() {
				mtx.Lock()
				defer mtx.Unlock()
				events += "f"
			},
			changeState: func(_ bool) {
				assert.FailNow(t, "state event should not be happened when authentication failed")
			},
			recvConfig: func(_ *config.Cluster) {
				assert.FailNow(t, "config data should not be received when authentication failed")
			},
			recvPacket: func(_ *shared.Packet) {
				assert.FailNow(t, "no one should send packet on this test")
			},
			requireRandomConnect: func() {
				assert.FailNow(t, "random connect event should not be happened when only one node")
			},
		},
		URL:          testSeed.URL(),
		TripInterval: 10 * time.Second,
	}

	// authentication failed when token is different
	config.LocalNodeID = nodeID
	config.Token = []byte("different")
	accessor := NewSeedAccessor(config)
	// defer accessor.destruct()
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return events == "f"
	}, 3*time.Second, 100*time.Millisecond)
	assert.False(t, accessor.IsAuthenticated())
	accessor.Destruct()

	// authentication failed when nodeID is different
	config.LocalNodeID = differentID
	config.Token = []byte("token")
	accessor = NewSeedAccessor(config)
	// defer accessor.destruct()
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return events == "ff"
	}, 3*time.Second, 100*time.Millisecond)
	assert.False(t, accessor.IsAuthenticated())
	accessor.Destruct()
}

func TestSeedAccessor_SetEnabled(t *testing.T) {
	nodeID := shared.NewRandomNodeID()

	auth := &authenticator{
		expectedAccount: map[string][]byte{
			nodeID.String(): []byte("token"),
		},
	}

	testSeed := testing_seed.NewTestingSeed(testing_seed.WithAuthenticator(auth))
	defer testSeed.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	events := ""

	config := &Config{
		Ctx:    ctx,
		Logger: slog.Default(),
		Transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		Handler: &seedEventHandlerHelper{
			authorizeFailed: func() {
				assert.FailNow(t, "authentication should be success")
			},
			changeState: func(online bool) {
				mtx.Lock()
				defer mtx.Unlock()
				if online {
					events += "1"
				} else {
					events += "0"
				}
			},
			recvConfig: func(_ *config.Cluster) {
				mtx.Lock()
				defer mtx.Unlock()
				events += "c"
			},
			recvPacket: func(_ *shared.Packet) {
				assert.FailNow(t, "no one should send packet on this test")
			},
			requireRandomConnect: func() {
				assert.FailNow(t, "random connect event should not be happened when only one node")
			},
		},
		URL:          testSeed.URL(),
		LocalNodeID:  nodeID,
		Token:        []byte("token"),
		TripInterval: 10 * time.Second,
	}

	accessor := NewSeedAccessor(config)
	// defer accessor.destruct()
	assert.Eventually(t, func() bool {
		return accessor.IsAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	assert.Equal(t, "c1", events)
	mtx.Unlock()

	accessor.SetEnabled(false)
	assert.Eventually(t, func() bool {
		return !accessor.IsAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	assert.Equal(t, "c10", events)
	mtx.Unlock()

	accessor.SetEnabled(true)
	assert.Eventually(t, func() bool {
		return accessor.IsAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	assert.Equal(t, "c10c1", events)
	mtx.Unlock()

	accessor.Destruct()
	mtx.Lock()
	assert.Equal(t, "c10c10", events)
	mtx.Unlock()
}

func TestSeedAccessor_sessions(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testSeed := testing_seed.NewTestingSeed(
		testing_seed.WithPollingTimeout(500*time.Millisecond),
		testing_seed.WithSessionTimeout(1500*time.Millisecond),
		testing_seed.WithKeepaliveInterval(200*time.Millisecond),
	)

	config := &Config{
		Ctx: ctx,
		Transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		Handler: &seedEventHandlerHelper{
			authorizeFailed: func() {
				assert.FailNow(t, "authentication should be success")
			},
		},
		URL:          testSeed.URL(),
		TripInterval: 1 * time.Second,
	}

	// create pinger
	configPinger := *config
	configPinger.LocalNodeID = nodeIDs[0]
	configPinger.Logger = slog.With("node", "pinger")
	pinger := NewSeedAccessor(&configPinger)
	defer pinger.Destruct()
	assert.Eventually(t, func() bool {
		return pinger.IsAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)
	assert.True(t, pinger.IsOnlyOne())

	// create node will be offline gracefully
	configTarget := *config
	configTarget.LocalNodeID = nodeIDs[1]
	configTarget.Logger = slog.With("node", "target")
	target := NewSeedAccessor(&configTarget)
	// defer target.destruct()
	assert.Eventually(t, func() bool {
		return target.IsAuthenticated() && !target.IsOnlyOne() && !pinger.IsOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	target.SetEnabled(false)
	assert.Eventually(t, func() bool {
		return !target.IsAuthenticated() && pinger.IsOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	target.SetEnabled(true)
	assert.Eventually(t, func() bool {
		return target.IsAuthenticated() && !target.IsOnlyOne() && !pinger.IsOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	target.Destruct()
	assert.Eventually(t, func() bool {
		return !target.IsAuthenticated() && pinger.IsOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	// create node will be offline ungracefully
	ctx2, cancel2 := context.WithCancel(context.Background())
	configTarget.Ctx = ctx2
	target = NewSeedAccessor(&configTarget)
	defer target.Destruct()
	assert.Eventually(t, func() bool {
		return target.IsAuthenticated() && !target.IsOnlyOne() && !pinger.IsOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	cancel2() // cause ungraceful offline by canceling context
	assert.Eventually(t, func() bool {
		return pinger.IsOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	// be offline by session timeout
	testSeed.Pause()
	assert.Eventually(t, func() bool {
		return !pinger.IsAuthenticated()
	}, 10*time.Second, 100*time.Millisecond)

	// be online again
	testSeed.Resume()
	defer testSeed.Stop()
	assert.Eventually(t, func() bool {
		return pinger.IsAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)
}

func TestSeedAccessor_RelayPacket(t *testing.T) {
	// receiver1, receiver2, sender, destination of random, source of random
	nodeIDs := testUtil.UniqueNodeIDs(5)

	// test packet base
	packetBase := shared.Packet{
		DstNodeID: nodeIDs[3],
		SrcNodeID: nodeIDs[4],
		HopCount:  2,
		Mode:      shared.PacketModeRelaySeed,
		Content: &proto.PacketContent{
			Content: &proto.PacketContent_Error{
				Error: &proto.Error{
					Code:    2,
					Message: "test",
				},
			},
		},
	}

	testSeed := testing_seed.NewTestingSeed()
	defer testSeed.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}

	config := &Config{
		Ctx: ctx,
		Transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		URL:          testSeed.URL(),
		TripInterval: 1 * time.Second,
	}

	// create accessor for sender
	configSender := *config
	configSender.LocalNodeID = nodeIDs[2]
	configSender.Logger = slog.With("node", "sender")
	configSender.Handler = &seedEventHandlerHelper{
		authorizeFailed: func() {
			assert.FailNow(t, "authentication should be success")
		},
		recvPacket: func(packet *shared.Packet) {
			assert.FailNow(t, "sender should not receive packet")
		},
	}
	sender := NewSeedAccessor(&configSender)
	defer sender.Destruct()

	// create accessor for receiver 1
	var received *shared.Packet
	receiveCount1 := 0
	configReceiver1 := *config
	configReceiver1.LocalNodeID = nodeIDs[0]
	configReceiver1.Logger = slog.With("node", "receiver1")
	configReceiver1.Handler = &seedEventHandlerHelper{
		authorizeFailed: func() {
			assert.FailNow(t, "authentication should be success")
		},
		recvPacket: func(packet *shared.Packet) {
			mtx.Lock()
			defer mtx.Unlock()
			received = packet
			receiveCount1 += 1
		},
	}
	receiver1 := NewSeedAccessor(&configReceiver1)

	// wait for sender and receiver1 to be online
	assert.Eventually(t, func() bool {
		return sender.IsAuthenticated() && receiver1.IsAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	// check packet content is correctly relayed
	packet := packetBase
	packet.ID = 10
	sender.RelayPacket(&packet)
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return received != nil
	}, 3*time.Second, 100*time.Millisecond)
	require.NotNil(t, received)
	assert.Equal(t, nodeIDs[3].String(), received.DstNodeID.String())
	assert.Equal(t, nodeIDs[4].String(), received.SrcNodeID.String())
	assert.Equal(t, uint32(10), received.ID)
	assert.Equal(t, uint32(2), received.HopCount)
	assert.Equal(t, shared.PacketModeRelaySeed, received.Mode)
	assert.Equal(t, uint32(2), received.Content.GetError().Code)
	assert.Equal(t, "test", received.Content.GetError().Message)
	receiver1.Destruct()

	// recreate receiver1 and receiver2
	mtx.Lock()
	receiveCount1 = 0
	mtx.Unlock()
	receiver1 = NewSeedAccessor(&configReceiver1)
	defer receiver1.Destruct()

	configReceiver2 := *config
	configReceiver2.LocalNodeID = nodeIDs[1]
	configReceiver2.Logger = slog.With("node", "receiver2")
	receiveCount2 := 0
	configReceiver2.Handler = &seedEventHandlerHelper{
		authorizeFailed: func() {
			assert.FailNow(t, "authentication should be success")
		},
		recvPacket: func(packet *shared.Packet) {
			mtx.Lock()
			defer mtx.Unlock()
			receiveCount2 += 1
		},
	}
	receiver2 := NewSeedAccessor(&configReceiver2)
	defer receiver2.Destruct()

	assert.Eventually(t, func() bool {
		return receiver1.IsAuthenticated() && receiver2.IsAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	// send 10 packets and check if receiver1 or receiver2 receive all packets
	for i := 0; i < 10; i++ {
		packet := packetBase
		packet.ID = uint32(100 + i)
		sender.RelayPacket(&packet)
	}
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return receiveCount1+receiveCount2 == 10
	}, 3*time.Second, 100*time.Millisecond)

	// disconnect the receiver that received more packets and check if the other receiver receives all packets
	if receiveCount1 > receiveCount2 {
		receiver1.Destruct()
	} else {
		receiver2.Destruct()
	}
	receiveCount1 = 0
	receiveCount2 = 0

	for i := 0; i < 10; i++ {
		packet := packetBase
		packet.ID = uint32(200 + i)
		sender.RelayPacket(&packet)
	}
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return receiveCount1+receiveCount2 == 10
	}, 3*time.Second, 100*time.Millisecond)
}
