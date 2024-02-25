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

/*
#ifndef SET_LDFLAGS
#cgo LDFLAGS: -L../../../dep/lib -L../../../output -lcolonio -lwebrtc -lstdc++ -lm -lpthread
#endif

#include <stdlib.h>

#include "../../c/webrtc.h"

// functions exported from webrtc_link_native_cb.go
extern void UpdateWebRTCLinkState(unsigned int id, int online);
extern void UpdateICE(unsigned int id, const void* ice, int ice_len);
extern void ReceiveData(unsigned int id, const void* data, int data_len);
extern void ReceiveError(unsigned int id, const char* message, int message_len);

void cgo_webrtc_link_get_error_message(const char** message, int* message_len) {
	webrtc_link_get_error_message(message, message_len);
}

void cgo_webrtc_link_init() {
	webrtc_link_init(UpdateWebRTCLinkState, UpdateICE, ReceiveData, ReceiveError);
}

int cgo_webrtc_link_disconnect(unsigned int id) {
	return webrtc_link_disconnect(id);
}

unsigned int cgo_webrtc_link_new(unsigned int config_id, int create_data_channel) {
	return webrtc_link_new(config_id, create_data_channel);
}

int cgo_webrtc_link_get_local_sdp(unsigned int id, const char** sdp, int* sdp_len) {
	return webrtc_link_get_local_sdp(id, sdp, sdp_len);
}

int cgo_webrtc_link_set_remote_sdp(unsigned int id, _GoString_ sdp) {
	return webrtc_link_set_remote_sdp(id, _GoStringPtr(sdp), _GoStringLen(sdp));
}

int cgo_webrtc_link_update_ice(unsigned int id, _GoString_ ice) {
	return webrtc_link_update_ice(id, _GoStringPtr(ice), _GoStringLen(ice));
}

int cgo_webrtc_link_send(unsigned int id, const void* data, int data_len) {
	return webrtc_link_send(id, data, data_len);
}
*/
import "C"
import (
	"fmt"
	"sync"
)

type webRTCLinkNative struct {
	config       *webRTCLinkConfig
	eventHandler *webRTCLinkEventHandler
	id           C.uint

	mtx    sync.RWMutex
	active bool
	online bool
}

var links = make(map[C.uint]*webRTCLinkNative)
var linksMtx = sync.RWMutex{}

var _ webRTCLink = &webRTCLinkNative{}

func newWebRTCLinkNative(config *webRTCLinkConfig, eventHandler *webRTCLinkEventHandler) (webRTCLink, error) {
	createDataChannel := C.int(0)
	if config.createDataChannel {
		createDataChannel = C.int(1)
	}
	configID := C.uint(config.webrtcConfig.getConfigID())
	linkID := C.cgo_webrtc_link_new(configID, createDataChannel)
	if linkID == 0 {
		return nil, fmt.Errorf("failed to create webrtc link")
	}

	link := &webRTCLinkNative{
		config:       config,
		eventHandler: eventHandler,
		id:           linkID,
		mtx:          sync.RWMutex{},
		active:       true,
		online:       false,
	}

	linksMtx.Lock()
	defer linksMtx.Unlock()
	links[link.id] = link

	return link, nil
}

func getErrorMessage() string {
	var messagePtr *C.char
	var messageLen C.int
	C.cgo_webrtc_link_get_error_message(&messagePtr, &messageLen)
	return C.GoStringN(messagePtr, messageLen)
}

func (w *webRTCLinkNative) isActive() bool {
	w.mtx.RLock()
	defer w.mtx.RUnlock()
	if w.id == 0 {
		return false

	}
	return w.active
}

func (w *webRTCLinkNative) isOnline() bool {
	w.mtx.RLock()
	defer w.mtx.RUnlock()
	if w.id == 0 {
		return false
	}
	return w.online
}

func (w *webRTCLinkNative) getLocalSDP() (string, error) {
	w.mtx.Lock()
	id := w.id
	w.mtx.Unlock()

	if id == 0 {
		return "", fmt.Errorf("link is not active")
	}

	var sdpPtr *C.char
	var sdpLen C.int
	err := C.cgo_webrtc_link_get_local_sdp(id, &sdpPtr, &sdpLen)
	if err != 0 {
		return "", fmt.Errorf("failed to get local SDP: %s", getErrorMessage())
	}
	return C.GoStringN(sdpPtr, sdpLen), nil
}

func (w *webRTCLinkNative) setRemoteSDP(sdp string) error {
	w.mtx.Lock()
	id := w.id
	w.mtx.Unlock()

	if id == 0 {
		return fmt.Errorf("link is not active")
	}

	err := C.cgo_webrtc_link_set_remote_sdp(id, sdp)
	if err != 0 {
		return fmt.Errorf("failed to set remote SDP: %s", getErrorMessage())
	}
	return nil
}

func (w *webRTCLinkNative) updateICE(ice string) error {
	w.mtx.Lock()
	id := w.id
	w.mtx.Unlock()

	if id == 0 {
		return fmt.Errorf("link is not active")
	}

	err := C.cgo_webrtc_link_update_ice(id, ice)
	if err != 0 {
		return fmt.Errorf("failed to update ICE: %s", getErrorMessage())
	}
	return nil
}

func (w *webRTCLinkNative) send(data []byte) error {
	w.mtx.Lock()
	id := w.id
	w.mtx.Unlock()

	if id == 0 {
		return fmt.Errorf("link is not active")
	}

	ptr := C.CBytes(data)
	defer C.free(ptr)
	err := C.cgo_webrtc_link_send(id, ptr, C.int(len(data)))
	if err != 0 {
		return fmt.Errorf("failed to send data: %s", getErrorMessage())
	}
	return nil
}

func (w *webRTCLinkNative) disconnect() error {
	w.mtx.Lock()
	id := w.id
	active := w.active
	w.id = 0
	w.active = false
	w.mtx.Unlock()

	if id == 0 {
		return nil
	}

	linksMtx.Lock()
	delete(links, id)
	linksMtx.Unlock()

	err := C.cgo_webrtc_link_disconnect(id)
	if err != 0 {
		return fmt.Errorf("failed to disconnect: %s", getErrorMessage())
	}

	if active {
		go w.eventHandler.changeLinkState(false, false)
	}

	return nil
}

func init() {
	C.cgo_webrtc_link_init()
	defaultWebRTCLinkFactory = func(config *webRTCLinkConfig, eventHandler *webRTCLinkEventHandler) (webRTCLink, error) {
		return newWebRTCLinkNative(config, eventHandler)
	}
}
