// +build js

package colonio

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"syscall/js"
	"time"
)

type colonioImpl struct {
	jsModule js.Value

	childrenMtx sync.Mutex
	maps        map[string]*mapImpl
	pubsub2ds   map[string]*pubsub2dImpl
}

type valueImpl struct {
	valueType int32
	value     js.Value
}

type mapImpl struct {
	jsModule js.Value
}

type pubsub2dImpl struct {
	jsModule  js.Value
	eventsMtx sync.Mutex
	events    map[string]uint32
}

const (
	jsModuleName    = "colonioSuite"
	valueTypeNull   = 0
	valueTypeBool   = 1
	valueTypeInt    = 2
	valueTypeDouble = 3
	valueTypeString = 4
)

var (
	jsSuite = js.Global().Get(jsModuleName)

	eventReceivers    = make(map[uint32]func(js.Value))
	eventReceiversMtx sync.Mutex
	respChannels      = make(map[uint32]chan js.Value)
	respChannelsMtx   sync.Mutex
)

func init() {
	rand.Seed(time.Now().UnixNano())
	jsSuite.Set("onEvent", js.FuncOf(onEvent))
	jsSuite.Set("onResponse", js.FuncOf(onResponse))
}

func checkJsError(v js.Value) error {
	if v.IsNull() || v.IsUndefined() {
		return nil
	}

	return newErr(uint32(v.Get("code").Int()), v.Get("message").String())
}

func assignEventReceiver(f func(js.Value)) (key uint32) {
	eventReceiversMtx.Lock()
	defer eventReceiversMtx.Unlock()

	for {
		key = rand.Uint32()
		if _, ok := eventReceivers[key]; !ok {
			eventReceivers[key] = f
			return
		}
	}
}

func deleteEventReceiver(key uint32) {
	eventReceiversMtx.Lock()
	defer eventReceiversMtx.Unlock()

	delete(eventReceivers, key)
}

func onEvent(_ js.Value, args []js.Value) interface{} {
	key := uint32(args[0].Int())
	eventReceiversMtx.Lock()
	eventReceiver, ok := eventReceivers[key]
	eventReceiversMtx.Unlock()
	if !ok {
		panic("event key does not match")
	}

	eventReceiver(args[1])
	return nil
}

func assignRespChannel() (key uint32, respChannel chan js.Value) {
	respChannelsMtx.Lock()
	defer respChannelsMtx.Unlock()

	for {
		key = rand.Uint32()
		if _, ok := respChannels[key]; !ok {
			respChannel = make(chan js.Value)
			respChannels[key] = respChannel
			return
		}
	}
}

func deleteRespChannel(key uint32) {
	respChannelsMtx.Lock()
	defer respChannelsMtx.Unlock()

	respChannel, ok := respChannels[key]
	if !ok {
		return
	}

	close(respChannel)
	delete(respChannels, key)
}

func onResponse(_ js.Value, args []js.Value) interface{} {
	key := uint32(args[0].Int())
	respChannelsMtx.Lock()
	defer respChannelsMtx.Unlock()
	respChannel, ok := respChannels[key]
	if !ok {
		panic("response key does not match")
	}

	if len(args) == 1 {
		respChannel <- js.Undefined()
	} else { // len(args) == 2
		respChannel <- args[1]
	}
	close(respChannel)
	delete(respChannels, key)
	return nil
}

// NewColonio creates a new instance of colonio object.
func NewColonio() (Colonio, error) {
	impl := &colonioImpl{
		jsModule:  jsSuite.Call("newColonio"),
		maps:      make(map[string]*mapImpl),
		pubsub2ds: make(map[string]*pubsub2dImpl),
	}

	return impl, nil
}

// Connect to seed and join the cluster.
func (c *colonioImpl) Connect(url, token string) error {
	key, respChannel := assignRespChannel()
	defer deleteRespChannel(key)

	c.jsModule.Call("connect", key, url, token)

	return checkJsError(<-respChannel)
}

// Disconnect from the cluster and the seed.
func (c *colonioImpl) Disconnect() error {
	key, respChannel := assignRespChannel()
	defer deleteRespChannel(key)

	c.jsModule.Call("disconnect", key)

	return checkJsError(<-respChannel)
}

// Get Map accessor associated with the name.
func (c *colonioImpl) AccessMap(name string) Map {
	c.childrenMtx.Lock()
	defer c.childrenMtx.Unlock()

	if impl, ok := c.maps[name]; ok {
		return impl
	}

	impl := &mapImpl{
		jsModule: c.jsModule.Call("accessMap", name),
	}
	c.maps[name] = impl

	return impl
}

// Get Pubsub2D accessor associated with the name.
func (c *colonioImpl) AccessPubsub2D(name string) Pubsub2D {
	c.childrenMtx.Lock()
	defer c.childrenMtx.Unlock()

	if impl, ok := c.pubsub2ds[name]; ok {
		return impl
	}

	impl := &pubsub2dImpl{
		jsModule: c.jsModule.Call("accessPubsub2D", name),
		events:   make(map[string]uint32),
	}
	c.pubsub2ds[name] = impl

	return impl
}

// Get the node-id of this node.
func (c *colonioImpl) GetLocalNid() string {
	return c.jsModule.Call("getLocalNid").String()
}

