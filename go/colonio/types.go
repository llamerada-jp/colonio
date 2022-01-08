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

// Value is an instance, it is equivalent to one value.
type Value interface {
	IsNil() bool
	IsBool() bool
	IsInt() bool
	IsDouble() bool
	IsString() bool
	Set(val interface{}) error
	GetBool() (bool, error)
	GetInt() (int64, error)
	GetDouble() (float64, error)
	GetString() (string, error)
}

// Options for Colonio.CallByNid opt parameter.
const (
	// If there is no node with a matching node-id, the node with the closest node-id will receive the call.
	ColonioCallAcceptNearby uint32 = 0x01
	// If this option is specified, call will not wait for a response. Also, no error will occur if no node receives the
	// call. You should return null value instead of this option if you just don't need return value.
	ColonioCallIgnoreReply uint32 = 0x02
)

// CallParameter is used by OnCall. it is pass from CallByNid.
type CallParameter struct {
	Name    string
	Value   Value
	Options uint32
}

// Colonio is an interface. It is equivalent to one node.
type Colonio interface {
	Connect(url, token string) error
	Disconnect() error
	IsConnected() bool
	AccessMap(name string) (Map, error)
	AccessPubsub2D(name string) (Pubsub2D, error)
	GetLocalNid() string
	SetPosition(x, y float64) (float64, float64, error)
	CallByNid(dst, name string, val interface{}, opt uint32) (Value, error)
	OnCall(name string, f func(*CallParameter) interface{})
	OffCall(name string)
	Quit() error
}

// Logger is an interface to configure the output destination of the logs.
type Logger interface {
	// The log messages are passed as JSON format strings.
	// Colonio runs on multi-threads, so this interface is called on multi-threads.
	Output(message string)
}

// Options for Map interface's opt parameter
const (
	MapErrorWithoutExist uint32 = 0x1 // for del, unlock
	MapErrorWithExist    uint32 = 0x2 // for set (haven't done enough testing)
)

// Map is an interface to use key-value-store.
type Map interface {
	ForeachLocalValue(cb func(key, value Value, attr uint32)) error
	Get(key interface{}) (Value, error)
	Set(key, val interface{}, opt uint32) error
}

// Options for Pubsub2D interface's opt parameter
const (
	Pubsub2DRaiseNoOneRecv uint32 = 0x1
)

// Pubsub2D is an interface to using publish–subscribe with 2D coordinate information.
type Pubsub2D interface {
	Publish(name string, x, y, r float64, val interface{}, opt uint32) error
	On(name string, cb func(Value))
	Off(name string)
}
