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
#cgo LDFLAGS: -lcolonio -lwebrtc -lm -lprotobuf -lstdc++
#cgo darwin LDFLAGS: -framework Foundation
#cgo darwin LDFLAGS: -L/usr/local/opt/openssl/lib -lcrypto -lssl
#cgo linux pkg-config: openssl

#include <stdlib.h>
#include "../../src/colonio/colonio.h"

extern const unsigned int cgo_colonio_nid_length;
extern const colonio_messaging_writer_t cgo_colonio_messaging_writer_none;

typedef void* colonio_t;
typedef void* colonio_value_t;

// colonio
colonio_error_t* cgo_colonio_init(colonio_t* colonio);
colonio_error_t* cgo_colonio_connect(colonio_t colonio, _GoString_ url, _GoString_ token);
int cgo_colonio_is_connected(colonio_t colonio);

// messaging
colonio_error_t* cgo_colonio_messaging_post(colonio_t colonio, _GoString_ dst, _GoString_ name, colonio_const_value_t val, uint32_t opt, colonio_value_t* result);
void cgo_colonio_messaging_set_handler(colonio_t colonio, _GoString_ name, unsigned long id);
void cgo_colonio_messaging_unset_handler(colonio_t colonio, _GoString_ name);

// value
void cgo_colonio_value_set_string(colonio_value_t value, _GoString_ s);

// kvs
colonio_error_t* cgo_colonio_kvs_get(colonio_t colonio, _GoString_ key, colonio_value_t* dst);
colonio_error_t* cgo_colonio_kvs_set(colonio_t colonio, _GoString_ key, colonio_const_value_t value, uint32_t opt);

// spread
colonio_error_t* cgo_colonio_spread_post(
    colonio_t colonio, double x, double y, double r, _GoString_ name, colonio_const_value_t value,
    uint32_t opt);
void cgo_colonio_spread_set_handler(colonio_t colonio, _GoString_ name, unsigned long id);
void cgo_colonio_spread_unset_handler(colonio_t colonio, _GoString_ name);
*/
import "C"

import (
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"sync"
	"unsafe"
)

type colonioImpl struct {
	colonioC              C.colonio_t
	loggerFunc            func(string)
	messagingMutex        sync.RWMutex
	messagingHandlers     map[uint32]func(*MessagingRequest, MessagingResponseWriter)
	messagingHandlerNames map[string]uint32
	spreadMutex           sync.RWMutex
	spreadHandlers        map[uint32]func(*SpreadRequest)
	spreadHandlerNames    map[string]uint32
}

type valueImpl struct {
	valueTypeC C.enum_COLONIO_VALUE_TYPE
	boolG      bool
	intG       int64
	doubleG    float64
	stringG    string
	binaryG    []byte
}

type messagingWriterImpl struct {
	colonioC C.colonio_t
	writerC  C.colonio_messaging_writer_t
}

type kvsLocalDataImpl struct {
	colonioC C.colonio_t
	cursorC  C.colonio_kvs_data_t
	keys     []string
	keyMap   map[string]C.uint
}

var colonioMap map[C.colonio_t]*colonioImpl

func init() {
	colonioMap = make(map[C.colonio_t]*colonioImpl)
}

func convertError(errC *C.struct_colonio_error_s) error {
	if errC == nil {
		return nil
	}
	return newErr(uint32(errC.code), C.GoStringN(errC.message, C.int(errC.message_siz)),
		uint(errC.line), C.GoStringN(errC.file, C.int(errC.file_siz)))
}

func NewConfig() *ColonioConfig {
	return &ColonioConfig{
		LoggerFunc: func(s string) {
			log.Println(s)
		},
	}
}

// NewColonio creates a new initialized instance.
func NewColonio(config *ColonioConfig) (Colonio, error) {
	impl := &colonioImpl{
		loggerFunc:            config.LoggerFunc,
		messagingMutex:        sync.RWMutex{},
		messagingHandlers:     make(map[uint32]func(*MessagingRequest, MessagingResponseWriter)),
		messagingHandlerNames: make(map[string]uint32),
		spreadMutex:           sync.RWMutex{},
		spreadHandlers:        make(map[uint32]func(*SpreadRequest)),
		spreadHandlerNames:    make(map[string]uint32),
	}

	errC := C.cgo_colonio_init(&impl.colonioC)
	if errC != nil {
		return nil, convertError(errC)
	}

	colonioMap[impl.colonioC] = impl

	return impl, nil
}

