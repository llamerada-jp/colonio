//go:build !js

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

/*
#cgo darwin LDFLAGS: -framework Foundation
#cgo LDFLAGS: -lcolonio -lwebrtc -lm -lprotobuf -lstdc++
#cgo linux pkg-config: openssl
#cgo darwin LDFLAGS: -L/usr/local/opt/openssl/lib -lcrypto -lssl

#include "../../src/colonio/colonio.h"

extern const unsigned int cgo_colonio_nid_length;

// colonio
colonio_error_t *cgo_colonio_init(colonio_t *colonio);
colonio_error_t *cgo_colonio_connect(colonio_t *colonio, _GoString_ url, _GoString_ token);
colonio_map_t cgo_colonio_access_map(colonio_t *colonio, _GoString_ name);
colonio_pubsub_2d_t cgo_colonio_access_pubsub_2d(colonio_t *colonio, _GoString_ name);

// value
void cgo_colonio_value_set_string(colonio_value_t *value, _GoString_ s);

// pubsub
colonio_error_t *cgo_colonio_pubsub_2d_publish(
    colonio_pubsub_2d_t *pubsub_2d, _GoString_ name, double x, double y, double r, const colonio_value_t *value,
    uint32_t opt);
void cgo_cb_colonio_pubsub_2d_on(colonio_pubsub_2d_t *pubsub_2d, void *ptr, const colonio_value_t *val);
void cgo_colonio_pubsub_2d_on(colonio_pubsub_2d_t *pubsub_2d, _GoString_ name, void *ptr);
void cgo_colonio_pubsub_2d_off(colonio_pubsub_2d_t *pubsub_2d, _GoString_ name);
*/
import "C"

import (
	"fmt"
	"log"
	"reflect"
	"sync"
	"unsafe"
)

type colonioImpl struct {
	cInstance     C.struct_colonio_s
	mapCache      map[string]*mapImpl
	pubsub2DCache map[string]*pubsub2dImpl
}

type valueImpl struct {
	valueType C.enum_COLONIO_VALUE_TYPE
	vBool     bool
	vInt      int64
	vDouble   float64
	vString   string
}

type mapImpl struct {
	cInstance C.struct_colonio_map_s
}

type pubsub2dImpl struct {
	cInstance C.struct_colonio_pubsub_2d_s
	cbMutex   sync.RWMutex
	cbMap     map[*string]func(Value)
}

type defaultLogger struct {
}

// DefaultLogger is the default log output module that outputs logs to the golang log module.
var DefaultLogger *defaultLogger
var loggerMap map[*C.struct_colonio_s]Logger
var pubsub2DMutex sync.RWMutex
var pubsub2DMap map[*C.struct_colonio_pubsub_2d_s]*pubsub2dImpl

func init() {
	loggerMap = make(map[*C.struct_colonio_s]Logger)
	pubsub2DMutex = sync.RWMutex{}
	pubsub2DMap = make(map[*C.struct_colonio_pubsub_2d_s]*pubsub2dImpl)
}

func convertError(err *C.struct_colonio_error_s) error {
	return newErr(uint32(err.code), C.GoString(err.message))
}

// NewColonio creates a new initialized instance.
func NewColonio(logger Logger) (Colonio, error) {
	instance := &colonioImpl{
		mapCache:      make(map[string]*mapImpl),
		pubsub2DCache: make(map[string]*pubsub2dImpl),
	}
	loggerMap[&instance.cInstance] = logger

	err := C.cgo_colonio_init(&instance.cInstance)
	if err != nil {
		return nil, convertError(err)
	}

	go C.colonio_start_on_event_thread(&instance.cInstance)

	return instance, nil
}

//export cgoCbColonioLogger
func cgoCbColonioLogger(cInstancePtr *C.struct_colonio_s, messagePtr unsafe.Pointer, len C.int) {
	logger, ok := loggerMap[cInstancePtr]

	if !ok {
		log.Fatal("logger not found or deallocated yet")
	}

	logger.Output(C.GoStringN((*C.char)(messagePtr), len))
}

// Connect to seed and join the cluster.
func (c *colonioImpl) Connect(url, token string) error {
	err := C.cgo_colonio_connect(&c.cInstance, url, token)
	if err != nil {
		return convertError(err)
	}
	return nil
}

// Disconnect from the cluster and the seed.
func (c *colonioImpl) Disconnect() error {
	err := C.colonio_disconnect(&c.cInstance)
	if err != nil {
		return convertError(err)
	}
	return nil
}

func (c *colonioImpl) AccessMap(name string) Map {
	if ret, ok := c.mapCache[name]; ok {
		return ret
	}

	instance := &mapImpl{
		cInstance: C.cgo_colonio_access_map(&c.cInstance, name),
	}

	c.mapCache[name] = instance
	return instance
}

