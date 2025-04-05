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
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/llamerada-jp/colonio/config"
	webrtc "github.com/pion/webrtc/v4"
)

var (
	mtxConfig        sync.RWMutex
	configurationMap = make(map[uint]*webrtc.Configuration)
)

type webRTCConfigNative struct {
	id uint
}

var _ webRTCConfig = &webRTCConfigNative{}

func getConfigByID(id uint) (*webrtc.Configuration, error) {
	mtxConfig.RLock()
	defer mtxConfig.RUnlock()

	config, ok := configurationMap[id]
	if !ok {
		return nil, fmt.Errorf("invalid config ID: %d", id)
	}
	return config, nil
}

func newWebRTCConfigNative(ice []config.ICEServer) (*webRTCConfigNative, error) {
	mtxConfig.Lock()
	defer mtxConfig.Unlock()

	id := rand.Uint()
	for {
		if _, ok := configurationMap[id]; !ok {
			break
		}
		id = rand.Uint()
	}

	config := &webrtc.Configuration{
		ICEServers: make([]webrtc.ICEServer, 0, len(ice)),
	}
	for _, i := range ice {
		config.ICEServers = append(config.ICEServers, webrtc.ICEServer{
			URLs:       i.URLs,
			Username:   i.Username,
			Credential: i.Credential,
		})
	}
	configurationMap[id] = config

	return &webRTCConfigNative{
		id: id,
	}, nil
}

func (w *webRTCConfigNative) getConfigID() uint {
	mtxConfig.RLock()
	defer mtxConfig.RUnlock()
	return w.id
}

func (w *webRTCConfigNative) destruct() error {
	mtxConfig.Lock()
	defer mtxConfig.Unlock()
	delete(configurationMap, w.id)
	return nil
}

func init() {
	defaultWebRTCConfigFactory =
		func(ice []config.ICEServer) (webRTCConfig, error) {
			return newWebRTCConfigNative(ice)
		}
}
