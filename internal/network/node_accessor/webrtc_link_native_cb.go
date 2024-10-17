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
	"C"
	"unsafe"
)

//export UpdateWebRTCLinkState
func UpdateWebRTCLinkState(id C.uint, online C.int) {
	link, ok := getLink(id)
	if !ok {
		return
	}

	link.mtx.Lock()
	prevActive := link.active
	prevOnline := link.online

	if link.online && online == 0 {
		link.active = false
	}
	link.online = online != 0

	if prevActive != link.active || prevOnline != link.online {
		a := link.active
		o := link.online
		defer link.eventHandler.changeLinkState(a, o)
	}
	link.mtx.Unlock()
}

//export UpdateICE
func UpdateICE(id C.uint, ice unsafe.Pointer, iceLen C.int) {
	link, ok := getLink(id)
	if !ok {
		return
	}

	link.eventHandler.updateICE(string(C.GoBytes(ice, C.int(iceLen))))
}

//export ReceiveData
func ReceiveData(id C.uint, data unsafe.Pointer, dataLen C.int) {
	link, ok := getLink(id)
	if !ok {
		return
	}

	link.eventHandler.recvData(C.GoBytes(data, dataLen))
}

//export ReceiveError
func ReceiveError(id C.uint, message *C.char, messageLen C.int) {
	link, ok := getLink(id)
	if !ok {
		return
	}

	link.eventHandler.raiseError(C.GoStringN(message, messageLen))
}

func getLink(id C.uint) (*webRTCLinkNative, bool) {
	linksMtx.Lock()
	defer linksMtx.Unlock()
	link, ok := links[id]
	return link, ok
}
