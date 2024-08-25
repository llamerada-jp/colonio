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

package seed_accessor

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"syscall/js"
)

type response struct {
	data   []byte
	status int
	err    error
}

type seedTransportWASM struct {
	jsInstance js.Value
	mtx        sync.Mutex
	receivers  map[uint32]chan response
}

func NewSeedTransportWASM(opt *SeedTransporterOption) SeedTransporter {
	t := &seedTransportWASM{
		jsInstance: js.Global().Get("SeedTransport").New(),
		receivers:  make(map[uint32]chan response),
	}

	t.jsInstance.Set("onReceive", js.FuncOf(func(this js.Value, args []js.Value) any {
		return t.jsReceive(args)
	}))

	return t
}

// Send implements SeedTransporter.Send.
// This function sends data to the specified URL and returns the response. It processes synchronously.
// TODO: Cannot use context. Hint: js's AbortController, AbortSignal.
func (t *seedTransportWASM) Send(ctx context.Context, url string, data []byte) ([]byte, int, error) {
	id, receiver := t.assignReceiver()

	buffer := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(buffer, data)
	go func() {
		t.jsInstance.Call("send", id, url, buffer)
	}()

	response, ok := <-receiver
	if !ok {
		return nil, 0, fmt.Errorf("receiver closed")
	}

	return response.data, response.status, response.err
}

func (t *seedTransportWASM) jsReceive(args []js.Value) any {
	id := args[0].Int()
	buffer := args[1]
	status := args[2].Int()
	err := args[3].String()

	go func() {
		if err != "" {
			t.receive(uint32(id), nil, 0, fmt.Errorf(err))
			return
		}

		data := make([]byte, buffer.Get("byteLength").Int())
		js.CopyBytesToGo(data, buffer)

		t.receive(uint32(id), data, status, nil)
	}()

	return nil
}

func (t *seedTransportWASM) receive(id uint32, data []byte, status int, err error) {
	t.mtx.Lock()
	ch, ok := t.receivers[id]
	if !ok {
		//panic("receiver not found")
	}
	delete(t.receivers, id)
	t.mtx.Unlock()

	ch <- response{data, status, err}
	close(ch)
}

func (t *seedTransportWASM) assignReceiver() (uint32, chan response) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	// generate random unique id
	var id uint32
	for {
		id = rand.Uint32()
		if _, ok := t.receivers[id]; !ok {
			break
		}
	}

	// make receiver
	receiver := make(chan response, 1)
	t.receivers[id] = receiver

	return id, receiver
}

func init() {
	DefaultSeedTransporterFactory =
		func(opt *SeedTransporterOption) SeedTransporter {
			return NewSeedTransportWASM(opt)
		}
}
