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

type webRTCLinkConfig struct {
	webrtcConfig      webRTCConfig
	createDataChannel bool
}

type webRTCLinkEventHandler struct {
	raiseError func(string)
	// tell active, online value
	changeLinkState func(bool, bool)
	updateICE       func(string)
	recvData        func([]byte)
}

type webRTCLink interface {
	isActive() bool
	isOnline() bool
	getLocalSDP() (string, error)
	setRemoteSDP(sdp string) error
	updateICE(ice string) error
	send(data []byte) error
	disconnect() error
}

var defaultWebRTCLinkFactory func(config *webRTCLinkConfig, eventHandler *webRTCLinkEventHandler) (webRTCLink, error)
