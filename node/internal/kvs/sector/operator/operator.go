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
	"errors"
	"fmt"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

// ErrOperationTimeout is returned when a proposed operation was not applied
// within the timeout. The result is unknown: the operation may still be
// committed and applied later.
var ErrOperationTimeout = errors.New("operation was not applied within timeout")

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

	// operationTimeout is how long a write waits for its operation to be
	// applied. A group that lost its quorum commits nothing, so the wait
	// must end.
	operationTimeout time.Duration
	// waiters has the writes that are waiting for their operation to be
	// applied on this replica, keyed by the operation ID. The IDs are unique
	// within this operator only: the hosting sector of a group is the only
	// one that proposes operations, so an applied ID is either in this map
	// or has no waiter at all.
	nextOperationID uint32
	waiters         map[uint32]chan error
	// mergeFenced rejects writes while the sector has a prepare_merge claim:
	// its range is about to be exported by the node that merges it. The
	// sector sets it when it applies the claim and its release, so every
	// replica has the same value.
	mergeFenced bool
}

var _ Operations = &Operator{}

func NewOperator(config *Config) *Operator {
	return &Operator{
		sectorKey:        *config.SectorKey,
		handler:          config.Handler,
		store:            config.Store,
		head:             *config.Head,
		keys:             make(map[string]any),
		operationTimeout: 10 * time.Second,
		waiters:          make(map[uint32]chan error),
	}
}

// inRangeLocked reports whether the key is in the range of the sector. A tail
// equal to the head means the whole ring.
func (s *Operator) inRangeLocked(keyHash *types.NodeID) bool {
	if s.tail == nil {
		return false
	}
	if s.head.Equal(s.tail) {
		return true
	}
	return keyHash.IsBetween(&s.head, s.tail)
}

// writableLocked returns ErrorSectorNotReady when the sector cannot accept a
// write to the key now:
//   - the sector is not active, or the key is out of its range;
//   - the sector has a prepare_merge claim, so the whole range is about to be
//     exported;
//   - the key is in the range that a split is handing over. A write applied
//     after the export would be dropped when the split shrinks the range.
func (s *Operator) writableLocked(keyHash *types.NodeID) error {
	if !s.inRangeLocked(keyHash) {
		return kvsTypes.ErrorSectorNotReady
	}
	if s.mergeFenced {
		return kvsTypes.ErrorSectorNotReady
	}
	if s.splittingAddress != nil && !s.splittingAddress.Equal(s.tail) &&
		keyHash.IsBetween(s.splittingAddress, s.tail) {
		return kvsTypes.ErrorSectorNotReady
	}
	return nil
}

// Get reads from the store of this replica, without going through raft. A
// write returns only after it is applied here, so a read sees the writes
// acknowledged by this replica. It may miss an operation that is committed
// but not applied here yet.
func (s *Operator) Get(key string) ([]byte, error) {
	keyHash := types.NewHashedNodeID([]byte(key))

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if !s.inRangeLocked(keyHash) {
		return nil, kvsTypes.ErrorSectorNotReady
	}
	if _, ok := s.keys[key]; !ok {
		return nil, kvsTypes.ErrorStoreKeyNotFound
	}
	return s.store.Get(&s.sectorKey, key)
}

func (s *Operator) Set(key string, value []byte) error {
	return s.proposeOperation(proto.Operation_COMMAND_SET, key, value)
}

func (s *Operator) Patch(key string, value []byte) error {
	return s.proposeOperation(proto.Operation_COMMAND_PATCH, key, value)
}

func (s *Operator) Delete(key string) error {
	return s.proposeOperation(proto.Operation_COMMAND_DELETE, key, nil)
}

