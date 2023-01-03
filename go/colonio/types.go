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
	IsBinary() bool
	Set(val interface{}) error
	GetBool() (bool, error)
	GetInt() (int64, error)
	GetDouble() (float64, error)
	GetString() (string, error)
	GetBinary() ([]byte, error)
}

const (
	// If there is no node with a matching node-id, the node with the closest node-id will receive the call.
	MessagingAcceptNearby uint32 = 0x01
	// If this option is specified, call will not wait for a response. Also, no error will occur if no node receives the
	// call. You should return null value instead of this option if you just don't need return value.
	MessagingIgnoreReply uint32 = 0x02

	KvsMustExistKey      = 0x01
	KvsProhibitOverwrite = 0x02

	SpreadSomeoneMustReceive = 0x01
)

type ColonioConfig struct {
	MaxUserThreads int
	LoggerFunc     func(string)
}

type MessagingRequest struct {
	SourceNid string
	Message   Value
	Options   uint32
}

type MessagingResponseWriter interface {
	Write(interface{})
}

type KvsLocalData interface {
	GetKeys() []string
	GetValue(key string) (Value, error)
	Free()
}

type SpreadRequest struct {
	SourceNid string
	Message   Value
	Options   uint32
}

// Colonio is an interface. It is equivalent to one node.
type Colonio interface {
	Connect(url, token string) error
	Disconnect() error
	IsConnected() bool
	GetLocalNid() string
	SetPosition(x, y float64) (float64, float64, error)
	// messaging
	MessagingPost(dst, name string, val interface{}, opt uint32) (Value, error)
	MessagingSetHandler(name string, handler func(*MessagingRequest, MessagingResponseWriter))
	MessagingUnsetHandler(name string)
	// kvs
	KvsGetLocalData() KvsLocalData
	KvsGet(key string) (Value, error)
	KvsSet(key string, val interface{}, opt uint32) error
	// spread
	SpreadPost(x, y, r float64, name string, message interface{}, opt uint32) error
	SpreadSetHandler(name string, handler func(*SpreadRequest))
	SpreadUnsetHandler(name string)
}
