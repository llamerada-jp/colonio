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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/require"
)

// storeHelper is a minimal functional in-memory store for one sector.
type storeHelper struct {
	mtx     sync.Mutex
	records map[string][]byte
}

var _ kvsTypes.Store = &storeHelper{}

func newStoreHelper() *storeHelper {
	return &storeHelper{records: map[string][]byte{}}
}

func (s *storeHelper) AllocateSector(sectorKey *kvsTypes.SectorKey) error { return nil }
func (s *storeHelper) ReleaseSector(sectorKey *kvsTypes.SectorKey) error  { return nil }

func (s *storeHelper) Get(sectorKey *kvsTypes.SectorKey, key string) ([]byte, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	value, ok := s.records[key]
	if !ok {
		return nil, kvsTypes.ErrorStoreKeyNotFound
	}
	return value, nil
}

func (s *storeHelper) Set(sectorKey *kvsTypes.SectorKey, key string, value []byte) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.records[key] = value
	return nil
}

func (s *storeHelper) Delete(sectorKey *kvsTypes.SectorKey, key string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	delete(s.records, key)
	return nil
}

// handlerHelper lets each test decide how a proposed operation is "committed".
type handlerHelper struct {
	proposeF func(operation *proto.Operation)
}

var _ Handler = &handlerHelper{}

func (h *handlerHelper) OperatorProposeOperation(operation *proto.Operation) {
	h.proposeF(operation)
}

func newTestOperator(handler Handler) *Operator {
	head := types.NewNormalNodeID(0x4000000000000000, 0)
	return NewOperator(&Config{
		SectorKey: &kvsTypes.SectorKey{
			SectorID: kvsTypes.SectorID(uuid.New()),
			SectorNo: kvsTypes.HostNodeSectorNo,
		},
		Handler: handler,
		Store:   newStoreHelper(),
		Head:    head,
	})
}

// echoHandler applies proposals inline, mimicking an instantly committing
// single-member raft group.
func echoHandler(op **Operator) *handlerHelper {
	return &handlerHelper{
		proposeF: func(operation *proto.Operation) {
			// the waiter channel is buffered, so an inline apply is fine
			_ = (*op).ApplyProposal(operation)
		},
	}
}

// findKey searches a key whose hash satisfies the predicate (keys are hashed
// onto the ring, so in/out-of-range keys must be found by probing).
func findKey(t *testing.T, predicate func(hash *types.NodeID) bool) string {
	t.Helper()
	for i := 0; i < 100000; i++ {
		key := fmt.Sprintf("key-%d", i)
		if predicate(types.NewHashedNodeID([]byte(key))) {
			return key
		}
	}
	require.FailNow(t, "no key found for the predicate")
	return ""
}

func TestOperator_setGetDelete(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	// tail == head: the sector covers the whole ring, every key is in range
	require.NoError(t, o.SetRange(o.head))

	// read-your-writes on the host: Set is acknowledged after the local apply
	require.NoError(t, o.Set("key1", []byte("value1")))
	value, err := o.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)

	// overwrite
	require.NoError(t, o.Set("key1", []byte("value2")))
	value, err = o.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value2"), value)

	// missing key
	_, err = o.Get("nope")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	// delete
	require.NoError(t, o.Delete("key1"))
	_, err = o.Get("key1")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	// deleting an absent key is a client-level miss
	require.ErrorIs(t, o.Delete("key1"), kvsTypes.ErrorStoreKeyNotFound)
}

func TestOperator_notActivated(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	// no SetRange: the sector is not activated

	require.ErrorIs(t, o.Set("key1", []byte("value1")), kvsTypes.ErrorSectorNotReady)
	_, err := o.Get("key1")
	require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
	require.ErrorIs(t, o.Delete("key1"), kvsTypes.ErrorSectorNotReady)
}

func TestOperator_outOfRange(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	tail := types.NewNormalNodeID(0x8000000000000000, 0)
	require.NoError(t, o.SetRange(*tail))

	inKey := findKey(t, func(hash *types.NodeID) bool { return hash.IsBetween(&o.head, tail) })
	outKey := findKey(t, func(hash *types.NodeID) bool { return !hash.IsBetween(&o.head, tail) })

	require.NoError(t, o.Set(inKey, []byte("value")))
	require.ErrorIs(t, o.Set(outKey, []byte("value")), kvsTypes.ErrorSectorNotReady)
	_, err := o.Get(outKey)
	require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
}