//export cgoWrapColonioLogger
func cgoWrapColonioLogger(colonioC C.colonio_t, messageC unsafe.Pointer, siz C.int) {
	colonio, ok := colonioMap[colonioC]
	messageG := C.GoStringN((*C.char)(messageC), siz)

	if !ok {
		log.Println(messageG)
	}

	go colonio.loggerFunc(messageG)
}

// Connect to seed and join the cluster.
func (ci *colonioImpl) Connect(url, token string) error {
	errC := C.cgo_colonio_connect(ci.colonioC, url, token)
	return convertError(errC)
}

// Disconnect from the cluster and the seed.
func (ci *colonioImpl) Disconnect() error {
	errC := C.colonio_disconnect(ci.colonioC)
	err := convertError(errC)
	if err != nil {
		return nil
	}

	defer delete(colonioMap, ci.colonioC)
	errC = C.colonio_quit(&ci.colonioC)
	return convertError(errC)
}

func (ci *colonioImpl) IsConnected() bool {
	return C.cgo_colonio_is_connected(ci.colonioC) != C.int(0)
}

func (ci *colonioImpl) GetLocalNid() string {
	buf := make([]byte, C.cgo_colonio_nid_length+1)
	data := (*reflect.SliceHeader)(unsafe.Pointer(&buf)).Data
	C.colonio_get_local_nid(ci.colonioC, (*C.char)(unsafe.Pointer(data)))
	return string(buf[:C.cgo_colonio_nid_length])
}

func (ci *colonioImpl) SetPosition(xG, yG float64) (float64, float64, error) {
	xC := C.double(xG)
	yC := C.double(yG)
	errC := C.colonio_set_position(ci.colonioC, &xC, &yC)
	if errC != nil {
		return 0, 0, convertError(errC)
	}
	return float64(xC), float64(yC), nil
}

func (ci *colonioImpl) MessagingPost(dst, name string, messageG interface{}, opt uint32) (Value, error) {
	// value
	message, err := NewValue(messageG)
	if err != nil {
		return nil, err
	}
	var messageC C.colonio_value_t
	C.colonio_value_create(&messageC)
	defer C.colonio_value_free(&messageC)
	writeOut(messageC, message)

	// result
	var resultC C.colonio_value_t
	defer C.colonio_value_free(&resultC)

	errC := C.cgo_colonio_messaging_post(ci.colonioC, dst, name, C.colonio_const_value_t(messageC), C.uint32_t(opt), &resultC)
	if errC != nil {
		return nil, convertError(errC)
	}

	return newValue(C.colonio_const_value_t(resultC)), nil
}

func (mwi *messagingWriterImpl) Write(valueG interface{}) {
	value, err := NewValue(valueG)
	if err != nil {
		log.Fatalln(err)
	}

	var valueC C.colonio_value_t
	C.colonio_value_create(&valueC)
	defer C.colonio_value_free(&valueC)
	writeOut(valueC, value)

	C.colonio_messaging_response_writer(mwi.colonioC, mwi.writerC, C.colonio_const_value_t(valueC))
}

//export cgoWrapMessagingHandler
func cgoWrapMessagingHandler(colonioC C.colonio_t, id C.ulong, src unsafe.Pointer, messageC C.colonio_const_value_t, opt C.uint32_t, writerC C.colonio_messaging_writer_t) {
	colonio, ok := colonioMap[colonioC]
	if !ok {
		log.Fatalln("colonioImpl not found")
	}

	request := &MessagingRequest{
		SourceNid: C.GoStringN((*C.char)(src), C.int(C.cgo_colonio_nid_length)),
		Message:   newValue(messageC),
		Options:   uint32(opt),
	}

	colonio.messagingMutex.RLock()
	defer colonio.messagingMutex.RUnlock()

	handler, ok := colonio.messagingHandlers[uint32(id)]
	if !ok {
		log.Fatalln("messaging handler not found")
	}

	var writer MessagingResponseWriter
	if writerC != C.cgo_colonio_messaging_writer_none {
		writer = &messagingWriterImpl{
			colonioC: colonioC,
			writerC:  writerC,
		}
	}

	go handler(request, writer)
}

