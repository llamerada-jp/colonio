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

func TestSeedAccessorAuthSuccess(t *testing.T) {
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

	config := &seedAccessorConfig{
		ctx:    ctx,
		logger: slog.Default(),
		transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		url:          fmt.Sprintf("https://localhost:%d/test", testSeed.Port()),
		nodeID:       nodeID,
		token:        []byte("token"),
		tripInterval: 10 * time.Second,

		authorizeFailedEventHandler: func() {
			assert.FailNow(t, "authentication should be success")
		},
		changeStateEventHandler: func() {
			mtx.Lock()
			defer mtx.Unlock()
			events += "s"
		},
		recvConfigEventHandler: func(conf *config.Cluster) {
			mtx.Lock()
			defer mtx.Unlock()
			events += "c"

			assert.Equal(t, 3.14, conf.Revision)
			assert.Equal(t, 35*time.Second, conf.SessionTimeout)
			assert.Equal(t, 12*time.Second, conf.PollingTimeout)
		},
		recvPacketEventHandler: func(_ *shared.Packet) {
			assert.FailNow(t, "no one should send packet on this test")
		},
		requireRandomConnectEventHandler: func() {
			assert.FailNow(t, "random connect event should not be happened when only one node")
		},
	}

	accessor := newSeedAccessor(config)
	// defer accessor.destruct()
	assert.Eventually(t, func() bool {
		return accessor.isAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	// change state to online, recv config
	assert.Equal(t, "cs", events)
	mtx.Unlock()

	accessor.destruct()
	assert.True(t, assert.Eventually(t, func() bool {
		return !accessor.isAuthenticated()
	}, 3*time.Second, 100*time.Millisecond))

	mtx.Lock()
	// change state to online, recv config, change state to offline
	assert.Equal(t, "css", events)
	mtx.Unlock()
}

func TestSeedAccessorAuthFailure(t *testing.T) {
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

	config := &seedAccessorConfig{
		ctx:    ctx,
		logger: slog.Default(),
		transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		url:          fmt.Sprintf("https://localhost:%d/test", testSeed.Port()),
		tripInterval: 10 * time.Second,

		authorizeFailedEventHandler: func() {
			mtx.Lock()
			defer mtx.Unlock()
			events += "f"
		},
		changeStateEventHandler: func() {
			assert.FailNow(t, "state event should not be happened when authentication failed")
		},
		recvConfigEventHandler: func(_ *config.Cluster) {
			assert.FailNow(t, "config data should not be received when authentication failed")
		},
		recvPacketEventHandler: func(_ *shared.Packet) {
			assert.FailNow(t, "no one should send packet on this test")
		},
		requireRandomConnectEventHandler: func() {
			assert.FailNow(t, "random connect event should not be happened when only one node")
		},
	}

	// authentication failed when token is different
	config.nodeID = nodeID
	config.token = []byte("different")
	accessor := newSeedAccessor(config)
	// defer accessor.destruct()
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return events == "f"
	}, 3*time.Second, 100*time.Millisecond)
	assert.False(t, accessor.isAuthenticated())
	accessor.destruct()

	// authentication failed when nodeID is different
	config.nodeID = differentID
	config.token = []byte("token")
	accessor = newSeedAccessor(config)
	// defer accessor.destruct()
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return events == "ff"
	}, 3*time.Second, 100*time.Millisecond)
	assert.False(t, accessor.isAuthenticated())
	accessor.destruct()
}

