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
	"fmt"
	"sync"
	"syscall/js"
)

type webRTCLinkWASM struct {
	config       *webRTCLinkConfig
	eventHandler *webRTCLinkEventHandler
	id           int

	mtx        sync.RWMutex
	active     bool
	online     bool
	label      string
	localSDPCh chan string
}

var links = make(map[int]*webRTCLinkWASM)
var linksMtx sync.Mutex
var jsWrapper js.Value

var _ webRTCLink = &webRTCLinkWASM{}

func jsOnUpdateLinkState(_ js.Value, args []js.Value) interface{} {
	id := args[0].Int()
	online := args[1].Bool()

	link, ok := getLink(id)
	if !ok {
		return nil
	}

	link.mtx.Lock()
	prevActive := link.active
	prevOnline := link.online

	if link.online && !online {
		link.active = false
	}
	link.online = online

	if prevActive != link.active || prevOnline != link.online {
		a := link.active
		o := link.online
		defer link.eventHandler.changeLinkState(a, o)

		if link.online {
			c := make(chan string, 1)
			defer close(c)

			go func() {
				res := jsWrapper.Call("getLabel", js.ValueOf(link.id)).String()
				c <- res
			}()

			res := <-c
			if len(res) == 0 {
				link.eventHandler.raiseError("failed to get label")
				return nil
			}
			link.label = res
		}
	}
	link.mtx.Unlock()

	return nil
}

func jsOnUpdateICE(_ js.Value, args []js.Value) interface{} {
	id := args[0].Int()
	ice := args[1].String()

	link, ok := getLink(id)
	if !ok {
		return nil
	}

	link.eventHandler.updateICE(ice)

	return nil
}

func jsOnGetLocalSDP(_ js.Value, args []js.Value) interface{} {
	id := args[0].Int()
	sdp := args[1].String()

	link, ok := getLink(id)
	if !ok {
		return nil
	}

	link.localSDPCh <- sdp

	return nil
}

func jsOnReceiveData(_ js.Value, args []js.Value) interface{} {
	id := args[0].Int()
	dataJS := args[1]
	data := make([]byte, dataJS.Get("byteLength").Int())
	js.CopyBytesToGo(data, dataJS)

	link, ok := getLink(id)
	if !ok {
		return nil
	}

	link.eventHandler.recvData(data)

	return nil
}

func jsOnRaiseError(_ js.Value, args []js.Value) interface{} {
	id := args[0].Int()
	message := args[1].String()

	link, ok := getLink(id)
	if !ok {
		return nil
	}

	link.eventHandler.raiseError(message)

	return nil
}

func newWebRTCLinkWASM(config *webRTCLinkConfig, eventHandler *webRTCLinkEventHandler) (webRTCLink, error) {
	type res struct {
		linkID int
		err    error
	}
	c := make(chan res, 1)

	go func() {
		linkID := jsWrapper.Call("newLink",
			js.ValueOf(config.webrtcConfig.getConfigID()),
			js.ValueOf(config.isOffer),
			js.ValueOf(config.label),
		).Int()
		if linkID == 0 {
			c <- res{
				linkID: 0,
				err:    fmt.Errorf("failed to create webrtc link"),
			}
		}
		c <- res{
			linkID: linkID,
			err:    nil,
		}
	}()

	r := <-c
	if r.err != nil {
		return nil, r.err
	}

	link := &webRTCLinkWASM{
		config:       config,
		eventHandler: eventHandler,
		id:           r.linkID,
		mtx:          sync.RWMutex{},
		active:       true,
		online:       false,
		localSDPCh:   make(chan string, 1),
	}
	links[link.id] = link

	return link, nil
}

func (w *webRTCLinkWASM) getLastError() string {
	return jsWrapper.Call("getLastError", js.ValueOf(w.id)).String()
}

func (w *webRTCLinkWASM) isActive() bool {
	w.mtx.RLock()
	defer w.mtx.RUnlock()
	return w.active
}

func (w *webRTCLinkWASM) isOnline() bool {
	w.mtx.RLock()
	defer w.mtx.RUnlock()
	return w.online
}

