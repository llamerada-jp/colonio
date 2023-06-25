//go:build js

package colonio

/*
 * Copyright 2017 Yuji Ito <llamerada.jp@gmail.com>
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

import (
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"sync"
	"syscall/js"
	"time"
)

type colonioImpl struct {
	colonioJ              js.Value
	messagingMutex        sync.Mutex
	messagingHandlerNames map[string]uint32
	spreadMutex           sync.Mutex
	spreadHandlerNames    map[string]uint32
}

type valueImpl struct {
	valueType int32
	valueJ    js.Value
}

type messagingResponseWriterImpl struct {
	writerJ js.Value
}

type kvsLocalDataImpl struct {
	cursorJ js.Value
	keys    []string
}

const (
	helperName      = "colonioGo"
	valueTypeNull   = 0
	valueTypeBool   = 1
	valueTypeInt    = 2
	valueTypeDouble = 3
	valueTypeString = 4
	valueTypeBinary = 5
)

var (
	helperJ             = js.Global().Get(helperName)
	errorEntryClass     = helperJ.Get("mod").Get("ErrorEntry")
	valueClass          = helperJ.Get("mod").Get("Value")
	eventReceiversMutex = sync.RWMutex{}
	eventReceivers      = make(map[uint32]func(js.Value))
	respChannelsMutex   = sync.Mutex{}
	respChannels        = make(map[uint32]chan js.Value)
)

func init() {
	rand.Seed(time.Now().UnixNano())
	helperJ.Set("onEvent", js.FuncOf(onEvent))
	helperJ.Set("onResponse", js.FuncOf(onResponse))
}

func checkJsError(v js.Value) error {
	if v.IsNull() || v.IsUndefined() {
		return nil
	}

	if !v.InstanceOf(errorEntryClass) {
		return nil
	}

	return newErr(uint32(v.Get("code").Int()), v.Get("message").String(),
		uint(v.Get("line").Int()), v.Get("file").String())
}

func assignEventReceiver(receiver func(js.Value)) (id uint32) {
	eventReceiversMutex.Lock()
	defer eventReceiversMutex.Unlock()

	for {
		id = rand.Uint32()
		if id == 0 {
			continue
		}
		if _, ok := eventReceivers[id]; !ok {
			eventReceivers[id] = receiver
			return
		}
	}
}

func deleteEventReceiver(id uint32) {
	eventReceiversMutex.Lock()
	defer eventReceiversMutex.Unlock()

	delete(eventReceivers, id)
}

func onEvent(_ js.Value, args []js.Value) interface{} {
	id := uint32(args[0].Int())
	eventReceiversMutex.RLock()
	receiver, ok := eventReceivers[id]
	eventReceiversMutex.RUnlock()
	if !ok {
		panic("event key does not match")
	}

	receiver(args[1])
	return nil
}

func assignRespChannel() (id uint32, respChannel chan js.Value) {
	respChannelsMutex.Lock()
	defer respChannelsMutex.Unlock()

	for {
		id = rand.Uint32()
		if _, ok := respChannels[id]; !ok {
			respChannel = make(chan js.Value)
			respChannels[id] = respChannel
			return
		}
	}
}

func deleteRespChannel(id uint32) {
	respChannelsMutex.Lock()
	defer respChannelsMutex.Unlock()

	respChannel, ok := respChannels[id]
	if !ok {
		return
	}

	close(respChannel)
	delete(respChannels, id)
}

func onResponse(_ js.Value, args []js.Value) interface{} {
	id := uint32(args[0].Int())
	respChannelsMutex.Lock()
	defer respChannelsMutex.Unlock()
	respChannel, ok := respChannels[id]
	if !ok {
		panic("response key does not match")
	}

	if len(args) == 1 {
		respChannel <- js.Undefined()
	} else { // len(args) == 2
		respChannel <- args[1]
	}
	close(respChannel)
	delete(respChannels, id)
	return nil
}

func NewConfig() *ColonioConfig {
	return &ColonioConfig{
		SeedSessionTimeoutMs:    30 * 1000,
		DisableSeedVerification: false,
		MaxUserThreads:          1,
		LoggerFunc: func(s string) {
			log.Println(s)
		},
	}
}

// NewColonio creates a new instance of colonio object.
func NewColonio(config *ColonioConfig) (Colonio, error) {
	impl := &colonioImpl{
		colonioJ: helperJ.Call("newColonio",
			js.ValueOf(config.SeedSessionTimeoutMs),
			js.ValueOf(config.DisableSeedVerification),
			js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				config.LoggerFunc(args[0].String())
				return nil
			})),
		messagingMutex:        sync.Mutex{},
		messagingHandlerNames: make(map[string]uint32),
		spreadMutex:           sync.Mutex{},
		spreadHandlerNames:    make(map[string]uint32),
	}

	return impl, nil
}

// Connect to seed and join the cluster.
func (ci *colonioImpl) Connect(url, token string) error {
	id, respChannel := assignRespChannel()
	defer deleteRespChannel(id)

	helperJ.Call("connect", ci.colonioJ, id, url, token)

	return checkJsError(<-respChannel)
}

// Disconnect from the cluster and the seed.
func (ci *colonioImpl) Disconnect() error {
	id, respChannel := assignRespChannel()
	defer deleteRespChannel(id)

	helperJ.Call("disconnect", ci.colonioJ, id)

	return checkJsError(<-respChannel)
}

func (ci *colonioImpl) IsConnected() bool {
	return ci.colonioJ.Call("isConnected").Bool()
}

// Get the node-id of this node.
func (ci *colonioImpl) GetLocalNid() string {
	return ci.colonioJ.Call("getLocalNid").String()
}

// Sets the current position of the node.
func (ci *colonioImpl) SetPosition(x, y float64) (float64, float64, error) {
	resp := ci.colonioJ.Call("setPosition", x, y)
	xG := resp.Get("x").Float()
	yG := resp.Get("y").Float()
	return xG, yG, checkJsError(resp)
}

func (wi *messagingResponseWriterImpl) Write(valueG interface{}) {
	valueJ, err := convertValueG2J(valueG)
	if err != nil {
		log.Fatalln(err)
		// return err
	}
	wi.writerJ.Call("write", valueJ)
}

func (ci *colonioImpl) MessagingPost(dst, name string, val interface{}, opt uint32) (Value, error) {
	vi := &valueImpl{}
	if err := vi.Set(val); err != nil {
		return nil, err
	}

	id, respChannel := assignRespChannel()
	defer deleteRespChannel(id)

	helperJ.Call("messagingPost", ci.colonioJ, id, dst, name, vi.valueType, vi.valueJ, opt)

	resp := <-respChannel
	if err := checkJsError(resp); err != nil {
		return nil, err
	}

	return convertValueJ2G(resp)
}

func (ci *colonioImpl) MessagingSetHandler(name string, handler func(*MessagingRequest, MessagingResponseWriter)) {
	ci.MessagingUnsetHandler(name)

	id := assignEventReceiver(func(v js.Value) {
		requestJ := v.Get("request")
		message, err := convertValueJ2G(requestJ.Get("message"))
		if err != nil {
			log.Fatal(err)
		}
		request := &MessagingRequest{
			SourceNid: requestJ.Get("sourceNid").String(),
			Message:   message,
			Options:   uint32(requestJ.Get("options").Int()),
		}

		var writer MessagingResponseWriter
		if !v.Get("writer").IsUndefined() {
			writer = &messagingResponseWriterImpl{
				writerJ: v.Get("writer"),
			}
		}
		go handler(request, writer)
	})

	ci.messagingMutex.Lock()
	defer ci.messagingMutex.Unlock()
	ci.messagingHandlerNames[name] = id

	helperJ.Call("messagingSetHandler", ci.colonioJ, id, name)
}

func (ci *colonioImpl) MessagingUnsetHandler(name string) {
	ci.messagingMutex.Lock()
	defer ci.messagingMutex.Unlock()

	id, ok := ci.messagingHandlerNames[name]
	if !ok {
		return
	}
	deleteEventReceiver(id)
	delete(ci.messagingHandlerNames, name)

	ci.colonioJ.Call("messagingUnsetHandler", name)
}

// kvs
func (kldi *kvsLocalDataImpl) GetKeys() []string {
	return kldi.keys
}

func (kldi *kvsLocalDataImpl) GetValue(key string) (Value, error) {
	valueJ := kldi.cursorJ.Call("getValue", key)
	return convertValueJ2G(valueJ)
}

func (kldi *kvsLocalDataImpl) Free() {
	kldi.cursorJ.Call("free")
}

func (ci *colonioImpl) KvsGetLocalData() KvsLocalData {
	id, respChannel := assignRespChannel()
	defer deleteRespChannel(id)

	helperJ.Call("kvsGetLocalData", ci.colonioJ, id)

	resp := <-respChannel
	if err := checkJsError(resp); err != nil {
		log.Fatal(err)
	}

	keysJ := resp.Call("getKeys")
	keysG := make([]string, keysJ.Length())
	for idx := 0; idx < keysJ.Length(); idx++ {
		keysG[idx] = keysJ.Index(idx).String()
	}

	return &kvsLocalDataImpl{
		cursorJ: resp,
		keys:    keysG,
	}
}

func (ci *colonioImpl) KvsGet(key string) (Value, error) {
	id, respChannel := assignRespChannel()
	defer deleteRespChannel(id)

	helperJ.Call("kvsGet", ci.colonioJ, id, key)

	resp := <-respChannel
	if err := checkJsError(resp); err != nil {
		return nil, err
	}

	return convertValueJ2G(resp)
}

func (ci *colonioImpl) KvsSet(key string, valueG interface{}, opt uint32) error {
	valueJ, err := convertValueG2J(valueG)
	if err != nil {
		return err
	}

	id, respChannel := assignRespChannel()
	defer deleteRespChannel(id)

	helperJ.Call("kvsSet", ci.colonioJ, id, key, valueJ, opt)

	return checkJsError(<-respChannel)
}

// spread
func (ci *colonioImpl) SpreadPost(x, y, r float64, name string, messageG interface{}, opt uint32) error {
	messageJ, err := convertValueG2J(messageG)
	if err != nil {
		return err
	}

	id, respChannel := assignRespChannel()
	defer deleteRespChannel(id)

	helperJ.Call("spreadPost", ci.colonioJ, id, x, y, r, name, messageJ, opt)

	return checkJsError(<-respChannel)
}

func (ci *colonioImpl) SpreadSetHandler(name string, handler func(*SpreadRequest)) {
	ci.SpreadUnsetHandler(name)

	id := assignEventReceiver(func(v js.Value) {
		message, err := convertValueJ2G(v.Get("message"))
		if err != nil {
			log.Fatal(err)
		}
		request := &SpreadRequest{
			SourceNid: v.Get("sourceNid").String(),
			Message:   message,
			Options:   uint32(v.Get("options").Int()),
		}
		go handler(request)
	})

	ci.spreadMutex.Lock()
	defer ci.spreadMutex.Unlock()
	ci.spreadHandlerNames[name] = id

	helperJ.Call("spreadSetHandler", ci.colonioJ, id, name)
}

func (ci *colonioImpl) SpreadUnsetHandler(name string) {
	ci.spreadMutex.Lock()
	defer ci.spreadMutex.Unlock()

	id, ok := ci.spreadHandlerNames[name]
	if !ok {
		return
	}
	deleteEventReceiver(id)
	delete(ci.spreadHandlerNames, name)

	ci.colonioJ.Call("spreadUnsetHandler", name)
}

func NewValue(v interface{}) (Value, error) {
	val := &valueImpl{}
	if err := val.Set(v); err != nil {
		return nil, err
	}
	return val, nil
}

func convertValueJ2G(valueJ js.Value) (Value, error) {
	if !valueJ.InstanceOf(valueClass) {
		return nil, fmt.Errorf("instance is not value class")
	}

	return &valueImpl{
		valueType: int32(valueJ.Call("getType").Int()),
		valueJ:    valueJ.Call("getJsValue"),
	}, nil
}

func convertValueG2J(valueG interface{}) (js.Value, error) {
	switch v := valueG.(type) {
	case bool:
		return helperJ.Call("newValue", valueTypeBool, v), nil

	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
		return helperJ.Call("newValue", valueTypeInt, v), nil

	case float32, float64:
		return helperJ.Call("newValue", valueTypeDouble, v), nil

	case string:
		return helperJ.Call("newValue", valueTypeString, v), nil

	case []byte:
		arrayBuf := js.Global().Get("ArrayBuffer").New(len(v))
		u8array := js.Global().Get("Uint8Array").New(arrayBuf)
		js.CopyBytesToJS(u8array, v)
		return helperJ.Call("newValue", valueTypeBinary, arrayBuf), nil

	case *valueImpl:
		return helperJ.Call("newValue", v.valueType, v.valueJ), nil
	}

	if valueG == nil || reflect.ValueOf(valueG).IsNil() {
		return helperJ.Call("newValue", valueTypeNull, js.Null()), nil
	}

	return js.Null(), fmt.Errorf("unsupported value type")
}

func (vi *valueImpl) IsNil() bool {
	return vi.valueType == valueTypeNull
}

func (vi *valueImpl) IsBool() bool {
	return vi.valueType == valueTypeBool
}

func (vi *valueImpl) IsInt() bool {
	return vi.valueType == valueTypeInt
}

func (vi *valueImpl) IsDouble() bool {
	return vi.valueType == valueTypeDouble
}

func (vi *valueImpl) IsString() bool {
	return vi.valueType == valueTypeString
}

func (vi *valueImpl) IsBinary() bool {
	return vi.valueType == valueTypeBinary
}

func (vi *valueImpl) Set(val interface{}) error {
	switch val := val.(type) {
	case bool:
		vi.valueType = valueTypeBool
		vi.valueJ = js.ValueOf(val)
		return nil

	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
		vi.valueType = valueTypeInt
		vi.valueJ = js.ValueOf(val)
		return nil

	case float32, float64:
		vi.valueType = valueTypeDouble
		vi.valueJ = js.ValueOf(val)
		return nil

	case string:
		vi.valueType = valueTypeString
		vi.valueJ = js.ValueOf(val)
		return nil

	case []byte:
		arrayBuf := js.Global().Get("ArrayBuffer").New(len(val))
		u8array := js.Global().Get("Uint8Array").New(arrayBuf)
		js.CopyBytesToJS(u8array, val)
		vi.valueType = valueTypeBinary
		vi.valueJ = arrayBuf
		return nil

	case *valueImpl:
		*vi = *val
		return nil
	}

	if val == nil || reflect.ValueOf(val).IsNil() {
		vi.valueType = valueTypeNull
		vi.valueJ = js.Null()
		return nil
	}

	return fmt.Errorf("unsupported value type")
}

func (vi *valueImpl) GetBool() (bool, error) {
	if vi.valueType != valueTypeBool {
		return false, fmt.Errorf("type mismatch")
	}
	return vi.valueJ.Bool(), nil
}

func (vi *valueImpl) GetInt() (int64, error) {
	if vi.valueType != valueTypeInt {
		return 0, fmt.Errorf("type mismatch")
	}
	return int64(vi.valueJ.Int()), nil
}

func (vi *valueImpl) GetDouble() (float64, error) {
	if vi.valueType != valueTypeDouble {
		return 0.0, fmt.Errorf("type mismatch")
	}
	return vi.valueJ.Float(), nil
}

func (vi *valueImpl) GetString() (string, error) {
	if vi.valueType != valueTypeString {
		return "", fmt.Errorf("type mismatch")
	}
	return vi.valueJ.String(), nil
}

func (vi *valueImpl) GetBinary() ([]byte, error) {
	if vi.valueType != valueTypeBinary {
		return nil, fmt.Errorf("type mismatch")
	}

	u8array := js.Global().Get("Uint8Array").New(vi.valueJ)
	buf := make([]byte, u8array.Get("byteLength").Int())
	js.CopyBytesToGo(buf, u8array)

	return buf, nil
}
