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
package sector

import (
	"sync"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Operations interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Patch(key string, value []byte) error
	Delete(key string) error
}

type operatorHandler interface {
	operatorProposeOperation(operation *proto.Operation)
}

type operatorConfig struct {
	sectorKey *config.KvsSectorKey
	handler   operatorHandler
	store     config.KvsStore
	head      *shared.NodeID
}

type lock struct {
	coordinator uint64
	address     *shared.NodeID
}

type operator struct {
	sectorKey config.KvsSectorKey
	handler   operatorHandler
	mtx       sync.RWMutex
	store     config.KvsStore
	head      shared.NodeID
	tail      *shared.NodeID
	readonly  *lock
	blocked   *lock
	keys      map[string]any
}

var _ Operations = &operator{}

func newOperator(config *operatorConfig) *operator {
	return &operator{
		sectorKey: *config.sectorKey,
		handler:   config.handler,
		store:     config.store,
		head:      *config.head,
		keys:      make(map[string]any),
	}
}

func (s *operator) Get(key string) ([]byte, error) {
	panic("get not implemented")
}

func (s *operator) Set(key string, value []byte) error {
	panic("set not implemented")
}

func (s *operator) Patch(key string, value []byte) error {
	panic("patch not implemented")
}

func (s *operator) Delete(key string) error {
	panic("delete not implemented")
}

func (s *operator) activate(tail *shared.NodeID) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.tail != nil {
		panic("logic error: already activated")
	}

	t := *tail
	s.tail = &t
}

func (s *operator) applyProposal(command *proto.Operation) {
	panic("apply not implemented")
}

func (s *operator) exportRecords(head, tail *shared.NodeID) []byte {
	panic("export not implemented")
}

func (s *operator) importRecords(data []byte) error {
	panic("import not implemented")
}

func (s *operator) exportSnapshot() ([]byte, error) {
	panic("exportSnapshot not implemented")
}

func (s *operator) importSnapshot(data []byte) error {
	panic("importSnapshot not implemented")
}