func (w *webRTCLinkWASM) getLabel() string {
	w.mtx.RLock()
	defer w.mtx.RUnlock()
	return w.label
}

func (w *webRTCLinkWASM) getLocalSDP() (string, error) {
	type resCh struct {
		sdp string
		err error
	}
	c := make(chan resCh, 1)
	defer close(c)

	go func() {
		res := jsWrapper.Call("getLocalSDP", js.ValueOf(w.id)).Bool()
		if !res {
			c <- resCh{
				sdp: "",
				err: fmt.Errorf("failed to get local SDP: %s", w.getLastError()),
			}
			return
		}

		sdp := <-w.localSDPCh
		close(w.localSDPCh)
		w.localSDPCh = nil
		if sdp == "" {
			c <- resCh{
				sdp: "",
				err: fmt.Errorf("failed to get local SDP: %s", w.getLastError()),
			}
			return
		}

		c <- resCh{
			sdp: sdp,
			err: nil,
		}
	}()

	r := <-c
	return r.sdp, r.err
}

func (w *webRTCLinkWASM) setRemoteSDP(sdp string) error {
	c := make(chan error, 1)

	go func() {
		res := jsWrapper.Call("setRemoteSDP", js.ValueOf(w.id), js.ValueOf(sdp)).Bool()
		if !res {
			c <- fmt.Errorf("failed to set remote SDP: %s", w.getLastError())
		}
		c <- nil
	}()

	return <-c
}

func (w *webRTCLinkWASM) updateICE(ice string) error {
	c := make(chan error, 1)

	go func() {
		res := jsWrapper.Call("updateICE", js.ValueOf(w.id), js.ValueOf(ice)).Bool()
		if !res {
			c <- fmt.Errorf("failed to update ICE: %s", w.getLastError())
		}
		c <- nil
	}()

	return <-c
}

func (w *webRTCLinkWASM) send(data []byte) error {
	c := make(chan error, 1)

	go func() {
		buffer := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(buffer, data)
		res := jsWrapper.Call("send", js.ValueOf(w.id), buffer).Bool()
		if !res {
			c <- fmt.Errorf("failed to send data: %s", w.getLastError())
		}
		c <- nil
	}()

	return <-c
}

func (w *webRTCLinkWASM) disconnect() error {
	c := make(chan error, 1)

	defer func() {
		w.mtx.Lock()
		if w.active {
			o := w.online
			defer w.eventHandler.changeLinkState(false, o)
		}
		w.active = false
		w.mtx.Unlock()
	}()

	go func() {
		res := jsWrapper.Call("deleteLink", js.ValueOf(w.id)).Bool()
		if !res {
			c <- fmt.Errorf("failed to disconnect: %s", w.getLastError())
		}
		c <- nil
	}()

	return <-c
}

func getLink(id int) (*webRTCLinkWASM, bool) {
	linksMtx.Lock()
	defer linksMtx.Unlock()
	link, ok := links[id]
	return link, ok
}

func init() {
	jsWrapper = js.Global().Get("webrtcWrapper")
	if jsWrapper.IsUndefined() || jsWrapper.IsNull() {
		panic("webrtcWrapper is not defined")
	}

	jsWrapper.Set("onUpdateLinkState", js.FuncOf(jsOnUpdateLinkState))
	jsWrapper.Set("onUpdateICE", js.FuncOf(jsOnUpdateICE))
	jsWrapper.Set("onGetLocalSDP", js.FuncOf(jsOnGetLocalSDP))
	jsWrapper.Set("onReceiveData", js.FuncOf(jsOnReceiveData))
	jsWrapper.Set("onRaiseError", js.FuncOf(jsOnRaiseError))

	go func() {
		jsWrapper.Call("setup")
	}()

	defaultWebRTCLinkFactory = func(config *webRTCLinkConfig, eventHandler *webRTCLinkEventHandler) (webRTCLink, error) {
		return newWebRTCLinkWASM(config, eventHandler)
	}
}