func TestSeedAccessorActivate(t *testing.T) {
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

	config := &seedAccessorConfig{
		ctx:    ctx,
		logger: slog.Default(),
		transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		url:          fmt.Sprintf("https://localhost:%d/test", testSeed.Port()),
		nodeID:       nodeID,
		token:        []byte("token"),
		tripInterval: 10 * time.Second,

		authorizeFailedEventHandler: func() {
			assert.FailNow(t, "authentication should be success")
		},
		changeStateEventHandler: func() {
			mtx.Lock()
			defer mtx.Unlock()
			events += "s"
		},
		recvConfigEventHandler: func(_ *config.Cluster) {
			mtx.Lock()
			defer mtx.Unlock()
			events += "c"
		},
		recvPacketEventHandler: func(_ *shared.Packet) {
			assert.FailNow(t, "no one should send packet on this test")
		},
		requireRandomConnectEventHandler: func() {
			assert.FailNow(t, "random connect event should not be happened when only one node")
		},
	}

	accessor := newSeedAccessor(config)
	// defer accessor.destruct()
	assert.Eventually(t, func() bool {
		return accessor.isAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	assert.Equal(t, "cs", events)
	mtx.Unlock()

	accessor.activate(false)
	assert.Eventually(t, func() bool {
		return !accessor.isAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	assert.Equal(t, "css", events)
	mtx.Unlock()

	accessor.activate(true)
	assert.Eventually(t, func() bool {
		return accessor.isAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	assert.Equal(t, "csscs", events)
	mtx.Unlock()

	accessor.destruct()
	assert.Equal(t, "csscss", events)
}

func TestSeedAccessorSessions(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testSeed := testing_seed.NewTestingSeed(
		testing_seed.WithPollingTimeout(500*time.Millisecond),
		testing_seed.WithSessionTimeout(1500*time.Millisecond),
	)

	config := &seedAccessorConfig{
		ctx: ctx,
		transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		url:          fmt.Sprintf("https://localhost:%d/test", testSeed.Port()),
		tripInterval: 1 * time.Second,

		authorizeFailedEventHandler: func() {
			assert.FailNow(t, "authentication should be success")
		},
		changeStateEventHandler:          func() {},
		recvConfigEventHandler:           func(_ *config.Cluster) {},
		recvPacketEventHandler:           func(_ *shared.Packet) {},
		requireRandomConnectEventHandler: func() {},
	}

	// create pinger
	configPinger := *config
	configPinger.nodeID = nodeIDs[0]
	configPinger.logger = slog.With("node", "pinger")
	pinger := newSeedAccessor(&configPinger)
	defer pinger.destruct()
	assert.Eventually(t, func() bool {
		return pinger.isAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)
	assert.True(t, pinger.isOnlyOne())

	// create node will be offline gracefully
	configTarget := *config
	configTarget.nodeID = nodeIDs[1]
	configTarget.logger = slog.With("node", "target")
	target := newSeedAccessor(&configTarget)
	// defer target.destruct()
	assert.Eventually(t, func() bool {
		return target.isAuthenticated() && !target.isOnlyOne() && !pinger.isOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	target.activate(false)
	assert.Eventually(t, func() bool {
		return !target.isAuthenticated() && pinger.isOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	target.activate(true)
	assert.Eventually(t, func() bool {
		return target.isAuthenticated() && !target.isOnlyOne() && !pinger.isOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	target.destruct()
	assert.Eventually(t, func() bool {
		return !target.isAuthenticated() && pinger.isOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	// create node will be offline ungracefully
	ctx2, cancel2 := context.WithCancel(context.Background())
	configTarget.ctx = ctx2
	target = newSeedAccessor(&configTarget)
	defer target.destruct()
	assert.Eventually(t, func() bool {
		return target.isAuthenticated() && !target.isOnlyOne() && !pinger.isOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	cancel2() // cause ungraceful offline by canceling context
	assert.Eventually(t, func() bool {
		return pinger.isOnlyOne()
	}, 3*time.Second, 100*time.Millisecond)

	// be offline by session timeout
	testSeed.Stop()
	assert.Eventually(t, func() bool {
		return !pinger.isAuthenticated()
	}, 10*time.Second, 100*time.Millisecond)

	// be online again
	testSeed = testing_seed.NewTestingSeed()
	defer testSeed.Stop()
	assert.Eventually(t, func() bool {
		return pinger.isAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)
}

func TestSeedAccessorRelayPacket(t *testing.T) {
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

	config := &seedAccessorConfig{
		ctx: ctx,
		transporter: DefaultSeedTransporterFactory(&SeedTransporterOption{
			Verification: false,
		}),
		url:          fmt.Sprintf("https://localhost:%d/test", testSeed.Port()),
		tripInterval: 1 * time.Second,

		authorizeFailedEventHandler: func() {
			assert.FailNow(t, "authentication should be success")
		},
		changeStateEventHandler:          func() {},
		recvConfigEventHandler:           func(_ *config.Cluster) {},
		recvPacketEventHandler:           func(_ *shared.Packet) {},
		requireRandomConnectEventHandler: func() {},
	}

	// create accessor for sender
	configSender := *config
	configSender.nodeID = nodeIDs[2]
	configSender.logger = slog.With("node", "sender")
	configSender.recvPacketEventHandler = func(packet *shared.Packet) {
		assert.FailNow(t, "sender should not receive packet")
	}
	sender := newSeedAccessor(&configSender)
	defer sender.destruct()

	// create accessor for receiver 1
	var received *shared.Packet
	configReceiver1 := *config
	configReceiver1.nodeID = nodeIDs[0]
	configReceiver1.logger = slog.With("node", "receiver1")
	configReceiver1.recvPacketEventHandler = func(packet *shared.Packet) {
		mtx.Lock()
		defer mtx.Unlock()
		received = packet
	}
	receiver1 := newSeedAccessor(&configReceiver1)

	// wait for sender and receiver1 to be online
	assert.Eventually(t, func() bool {
		return sender.isAuthenticated() && receiver1.isAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	// check packet content is correctly relayed
	packet := packetBase
	packet.ID = 10
	sender.relayPacket(&packet)
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
	receiver1.destruct()

	// recreate receiver1 and receiver2
	receiveCount1 := 0
	configReceiver1.recvPacketEventHandler = func(packet *shared.Packet) {
		mtx.Lock()
		defer mtx.Unlock()
		receiveCount1 += 1
	}
	receiver1 = newSeedAccessor(&configReceiver1)
	defer receiver1.destruct()

	configReceiver2 := *config
	configReceiver2.nodeID = nodeIDs[1]
	configReceiver2.logger = slog.With("node", "receiver2")
	receiveCount2 := 0
	configReceiver2.recvPacketEventHandler = func(packet *shared.Packet) {
		mtx.Lock()
		defer mtx.Unlock()
		receiveCount2 += 1
	}
	receiver2 := newSeedAccessor(&configReceiver2)
	defer receiver2.destruct()

	assert.Eventually(t, func() bool {
		return receiver1.isAuthenticated() && receiver2.isAuthenticated()
	}, 3*time.Second, 100*time.Millisecond)

	// send 10 packets and check if receiver1 or receiver2 receive all packets
	for i := 0; i < 10; i++ {
		packet := packetBase
		packet.ID = uint32(100 + i)
		sender.relayPacket(&packet)
	}
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return receiveCount1+receiveCount2 == 10
	}, 3*time.Second, 100*time.Millisecond)

	// disconnect the receiver that received more packets and check if the other receiver receives all packets
	if receiveCount1 > receiveCount2 {
		receiver1.destruct()
	} else {
		receiver2.destruct()
	}
	receiveCount1 = 0
	receiveCount2 = 0

	for i := 0; i < 10; i++ {
		packet := packetBase
		packet.ID = uint32(200 + i)
		sender.relayPacket(&packet)
	}
	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return receiveCount1+receiveCount2 == 10
	}, 3*time.Second, 100*time.Millisecond)
}
