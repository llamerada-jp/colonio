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
#cgo LDFLAGS: -L../../dep/lib -L../../output -lcolonio -lwebrtc -lstdc++ -lm -lpthread
#endif

#include "../../c/webrtc.h"

void cgo_webrtc_config_init() {
	webrtc_config_init();
}

unsigned int cgo_webrtc_config_new(_GoString_ ice) {
	return webrtc_config_new(_GoStringPtr(ice), _GoStringLen(ice));
}

void cgo_webrtc_config_destruct(unsigned int id) {
	webrtc_config_destruct(id);
}
*/
import "C"

import (
	"encoding/json"
	"fmt"

	"github.com/llamerada-jp/colonio/config"
)

type webRTCConfigNative struct {
	id uint
}

var _ webRTCConfig = &webRTCConfigNative{}

func newWebRTCConfigNative(ice []config.ICEServer) (*webRTCConfigNative, error) {
	iceBin, err := json.Marshal(ice)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ice: %w", err)
	}

	return &webRTCConfigNative{
		id: uint(C.cgo_webrtc_config_new(string(iceBin))),
	}, nil
}

func (w *webRTCConfigNative) getConfigID() uint {
	return w.id
}

func (w *webRTCConfigNative) destruct() error {
	C.cgo_webrtc_config_destruct(C.uint(w.id))
	return nil
}

func init() {
	C.cgo_webrtc_config_init()

	defaultWebRTCConfigFactory =
		func(ice []config.ICEServer) (webRTCConfig, error) {
			return newWebRTCConfigNative(ice)
		}
}