func TestOperator_mergeFence(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	require.NoError(t, o.SetRange(o.head))

	o.SetMergeFence(true)
	require.ErrorIs(t, o.Set("key1", []byte("value1")), kvsTypes.ErrorSectorNotReady)
	// reads stay available while the merge lock is held
	_, err := o.Get("key1")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	o.SetMergeFence(false)
	require.NoError(t, o.Set("key1", []byte("value1")))
}

func TestOperator_splittingFence(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	require.NoError(t, o.SetRange(o.head)) // whole ring
	splitting := types.NewNormalNodeID(0x8000000000000000, 0)

	keepKey := findKey(t, func(hash *types.NodeID) bool { return hash.IsBetween(&o.head, splitting) })
	moveKey := findKey(t, func(hash *types.NodeID) bool { return hash.IsBetween(splitting, &o.head) })

	o.SetSplitting(splitting)

	// the range being exported rejects writes; the surviving range accepts them
	require.ErrorIs(t, o.Set(moveKey, []byte("value")), kvsTypes.ErrorSectorNotReady)
	require.NoError(t, o.Set(keepKey, []byte("value")))

	o.SetSplitting(nil)
	require.NoError(t, o.Set(moveKey, []byte("value")))
}

func TestOperator_timeout(t *testing.T) {
	// the handler drops the proposal: no apply ever happens
	o := newTestOperator(&handlerHelper{proposeF: func(operation *proto.Operation) {}})
	o.operationTimeout = 100 * time.Millisecond
	require.NoError(t, o.SetRange(o.head))

	err := o.Set("key1", []byte("value1"))
	require.ErrorIs(t, err, ErrOperationTimeout)

	// the dangling waiter was deregistered
	o.mtx.RLock()
	require.Empty(t, o.waiters)
	o.mtx.RUnlock()
}

// TestOperator_applyRechecksRange covers the split/merge interleaving: an
// operation accepted under the old tail may be applied after a committed
// PreCommitSplit moved the key out of range. The apply must skip the write
// (deterministically on every replica) and fail only the local waiter, so the
// client retries against the new owner.
func TestOperator_applyRechecksRange(t *testing.T) {
	proposals := make(chan *proto.Operation, 1)
	o := newTestOperator(&handlerHelper{proposeF: func(operation *proto.Operation) {
		proposals <- operation
	}})
	require.NoError(t, o.SetRange(o.head)) // whole ring

	newTail := types.NewNormalNodeID(0x8000000000000000, 0)
	movedKey := findKey(t, func(hash *types.NodeID) bool { return hash.IsBetween(newTail, &o.head) })

	result := make(chan error, 1)
	go func() { result <- o.Set(movedKey, []byte("value")) }()
	operation := <-proposals

	// a split shrinks the range before the operation is applied
	require.NoError(t, o.SetRange(*newTail))
	require.NoError(t, o.ApplyProposal(operation))

	require.ErrorIs(t, <-result, kvsTypes.ErrorSectorNotReady)
	// the skipped write must not be visible
	o.mtx.RLock()
	_, exists := o.keys[movedKey]
	o.mtx.RUnlock()
	require.False(t, exists)
}

// TestOperator_setSplittingDrainsPending covers the export fence: operations
// accepted before SetSplitting may still be in flight, and the caller exports
// the records right after it returns — so SetSplitting must wait for the
// pending operations to be applied.
func TestOperator_setSplittingDrainsPending(t *testing.T) {
	proposals := make(chan *proto.Operation, 1)
	var o *Operator
	o = newTestOperator(&handlerHelper{proposeF: func(operation *proto.Operation) {
		proposals <- operation
	}})
	require.NoError(t, o.SetRange(o.head))

	result := make(chan error, 1)
	go func() { result <- o.Set("key1", []byte("value1")) }()
	operation := <-proposals // accepted, not yet applied

	// apply the pending operation shortly after; SetSplitting must block
	// until then
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = o.ApplyProposal(operation)
	}()

	start := time.Now()
	o.SetSplitting(types.NewNormalNodeID(0x8000000000000000, 0))
	require.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond)

	require.NoError(t, <-result)
	// the drained write is visible to the export that follows
	records, err := o.ExportAllRecords()
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{"key1": []byte("value1")}, records)
}
