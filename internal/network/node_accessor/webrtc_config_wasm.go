//go:build js

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
	"syscall/js"

	"github.com/llamerada-jp/colonio/config"
)

type webrtcConfigWASM struct {
	wrapper js.Value
	id      uint
}

var _ webRTCConfig = &webrtcConfigWASM{}

func newWebRTCConfigWASM(ice []config.ICEServer) (*webrtcConfigWASM, error) {
	wrapper := js.Global().Get("webrtcWrapper")
	if wrapper.Type() == js.TypeUndefined {
		return nil, fmt.Errorf("webrtcWrapper is not defined, please import colonio.js")
	}
	wrapper.Call("setup")

	iceBin, err := json.Marshal(ice)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ice: %w", err)
	}

	id := wrapper.Call("newConfig", js.ValueOf(string(iceBin))).Int()

	return &webrtcConfigWASM{
		wrapper: wrapper,
		id:      uint(id),
	}, nil
}

func (w *webrtcConfigWASM) getConfigID() uint {
	return w.id
}

func (w *webrtcConfigWASM) destruct() error {
	w.wrapper.Call("deleteConfig", js.ValueOf(w.id))
	return nil
}

func init() {
	defaultWebRTCConfigFactory =
		func(ice []config.ICEServer) (webRTCConfig, error) {
			return newWebRTCConfigWASM(ice)
		}
}