func (c *colonioImpl) AccessPubsub2D(name string) Pubsub2D {
	if ret, ok := c.pubsub2DCache[name]; ok {
		return ret
	}

	instance := &pubsub2dImpl{
		cInstance: C.cgo_colonio_access_pubsub_2d(&c.cInstance, name),
		cbMutex:   sync.RWMutex{},
		cbMap:     make(map[*string]func(Value)),
	}
	pubsub2DMutex.Lock()
	defer pubsub2DMutex.Unlock()
	if _, ok := pubsub2DMap[&instance.cInstance]; ok {

	} else {
		pubsub2DMap[&instance.cInstance] = instance
	}

	c.pubsub2DCache[name] = instance
	return instance
}

func (c *colonioImpl) GetLocalNid() string {
	buf := make([]byte, C.cgo_colonio_nid_length+1)
	data := (*reflect.SliceHeader)(unsafe.Pointer(&buf)).Data
	C.colonio_get_local_nid(&c.cInstance, (*C.char)(unsafe.Pointer(data)), nil)
	return string(buf)
}

func (c *colonioImpl) SetPosition(x, y float64) (float64, float64, error) {
	cX := C.double(x)
	cY := C.double(y)
	err := C.colonio_set_position(&c.cInstance, &cX, &cY)
	if err != nil {
		return 0, 0, convertError(err)
	}
	return float64(cX), float64(cY), nil
}

// Quit is the finalizer of the instance.
func (c *colonioImpl) Quit() error {
	err := C.colonio_quit(&c.cInstance)
	if err != nil {
		return convertError(err)
	}
	delete(loggerMap, &c.cInstance)
	return nil
}

func (l *defaultLogger) Output(message string) {
	log.Println(message)
}

func newValue(cValue *C.struct_colonio_value_s) Value {
	valueType := C.enum_COLONIO_VALUE_TYPE(C.colonio_value_get_type(cValue))
	switch valueType {
	case C.COLONIO_VALUE_TYPE_BOOL:
		return &valueImpl{
			valueType: valueType,
			vBool:     bool(C.colonio_value_get_bool(cValue)),
		}

	case C.COLONIO_VALUE_TYPE_INT:
		return &valueImpl{
			valueType: valueType,
			vInt:      int64(C.colonio_value_get_int(cValue)),
		}

	case C.COLONIO_VALUE_TYPE_DOUBLE:
		return &valueImpl{
			valueType: valueType,
			vDouble:   float64(C.colonio_value_get_double(cValue)),
		}

	case C.COLONIO_VALUE_TYPE_STRING:
		buf := make([]byte, uint(C.colonio_value_get_string_siz(cValue)))
		data := (*reflect.SliceHeader)(unsafe.Pointer(&buf)).Data
		C.colonio_value_get_string(cValue, (*C.char)(unsafe.Pointer(data)))
		return &valueImpl{
			valueType: valueType,
			vString:   string(buf),
		}

	default:
		return &valueImpl{
			valueType: valueType,
		}
	}
}