func (ci *colonioImpl) MessagingSetHandler(name string, handler func(*MessagingRequest, MessagingResponseWriter)) {
	ci.MessagingUnsetHandler(name)

	ci.messagingMutex.Lock()
	defer ci.messagingMutex.Unlock()

	var id uint32
	for {
		id = rand.Uint32()
		if _, ok := ci.messagingHandlers[id]; !ok {
			break
		}
	}
	ci.messagingHandlers[id] = handler
	ci.messagingHandlerNames[name] = id

	C.cgo_colonio_messaging_set_handler(ci.colonioC, name, C.ulong(id))
}

func (ci *colonioImpl) MessagingUnsetHandler(name string) {
	ci.messagingMutex.Lock()
	defer ci.messagingMutex.Unlock()

	id, ok := ci.messagingHandlerNames[name]
	// handler not found
	if !ok {
		return
	}
	delete(ci.messagingHandlers, id)
	delete(ci.messagingHandlerNames, name)

	C.cgo_colonio_messaging_unset_handler(ci.colonioC, name)
}

func newValue(valueC C.colonio_const_value_t) Value {
	valueTypeC := C.enum_COLONIO_VALUE_TYPE(C.colonio_value_get_type(valueC))
	switch valueTypeC {
	case C.COLONIO_VALUE_TYPE_BOOL:
		return &valueImpl{
			valueTypeC: valueTypeC,
			boolG:      bool(C.colonio_value_get_bool(valueC)),
		}

	case C.COLONIO_VALUE_TYPE_INT:
		return &valueImpl{
			valueTypeC: valueTypeC,
			intG:       int64(C.colonio_value_get_int(valueC)),
		}

	case C.COLONIO_VALUE_TYPE_DOUBLE:
		return &valueImpl{
			valueTypeC: valueTypeC,
			doubleG:    float64(C.colonio_value_get_double(valueC)),
		}

	case C.COLONIO_VALUE_TYPE_STRING:
		var siz C.uint
		ptr := C.colonio_value_get_string(valueC, &siz)
		return &valueImpl{
			valueTypeC: valueTypeC,
			stringG:    C.GoStringN(ptr, C.int(siz)),
		}

	case C.COLONIO_VALUE_TYPE_BINARY:
		var siz C.uint
		ptr := C.colonio_value_get_binary(valueC, &siz)
		return &valueImpl{
			valueTypeC: valueTypeC,
			binaryG:    C.GoBytes(ptr, C.int(siz)),
		}

	default:
		return &valueImpl{
			valueTypeC: valueTypeC,
		}
	}
}

// kvs
func (kldi *kvsLocalDataImpl) GetKeys() []string {
	return kldi.keys
}

func (kldi *kvsLocalDataImpl) GetValue(key string) (Value, error) {
	idx, ok := kldi.keyMap[key]
	if !ok {
		return nil, fmt.Errorf("key %s not found", key)
	}
	var valueC C.colonio_value_t
	C.colonio_kvs_local_data_get_value(kldi.colonioC, kldi.cursorC, idx, &valueC)
	return newValue(C.colonio_const_value_t(valueC)), nil
}

func (kldi *kvsLocalDataImpl) Free() {
	C.colonio_kvs_local_data_free(kldi.colonioC, kldi.cursorC)
}

func (ci *colonioImpl) KvsGetLocalData() KvsLocalData {
	cursorC := C.colonio_kvs_get_local_data(ci.colonioC)
	dataCount := C.colonio_kvs_local_data_get_siz(ci.colonioC, cursorC)

	keys := make([]string, dataCount)
	keyMap := make(map[string]C.uint)
	for idx := C.uint(0); idx < dataCount; idx++ {
		var keySiz C.uint
		ptr := C.colonio_kvs_local_data_get_key(ci.colonioC, cursorC, idx, &keySiz)
		key := C.GoStringN(ptr, C.int(keySiz))
		keys[idx] = key
		keyMap[key] = idx
	}

	return &kvsLocalDataImpl{
		colonioC: ci.colonioC,
		cursorC:  cursorC,
		keys:     keys,
		keyMap:   keyMap,
	}
}

