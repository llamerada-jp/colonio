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

	"github.com/llamerada-jp/colonio"
	"github.com/llamerada-jp/colonio/internal/node_id"
	"github.com/llamerada-jp/colonio/internal/proto"
	"github.com/llamerada-jp/colonio/seed"
	"github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
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
	nodeID := *node_id.NewRandom()

	auth := &authenticator{
		expectedAccount: map[string][]byte{
			nodeID.String(): []byte("token"),
		},
	}

	testSeed := util.NewTestingSeed(util.WithAuthenticator(auth))
	defer testSeed.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mtx := sync.Mutex{}
	events := ""

	config := &seedAccessorConfig{
		ctx:    ctx,
		logger: slog.Default(),
		transporter: colonio.DefaultSeedTransporterFactory(&colonio.SeedTransporterOption{
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
		recvConfigEventHandler: func(_ *seed.Config) {
			mtx.Lock()
			defer mtx.Unlock()
			events += "c"
		},
		recvPacketEventHandler: func(_ *proto.SeedPacket) {
			assert.FailNow(t, "no one should send packet on this test")
		},
		requireRandomConnectEventHandler: func() {
			assert.FailNow(t, "random connect event should not be happened when only one node")
		},
	}

	accessor := newSeedAccessor(config)
	assert.Eventually(t, func() bool {
		return accessor.authenticated()
	}, 3*time.Second, 100*time.Millisecond)

	mtx.Lock()
	// change state to online, recv config
	assert.Equal(t, "cs", events)
	mtx.Unlock()

	accessor.disconnect()
	assert.True(t, assert.Eventually(t, func() bool {
		return !accessor.authenticated()
	}, 3*time.Second, 100*time.Millisecond))

	mtx.Lock()
	// change state to online, recv config, change state to offline
	assert.Equal(t, "css", events)
	mtx.Unlock()
}
