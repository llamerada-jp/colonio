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
package node_accessor

import (
	"sync"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebRTCLink(t *testing.T) {
	config, err := defaultWebRTCConfigFactory([]config.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	})
	require.NoError(t, err)
	defer config.destruct()

	mtx := sync.Mutex{}
	disableErrorCheck := false
	received := ""
	active1 := ""
	online1 := ""
	active2 := ""
	online2 := ""

	// new
	var link1 webRTCLink
	var link2 webRTCLink
	link1, err = defaultWebRTCLinkFactory(&webRTCLinkConfig{
		webrtcConfig: config,
		isOffer:      true,
	}, &webRTCLinkEventHandler{
		changeLinkState: func(active bool, online bool) {
			mtx.Lock()
			defer mtx.Unlock()
			if active {
				active1 = active1 + "A"
			} else {
				active1 = active1 + "a"
			}
			if online {
				online1 = online1 + "O"
			} else {
				online1 = online1 + "o"
			}
		},
		updateICE: func(ice string) {
			er := link2.updateICE(ice)
			assert.NoError(t, er)
		},
		recvData: func(data []byte) {
			mtx.Lock()
			defer mtx.Unlock()
			received = received + string(data)
		},
		raiseError: func(err string) {
			assert.FailNow(t, "raise error: "+err)
		},
	})
	require.NoError(t, err)
	defer link1.disconnect()

	link2, err = defaultWebRTCLinkFactory(&webRTCLinkConfig{
		webrtcConfig: config,
		isOffer:      false,
	}, &webRTCLinkEventHandler{
		changeLinkState: func(active bool, online bool) {
			mtx.Lock()
			defer mtx.Unlock()
			if active {
				active2 = active2 + "A"
			} else {
				active2 = active2 + "a"
			}
			if online {
				online2 = online2 + "O"
			} else {
				online2 = online2 + "o"
			}
		},
		updateICE: func(ice string) {
			err := link1.updateICE(ice)
			assert.NoError(t, err)
		},
		recvData: func(data []byte) {
			mtx.Lock()
			defer mtx.Unlock()
			received = received + string(data)
		},
		raiseError: func(err string) {
			mtx.Lock()
			defer mtx.Unlock()
			if disableErrorCheck {
				return
			}
			assert.FailNow(t, "raise error: "+err)
		},
	})
	require.NoError(t, err)
	defer link2.disconnect()

	// connect
	go func() {
		sdp1, err := link1.getLocalSDP()
		require.NoError(t, err)
		err = link2.setRemoteSDP(sdp1)
		require.NoError(t, err)

		sdp2, err := link2.getLocalSDP()
		require.NoError(t, err)
		err = link1.setRemoteSDP(sdp2)
		require.NoError(t, err)
	}()

	require.Eventually(t, func() bool {
		return link1.isActive() && link1.isOnline() && link2.isActive() && link2.isOnline()
	}, 3*time.Second, 100*time.Millisecond)

	// send
	err = link1.send([]byte("hello"))
	assert.NoError(t, err)
	err = link2.send([]byte("world"))
	assert.NoError(t, err)

	require.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return received == "helloworld" || received == "worldhello"
	}, 3*time.Second, 100*time.Millisecond)

	// disable error check because an error may be raised when the connection is closed
	mtx.Lock()
	disableErrorCheck = true
	mtx.Unlock()

	// destruct
	link1.disconnect()
	assert.False(t, link1.isActive())

	require.Eventually(t, func() bool {
		return !link1.isOnline() && !link2.isActive() && !link2.isOnline()
	}, 3*time.Second, 100*time.Millisecond)

	link2.disconnect()
	assert.False(t, link2.isActive())
	assert.False(t, link2.isOnline())

	mtx.Lock()
	defer mtx.Unlock()
	assert.Regexp(t, "A+a+", active1)
	assert.Regexp(t, "O+o+", online1)
	assert.Regexp(t, "A+a+", active2)
	assert.Regexp(t, "O+o+", online2)
}