// NewValue create a value instance managed by colonio
func NewValue(v interface{}) (Value, error) {
	val := &valueImpl{}
	err := val.Set(v)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (v *valueImpl) IsNil() bool {
	return v.valueType == C.COLONIO_VALUE_TYPE_NULL
}

func (v *valueImpl) IsBool() bool {
	return v.valueType == C.COLONIO_VALUE_TYPE_BOOL
}

func (v *valueImpl) IsInt() bool {
	return v.valueType == C.COLONIO_VALUE_TYPE_INT
}

func (v *valueImpl) IsDouble() bool {
	return v.valueType == C.COLONIO_VALUE_TYPE_DOUBLE
}

func (v *valueImpl) IsString() bool {
	return v.valueType == C.COLONIO_VALUE_TYPE_STRING
}

func (v *valueImpl) Set(val interface{}) error {
	v.vString = ""

	if reflect.ValueOf(v).IsNil() {
		v.valueType = C.COLONIO_VALUE_TYPE_NULL
		return nil
	}

	switch val := val.(type) {
	case bool:
		v.valueType = C.COLONIO_VALUE_TYPE_BOOL
		v.vBool = val
		return nil

	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
		v.valueType = C.COLONIO_VALUE_TYPE_INT
		v.vInt = val.(int64)
		return nil

	case float32, float64:
		v.valueType = C.COLONIO_VALUE_TYPE_DOUBLE
		v.vDouble = val.(float64)
		return nil

	case string:
		v.valueType = C.COLONIO_VALUE_TYPE_STRING
		v.vString = val
		return nil

	case *valueImpl:
		*v = *val
		return nil
	}

	return fmt.Errorf("unsupported value type")
}

func (v *valueImpl) GetBool() (bool, error) {
	if v.valueType != C.COLONIO_VALUE_TYPE_BOOL {
		return false, fmt.Errorf("the type of value is wrong")
	}
	return v.vBool, nil
}

func (v *valueImpl) GetInt() (int64, error) {
	if v.valueType != C.COLONIO_VALUE_TYPE_INT {
		return 0, fmt.Errorf("the type of value is wrong")
	}
	return v.vInt, nil
}

func (v *valueImpl) GetDouble() (float64, error) {
	if v.valueType != C.COLONIO_VALUE_TYPE_DOUBLE {
		return 0, fmt.Errorf("the type of value is wrong")
	}
	return v.vDouble, nil
}

func (v *valueImpl) GetString() (string, error) {
	if v.valueType != C.COLONIO_VALUE_TYPE_STRING {
		return "", fmt.Errorf("the type of value is wrong")
	}
	return v.vString, nil
}

func writeOut(cValue *C.struct_colonio_value_s, value Value) {
	if value.IsBool() {
		v, _ := value.GetBool()
		C.colonio_value_set_bool(cValue, C.bool(v))
		return
	}

	if value.IsInt() {
		v, _ := value.GetInt()
		C.colonio_value_set_int(cValue, C.int64_t(v))
		return
	}

	if value.IsDouble() {
		v, _ := value.GetDouble()
		C.colonio_value_set_double(cValue, C.double(v))
		return
	}

	if value.IsString() {
		v, _ := value.GetString()
		C.cgo_colonio_value_set_string(cValue, v)
		return
	}

	C.colonio_value_free(cValue)
}

func (m *mapImpl) Get(key interface{}) (Value, error) {
	// key
	vKey, err := NewValue(key)
	if err != nil {
		return nil, err
	}
	cKey := C.struct_colonio_value_s{}
	C.colonio_value_init(&cKey)
	defer C.colonio_value_free(&cKey)
	writeOut(&cKey, vKey)

	// value
	cVal := C.struct_colonio_value_s{}
	C.colonio_value_init(&cVal)
	defer C.colonio_value_free(&cVal)

	// get
	cErr := C.colonio_map_get(&m.cInstance, &cKey, &cVal)
	if cErr != nil {
		return nil, convertError(cErr)
	}

	return newValue(&cVal), nil
}

func (m *mapImpl) Set(key, val interface{}, opt uint32) error {
	// key
	vKey, err := NewValue(key)
	if err != nil {
		return err
	}
	cKey := C.struct_colonio_value_s{}
	C.colonio_value_init(&cKey)
	defer C.colonio_value_free(&cKey)
	writeOut(&cKey, vKey)

	// value
	vValue, err := NewValue(val)
	if err != nil {
		return err
	}
	cVal := C.struct_colonio_value_s{}
	C.colonio_value_init(&cVal)
	defer C.colonio_value_free(&cVal)
	writeOut(&cVal, vValue)

	// set
	cErr := C.colonio_map_set(&m.cInstance, &cKey, &cVal, C.uint32_t(opt))
	if cErr != nil {
		return convertError(cErr)
	}
	return nil
}

func (p *pubsub2dImpl) Publish(name string, x, y, r float64, val interface{}, opt uint32) error {
	// value
	vValue, err := NewValue(val)
	if err != nil {
		return err
	}
	cVal := C.struct_colonio_value_s{}
	C.colonio_value_init(&cVal)
	defer C.colonio_value_free(&cVal)
	writeOut(&cVal, vValue)

	// publish
	cErr := C.cgo_colonio_pubsub_2d_publish(&p.cInstance, name, C.double(x), C.double(y), C.double(r), &cVal, C.uint32_t(opt))
	if cErr != nil {
		return convertError(cErr)
	}
	return nil
}

//export cgoCbPubsub2DOn
func cgoCbPubsub2DOn(cInstancePtr *C.struct_colonio_pubsub_2d_s, ptr unsafe.Pointer, cVal *C.struct_colonio_value_s) {
	var ps2 *pubsub2dImpl
	{
		pubsub2DMutex.RLock()
		defer pubsub2DMutex.RUnlock()
		if p, ok := pubsub2DMap[cInstancePtr]; ok {
			ps2 = p
		} else {
			return
		}
	}

	var cb func(Value)
	{
		ps2.cbMutex.RLock()
		defer ps2.cbMutex.RUnlock()
		if c, ok := ps2.cbMap[(*string)(ptr)]; ok {
			cb = c
		} else {
			return
		}
	}
	value := newValue(cVal)
	cb(value)
}

func (p *pubsub2dImpl) On(name string, cb func(Value)) {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	p.cbMap[&name] = cb
	C.cgo_colonio_pubsub_2d_on(&p.cInstance, name, unsafe.Pointer(&name))
}

func (p *pubsub2dImpl) Off(name string) {
	C.cgo_colonio_pubsub_2d_off(&p.cInstance, name)

	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()

	for s := range p.cbMap {
		if *s == name {
			delete(p.cbMap, s)
			return
		}
	}
}