// Sets the current position of the node.
func (c *colonioImpl) SetPosition(x, y float64) (float64, float64, error) {
	key, respChannel := assignRespChannel()
	defer deleteRespChannel(key)

	c.jsModule.Call("setPosition", key, x, y)

	resp := <-respChannel
	newX := resp.Get("x").Float()
	newY := resp.Get("y").Float()
	err := checkJsError(resp.Get("err"))
	return newX, newY, err
}

// Release some resources used by colonio object.
func (c *colonioImpl) Quit() error {
	// release binded events for pubsub2d
	for _, p := range c.pubsub2ds {
		p.eventsMtx.Lock()
		for _, key := range p.events {
			deleteEventReceiver(key)
		}
		p.eventsMtx.Unlock()
	}

	return nil
}

func (v *valueImpl) IsNil() bool {
	return v.valueType == valueTypeNull
}

func (v *valueImpl) IsBool() bool {
	return v.valueType == valueTypeBool
}

func (v *valueImpl) IsInt() bool {
	return v.valueType == valueTypeInt
}

func (v *valueImpl) IsDouble() bool {
	return v.valueType == valueTypeDouble
}

func (v *valueImpl) IsString() bool {
	return v.valueType == valueTypeString
}

func (v *valueImpl) Set(val interface{}) error {
	if reflect.ValueOf(v).IsNil() {
		v.valueType = valueTypeNull
		v.value = js.Null()
		return nil
	}

	switch val := val.(type) {
	case bool:
		v.valueType = valueTypeBool
		v.value = js.ValueOf(val)
		return nil

	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
		v.valueType = valueTypeInt
		v.value = js.ValueOf(val)
		return nil

	case float32, float64:
		v.valueType = valueTypeDouble
		v.value = js.ValueOf(val)
		return nil

	case string:
		v.valueType = valueTypeString
		v.value = js.ValueOf(val)
		return nil

	case *valueImpl:
		*v = *val
		return nil
	}

	return fmt.Errorf("unsupported value type")
}

func (v *valueImpl) GetBool() (bool, error) {
	if v.valueType != valueTypeBool {
		return false, fmt.Errorf("type mismatch")
	}
	return v.value.Bool(), nil
}

func (v *valueImpl) GetInt() (int64, error) {
	if v.valueType != valueTypeInt {
		return 0, fmt.Errorf("type mismatch")
	}
	return int64(v.value.Int()), nil
}

func (v *valueImpl) GetDouble() (float64, error) {
	if v.valueType != valueTypeDouble {
		return 0.0, fmt.Errorf("type mismatch")
	}
	return v.value.Float(), nil
}

func (v *valueImpl) GetString() (string, error) {
	if v.valueType != valueTypeString {
		return "", fmt.Errorf("type mismatch")
	}
	return v.value.String(), nil
}

func (m *mapImpl) Get(key interface{}) (Value, error) {
	keyImpl := &valueImpl{}
	if err := keyImpl.Set(key); err != nil {
		return nil, err
	}

	rKey, respChannel := assignRespChannel()
	defer deleteRespChannel(rKey)

	m.jsModule.Call("get", rKey, keyImpl.valueType, js.ValueOf(keyImpl.value))

	resp := <-respChannel
	err := checkJsError(resp.Get("err"))
	if err != nil {
		return nil, err
	}

	return &valueImpl{
		valueType: int32(resp.Get("type").Int()),
		value:     resp.Get("value"),
	}, nil
}

func (m *mapImpl) Set(key, val interface{}, opt uint32) error {
	keyImpl := &valueImpl{}
	if err := keyImpl.Set(key); err != nil {
		return err
	}

	valImpl := &valueImpl{}
	if err := valImpl.Set(val); err != nil {
		return err
	}

	rKey, respChannel := assignRespChannel()
	defer deleteRespChannel(rKey)

	m.jsModule.Call("set", rKey, keyImpl.valueType, js.ValueOf(keyImpl.value), valImpl.valueType, js.ValueOf(valImpl.value), opt)

	return checkJsError(<-respChannel)
}

func (p *pubsub2dImpl) Publish(name string, x, y, r float64, val interface{}, opt uint32) error {
	valImpl := &valueImpl{}
	if err := valImpl.Set(val); err != nil {
		return err
	}

	key, respChannel := assignRespChannel()
	defer deleteRespChannel(key)

	p.jsModule.Call("publish", key, name, x, y, r, valImpl.valueType, js.ValueOf(valImpl.value), opt)

	return checkJsError(<-respChannel)
}

func (p *pubsub2dImpl) On(name string, cb func(Value)) {
	key := assignEventReceiver(func(v js.Value) {
		cb(&valueImpl{
			valueType: int32(v.Get("type").Int()),
			value:     v.Get("value"),
		})
	})

	p.eventsMtx.Lock()
	if oldKey, ok := p.events[name]; ok {
		deleteEventReceiver(oldKey)
	}
	p.events[name] = key
	p.eventsMtx.Unlock()

	p.jsModule.Call("on", key, name)
}

func (p *pubsub2dImpl) Off(name string) {
	p.eventsMtx.Lock()
	defer p.eventsMtx.Unlock()
	if key, ok := p.events[name]; ok {
		deleteEventReceiver(key)
	}

	p.jsModule.Call("off", name)
}