func (ci *colonioImpl) KvsGet(key string) (Value, error) {
	var valueC C.colonio_value_t

	// get
	errC := C.cgo_colonio_kvs_get(ci.colonioC, key, &valueC)
	if errC != nil {
		return nil, convertError(errC)
	}

	return newValue(C.colonio_const_value_t(valueC)), nil
}

func (ci *colonioImpl) KvsSet(key string, valueG interface{}, opt uint32) error {
	value, err := NewValue(valueG)
	if err != nil {
		return err
	}
	var valueC C.colonio_value_t
	C.colonio_value_create(&valueC)
	defer C.colonio_value_free(&valueC)
	writeOut(valueC, value)

	// set
	errC := C.cgo_colonio_kvs_set(ci.colonioC, key, C.colonio_const_value_t(valueC), C.uint32_t(opt))

	return convertError(errC)
}

// spread
func (ci *colonioImpl) SpreadPost(x, y, r float64, name string, messageG interface{}, opt uint32) error {
	// value
	message, err := NewValue(messageG)
	if err != nil {
		return err
	}
	var messageC C.colonio_value_t
	C.colonio_value_create(&messageC)
	defer C.colonio_value_free(&messageC)
	writeOut(messageC, message)

	// post
	errC := C.cgo_colonio_spread_post(ci.colonioC, C.double(x), C.double(y), C.double(r), name, C.colonio_const_value_t(messageC), C.uint32_t(opt))
	return convertError(errC)
}

//export cgoWrapSpreadHandler
func cgoWrapSpreadHandler(colonioC C.colonio_t, id C.ulong, src unsafe.Pointer, messageC C.colonio_const_value_t, opt C.uint32_t) {
	colonio, ok := colonioMap[colonioC]
	if !ok {
		log.Fatalln("colonioImpl not found")
	}

	request := &SpreadRequest{
		SourceNid: C.GoStringN((*C.char)(src), C.int(C.cgo_colonio_nid_length)),
		Message:   newValue(messageC),
		Options:   uint32(opt),
	}

	colonio.spreadMutex.RLock()
	defer colonio.spreadMutex.RUnlock()

	handler, ok := colonio.spreadHandlers[uint32(id)]
	if !ok {
		log.Fatalln("messaging handler not found")
	}

	go handler(request)
}

func (ci *colonioImpl) SpreadSetHandler(name string, handler func(*SpreadRequest)) {
	ci.SpreadUnsetHandler(name)

	ci.spreadMutex.Lock()
	defer ci.spreadMutex.Unlock()

	var id uint32
	for {
		id = rand.Uint32()
		if _, ok := ci.spreadHandlers[id]; !ok {
			break
		}
	}
	ci.spreadHandlers[id] = handler
	ci.spreadHandlerNames[name] = id

	C.cgo_colonio_spread_set_handler(ci.colonioC, name, C.ulong(id))
}

func (ci *colonioImpl) SpreadUnsetHandler(name string) {
	ci.spreadMutex.Lock()
	defer ci.spreadMutex.Unlock()

	id, ok := ci.spreadHandlerNames[name]
	// handler not found
	if !ok {
		return
	}
	delete(ci.spreadHandlers, id)
	delete(ci.spreadHandlerNames, name)

	C.cgo_colonio_spread_unset_handler(ci.colonioC, name)
}

// NewValue create a value instance managed by colonio
func NewValue(valueG interface{}) (Value, error) {
	vi := &valueImpl{}
	err := vi.Set(valueG)
	if err != nil {
		return nil, err
	}
	return vi, nil
}

func (vi *valueImpl) IsNil() bool {
	return vi.valueTypeC == C.COLONIO_VALUE_TYPE_NULL
}