// proposeOperation proposes a write and waits until the committed operation
// is applied on this replica. The check and the registration of the waiter
// are done under one lock, so that SetSplitting sees every write accepted
// before it. The proposal is sent after the lock is released.
func (s *Operator) proposeOperation(command proto.Operation_Command, key string, value []byte) error {
	keyHash := types.NewHashedNodeID([]byte(key))

	s.mtx.Lock()
	if err := s.writableLocked(keyHash); err != nil {
		s.mtx.Unlock()
		return err
	}
	s.nextOperationID++
	operationID := s.nextOperationID
	waiter := make(chan error, 1)
	s.waiters[operationID] = waiter
	s.mtx.Unlock()

	s.handler.OperatorProposeOperation(&proto.Operation{
		Command:     command,
		OperationId: operationID,
		Key:         key,
		Value:       value,
	})

	timer := time.NewTimer(s.operationTimeout)
	defer timer.Stop()
	select {
	case err := <-waiter:
		return err

	case <-timer.C:
		s.mtx.Lock()
		delete(s.waiters, operationID)
		s.mtx.Unlock()
		// The operation may have been applied just before the waiter was
		// removed.
		select {
		case err := <-waiter:
			return err
		default:
		}
		return ErrOperationTimeout
	}
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

// SetSplitting makes the sector reject writes to the range from address to
// the tail, which a split is about to export, and waits for the writes that
// were accepted before. A write applied after the export would be dropped
// when the split shrinks the range, although it was acknowledged. The wait
// ends after operationTimeout, by when every accepted write has returned.
// A nil address stops the rejection.
func (s *Operator) SetSplitting(address *types.NodeID) {
	s.mtx.Lock()
	s.splittingAddress = address
	s.mtx.Unlock()

	if address == nil {
		return
	}

	deadline := time.Now().Add(s.operationTimeout)
	for {
		s.mtx.RLock()
		pending := len(s.waiters)
		s.mtx.RUnlock()
		if pending == 0 || time.Now().After(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// SetMergeFence sets whether the sector rejects writes because of a
// prepare_merge claim. See writableLocked.
func (s *Operator) SetMergeFence(fenced bool) {
	s.mtx.Lock()
	s.mergeFenced = fenced
	s.mtx.Unlock()
}

// ApplyProposal applies a committed operation to the store and, on the
// replica that proposed it, tells the result to the waiting write. It runs on
// every replica.
//
// The range is checked again here, since a split or a merge committed after
// the proposal may have moved the key out. The tail is replicated, so every
// replica skips the same operations. A skipped operation and a delete of a
// missing key are results for the waiter, not failures of the apply: only an
// error of the store is returned.
func (s *Operator) ApplyProposal(operation *proto.Operation) error {
	keyHash := types.NewHashedNodeID([]byte(operation.Key))

	s.mtx.Lock()

	var waiterErr error
	var storeErr error
	if !s.inRangeLocked(keyHash) {
		waiterErr = kvsTypes.ErrorSectorNotReady
	} else {
		switch operation.Command {
		case proto.Operation_COMMAND_SET:
			storeErr = s.store.Set(&s.sectorKey, operation.Key, operation.Value)
			if storeErr == nil {
				s.keys[operation.Key] = struct{}{}
			}

		case proto.Operation_COMMAND_PATCH:
			storeErr = s.store.Patch(&s.sectorKey, operation.Key, operation.Value)
			if storeErr == nil {
				s.keys[operation.Key] = struct{}{}
			}

		case proto.Operation_COMMAND_DELETE:
			if _, ok := s.keys[operation.Key]; !ok {
				waiterErr = kvsTypes.ErrorStoreKeyNotFound
			} else {
				storeErr = s.store.Delete(&s.sectorKey, operation.Key)
				if storeErr == nil {
					delete(s.keys, operation.Key)
				}
			}

		default:
			waiterErr = fmt.Errorf("unsupported operation command: %d", operation.Command)
		}
	}
	if storeErr != nil {
		waiterErr = storeErr
	}

	waiter := s.waiters[operation.OperationId]
	delete(s.waiters, operation.OperationId)

	s.mtx.Unlock()

	if waiter != nil {
		waiter <- waiterErr
	}

	return storeErr
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

// ExportAllRecords returns every record of the sector, whatever its range. A
// sector that is not active yet has no range but can hold records.
func (s *Operator) ExportAllRecords() (map[string][]byte, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	records := make(map[string][]byte)

	for key := range s.keys {
		value, err := s.store.Get(&s.sectorKey, key)
		if err != nil {
			return nil, err
		}
		records[key] = value
	}

	return records, nil
}

// ReplaceRecords makes records the whole content of the sector. Unlike
// ImportRecords, it forgets the keys that are not in records. The caller must
// empty the store sector before calling this.
func (s *Operator) ReplaceRecords(records map[string][]byte) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.keys = make(map[string]any)
	for key, value := range records {
		if err := s.store.Set(&s.sectorKey, key, value); err != nil {
			return err
		}
		s.keys[key] = struct{}{}
	}

	return nil
}
