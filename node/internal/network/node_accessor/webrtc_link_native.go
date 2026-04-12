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
package node_accessor

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/pion/webrtc/v4"
)

type webRTCLinkNative struct {
	isOffer        bool
	eventHandler   *webRTCLinkEventHandler
	peerConnection *webrtc.PeerConnection
	dataChannel    *webrtc.DataChannel
	mtx            sync.RWMutex
	ices           []string
	active         bool
	online         bool
}

var _ webRTCLink = &webRTCLinkNative{}

func newWebRTCLinkNative(c *webRTCLinkConfig, eventHandler *webRTCLinkEventHandler) (webRTCLink, error) {
	config := webrtc.Configuration{
		ICEServers: make([]webrtc.ICEServer, 0, len(c.iceServers)),
	}
	for _, i := range c.iceServers {
		config.ICEServers = append(config.ICEServers, webrtc.ICEServer{
			URLs:       i.URLs,
			Username:   i.Username,
			Credential: i.Credential,
		})
	}

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %s", err)
	}

	w := &webRTCLinkNative{
		isOffer:        c.isOffer,
		eventHandler:   eventHandler,
		peerConnection: peerConnection,
		ices:           make([]string, 0),
		active:         true,
		online:         false,
	}

	if c.isOffer {
		dc, err := peerConnection.CreateDataChannel(c.label, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create data channel: %s", err)
		}
		w.dataChannel = dc
		w.setDataChannelHandler(dc)
	}

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		w.mtx.Lock()

		a := w.active
		o := w.online

		if state == webrtc.PeerConnectionStateDisconnected ||
			state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			w.active = false
			w.online = false
		}

		if w.active != a || w.online != o {
			a := w.active
			o := w.online
			defer eventHandler.changeLinkState(a, o)
		}

		w.mtx.Unlock()
	})

	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		w.eventHandler.updateICE(candidate.ToJSON().Candidate)
	})

	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		w.mtx.Lock()
		defer w.mtx.Unlock()
		if w.isOffer {
			if w.dataChannel != d {
				panic("data channel is not the same")
			}
		} else {
			w.dataChannel = d
			w.setDataChannelHandler(d)
		}
	})

	return w, nil
}

func (w *webRTCLinkNative) setDataChannelHandler(dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		w.mtx.Lock()

		o := w.online
		w.online = true

		if w.online != o {
			a := w.active
			o := w.online
			defer w.eventHandler.changeLinkState(a, o)
		}

		w.mtx.Unlock()
	})

	dc.OnClose(func() {
		w.mtx.Lock()

		a := w.active
		o := w.online
		w.active = false
		w.online = false
		if w.active != a || w.online != o {
			a := w.active
			o := w.online
			defer w.eventHandler.changeLinkState(a, o)
		}
		w.mtx.Unlock()
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		w.eventHandler.recvData(msg.Data)
	})

	dc.OnError(func(err error) {
		w.mtx.RLock()
		a := w.active
		w.mtx.RUnlock()

		if !a {
			return
		}
		w.eventHandler.raiseError(err.Error())
	})
}

func (w *webRTCLinkNative) isActive() bool {
	w.mtx.RLock()
	defer w.mtx.RUnlock()
	return w.active
}

func (w *webRTCLinkNative) isOnline() bool {
	w.mtx.RLock()
	defer w.mtx.RUnlock()
	return w.online
}

func (w *webRTCLinkNative) getLabel() string {
	w.mtx.RLock()
	defer w.mtx.RUnlock()
	if w.dataChannel == nil {
		panic("data channel should not be nil")
	}
	return w.dataChannel.Label()
}

func (w *webRTCLinkNative) getLocalSDP() (string, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if w.peerConnection == nil {
		return "", fmt.Errorf("peer connection has been closed")
	}

	var sdp webrtc.SessionDescription
	var err error
	if w.isOffer {
		sdp, err = w.peerConnection.CreateOffer(nil)
		if err != nil {
			return "", fmt.Errorf("failed to create offer: %s", err)
		}
	} else {
		sdp, err = w.peerConnection.CreateAnswer(nil)
		if err != nil {
			return "", fmt.Errorf("failed to create answer: %s", err)
		}
	}

	if err := w.peerConnection.SetLocalDescription(sdp); err != nil {
		return "", fmt.Errorf("failed to set local description: %s", err)
	}

	b, err := json.Marshal(sdp)
	if err != nil {
		return "", fmt.Errorf("failed to marshal local description: %s", err)
	}
	return string(b), nil
}

func (w *webRTCLinkNative) setRemoteSDP(sdp string) error {
	description := &webrtc.SessionDescription{}
	err := json.Unmarshal([]byte(sdp), description)
	if err != nil {
		return fmt.Errorf("failed to unmarshal remote description: %s", err)
	}

	w.mtx.Lock()
	defer w.mtx.Unlock()
	if w.peerConnection == nil {
		return fmt.Errorf("peer connection has been closed")
	}

	if err := w.peerConnection.SetRemoteDescription(*description); err != nil {
		return fmt.Errorf("failed to set remote description: %s", err)
	}

	if w.ices != nil {
		for _, ice := range w.ices {
			if err := w.peerConnection.AddICECandidate(webrtc.ICECandidateInit{
				Candidate: ice,
			}); err != nil {
				return fmt.Errorf("failed to add ICE candidate: %s", err)
			}
		}
		w.ices = nil
	}

	return nil
}

func (w *webRTCLinkNative) updateICE(ice string) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.ices != nil {
		w.ices = append(w.ices, ice)
		return nil
	}

	if w.peerConnection == nil {
		return fmt.Errorf("peer connection has been closed")
	}

	if err := w.peerConnection.AddICECandidate(webrtc.ICECandidateInit{
		Candidate: ice,
	}); err != nil {
		return fmt.Errorf("failed to add ICE candidate: %s", err)
	}
	return nil
}

func (w *webRTCLinkNative) send(data []byte) error {
	if err := w.dataChannel.Send(data); err != nil {
		return fmt.Errorf("failed to send data: %s", err)
	}
	return nil
}

func (w *webRTCLinkNative) disconnect() error {
	w.mtx.Lock()

	a := w.active
	w.active = false

	o := w.online

	pc := w.peerConnection
	w.peerConnection = nil

	w.mtx.Unlock() // should not return before this line

	// the link is already closed
	if pc == nil {
		return nil
	}

	// when `active` is true before disconnect, state of link is changed to `false` after disconnect
	if a {
		defer w.eventHandler.changeLinkState(false, o)
	}

	if err := pc.Close(); err != nil {
		return fmt.Errorf("failed to close peer connection: %s", err)
	}

	return nil
}

func init() {
	defaultWebRTCLinkFactory = func(config *webRTCLinkConfig, eventHandler *webRTCLinkEventHandler) (webRTCLink, error) {
		return newWebRTCLinkNative(config, eventHandler)
	}
}
