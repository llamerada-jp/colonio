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
package operator

import (
	"sync"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

type Operations interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Patch(key string, value []byte) error
	Delete(key string) error
}

type Handler interface {
	OperatorProposeOperation(operation *proto.Operation)
}

type Config struct {
	SectorKey *kvsTypes.SectorKey
	Handler   Handler
	Store     kvsTypes.Store
	Head      *types.NodeID
}

type Operator struct {
	sectorKey        kvsTypes.SectorKey
	handler          Handler
	mtx              sync.RWMutex
	store            kvsTypes.Store
	head             types.NodeID
	tail             *types.NodeID
	splittingAddress *types.NodeID
	keys             map[string]any
}

var _ Operations = &Operator{}

func NewOperator(config *Config) *Operator {
	return &Operator{
		sectorKey: *config.SectorKey,
		handler:   config.Handler,
		store:     config.Store,
		head:      *config.Head,
		keys:      make(map[string]any),
	}
}

func (s *Operator) Get(key string) ([]byte, error) {
	panic("get not implemented")
}

func (s *Operator) Set(key string, value []byte) error {
	panic("set not implemented")
}

func (s *Operator) Patch(key string, value []byte) error {
	panic("patch not implemented")
}

func (s *Operator) Delete(key string) error {
	panic("delete not implemented")
}

func (s *Operator) SetRange(tail types.NodeID) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// tail address is not changed or expanded, so no need to delete records.
	if s.tail == nil || s.tail.Equal(&tail) || s.tail.IsBetween(&s.head, &tail) {
		s.tail = &tail
		return nil
	}

	// Delete records in the range (tail, s.tail].
	for key := range s.keys {
		keyHash := types.NewHashedNodeID([]byte(key))
		if !keyHash.IsBetween(&tail, s.tail) {
			continue
		}
		if err := s.store.Delete(&s.sectorKey, key); err != nil {
			return err
		}
		delete(s.keys, key)
	}

	s.tail = &tail
	return nil
}

func (s *Operator) ClearRange() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.tail = nil
}

func (s *Operator) SetSplitting(address *types.NodeID) {
	s.mtx.Lock()
	s.splittingAddress = address
	s.mtx.Unlock()

	if address == nil {
		return
	}

	// TODO: Wait for all proposed operations to be applied frontward from splittingTail.
}

func (s *Operator) ApplyProposal(command *proto.Operation) error {
	panic("apply not implemented")
}

func (s *Operator) ExportRecords(head, tail *types.NodeID) (map[string][]byte, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	records := make(map[string][]byte)

	for key := range s.keys {
		keyHash := types.NewHashedNodeID([]byte(key))
		if !keyHash.IsBetween(head, tail) {
			continue
		}

		value, err := s.store.Get(&s.sectorKey, key)
		if err != nil {
			return nil, err
		}
		records[key] = value
	}

	return records, nil
}

func (s *Operator) ImportRecords(records map[string][]byte) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for key, value := range records {
		if err := s.store.Set(&s.sectorKey, key, value); err != nil {
			return err
		}
		s.keys[key] = struct{}{}
	}

	return nil
}

func (s *Operator) ExportSnapshot() ([]byte, error) {
	panic("exportSnapshot not implemented")
}

func (s *Operator) ImportSnapshot(data []byte) error {
	panic("importSnapshot not implemented")
}