func (vi *valueImpl) IsBool() bool {
	return vi.valueTypeC == C.COLONIO_VALUE_TYPE_BOOL
}

func (vi *valueImpl) IsInt() bool {
	return vi.valueTypeC == C.COLONIO_VALUE_TYPE_INT
}

func (vi *valueImpl) IsDouble() bool {
	return vi.valueTypeC == C.COLONIO_VALUE_TYPE_DOUBLE
}

func (vi *valueImpl) IsString() bool {
	return vi.valueTypeC == C.COLONIO_VALUE_TYPE_STRING
}

func (vi *valueImpl) IsBinary() bool {
	return vi.valueTypeC == C.COLONIO_VALUE_TYPE_BINARY
}

func (vi *valueImpl) Set(valueG interface{}) error {
	vi.stringG = ""
	vi.binaryG = nil

	switch v := valueG.(type) {
	case bool:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_BOOL
		vi.boolG = v
		return nil

	case int:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = int64(v)
		return nil

	case int8:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = int64(v)
		return nil

	case int16:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = int64(v)
		return nil

	case int32:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = int64(v)
		return nil

	case int64:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = v
		return nil

	case uint:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = int64(v)
		return nil

	case uint8:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = int64(v)
		return nil

	case uint16:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = int64(v)
		return nil

	case uint32:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = int64(v)
		return nil

	case uint64:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_INT
		vi.intG = int64(v)
		return nil

	case float32:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_DOUBLE
		vi.doubleG = float64(v)
		return nil

	case float64:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_DOUBLE
		vi.doubleG = v
		return nil

	case string:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_STRING
		vi.stringG = v
		return nil

	case []byte:
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_BINARY
		vi.binaryG = v
		return nil

	case *valueImpl:
		*vi = *v
		return nil
	}

	if valueG == nil || reflect.ValueOf(valueG).IsNil() {
		vi.valueTypeC = C.COLONIO_VALUE_TYPE_NULL
		return nil
	}

	return fmt.Errorf("unsupported value type")
}

func (vi *valueImpl) GetBool() (bool, error) {
	if vi.valueTypeC != C.COLONIO_VALUE_TYPE_BOOL {
		return false, fmt.Errorf("the type of value is wrong")
	}
	return vi.boolG, nil
}

func (vi *valueImpl) GetInt() (int64, error) {
	if vi.valueTypeC != C.COLONIO_VALUE_TYPE_INT {
		return 0, fmt.Errorf("the type of value is wrong")
	}
	return vi.intG, nil
}

func (vi *valueImpl) GetDouble() (float64, error) {
	if vi.valueTypeC != C.COLONIO_VALUE_TYPE_DOUBLE {
		return 0, fmt.Errorf("the type of value is wrong")
	}
	return vi.doubleG, nil
}

func (vi *valueImpl) GetString() (string, error) {
	if vi.valueTypeC != C.COLONIO_VALUE_TYPE_STRING {
		return "", fmt.Errorf("the type of value is wrong")
	}
	return vi.stringG, nil
}

func (vi *valueImpl) GetBinary() ([]byte, error) {
	if vi.valueTypeC != C.COLONIO_VALUE_TYPE_BINARY {
		return nil, fmt.Errorf("the type of value is wrong")
	}
	return vi.binaryG, nil
}

func writeOut(valueC C.colonio_value_t, value Value) {
	if value.IsBool() {
		v, _ := value.GetBool()
		C.colonio_value_set_bool(valueC, C.bool(v))
		return
	}

	if value.IsInt() {
		v, _ := value.GetInt()
		C.colonio_value_set_int(valueC, C.int64_t(v))
		return
	}

	if value.IsDouble() {
		v, _ := value.GetDouble()
		C.colonio_value_set_double(valueC, C.double(v))
		return
	}

	if value.IsString() {
		v, _ := value.GetString()
		C.cgo_colonio_value_set_string(valueC, v)
		return
	}

	if value.IsBinary() {
		v, _ := value.GetBinary()
		b := C.CBytes(v)
		defer C.free(b)
		C.colonio_value_set_binary(valueC, b, C.uint(len(v)))
		return
	}
}
