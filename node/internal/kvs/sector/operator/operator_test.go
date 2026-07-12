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
	revision1, err := o.Set("key1", []byte("value1"), nil)
	require.NoError(t, err)
	require.Equal(t, uint64(1), revision1)
	value, revision, err := o.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)
	require.Equal(t, revision1, revision)

	// overwrite: the revision grows monotonically
	revision2, err := o.Set("key1", []byte("value2"), nil)
	require.NoError(t, err)
	require.Greater(t, revision2, revision1)
	value, _, err = o.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value2"), value)

	// missing key
	_, _, err = o.Get("nope")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	// delete
	require.NoError(t, o.Delete("key1", nil))
	_, _, err = o.Get("key1")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	// deleting an absent key is a client-level miss
	require.ErrorIs(t, o.Delete("key1", nil), kvsTypes.ErrorStoreKeyNotFound)

	// re-creation after delete continues past the old revision (no ABA)
	revision3, err := o.Set("key1", []byte("value3"), nil)
	require.NoError(t, err)
	require.Greater(t, revision3, revision2)
}

func TestOperator_notActivated(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	// no SetRange: the sector is not activated

	_, err := o.Set("key1", []byte("value1"), nil)
	require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
	_, _, err = o.Get("key1")
	require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
	require.ErrorIs(t, o.Delete("key1", nil), kvsTypes.ErrorSectorNotReady)
}

func TestOperator_outOfRange(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	tail := types.NewNormalNodeID(0x8000000000000000, 0)
	require.NoError(t, o.SetRange(*tail))

	inKey := findKey(t, func(hash *types.NodeID) bool { return hash.IsBetween(&o.head, tail) })
	outKey := findKey(t, func(hash *types.NodeID) bool { return !hash.IsBetween(&o.head, tail) })

	_, err := o.Set(inKey, []byte("value"), nil)
	require.NoError(t, err)
	_, err = o.Set(outKey, []byte("value"), nil)
	require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
	_, _, err = o.Get(outKey)
	require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
}

func TestOperator_mergeFence(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	require.NoError(t, o.SetRange(o.head))

	o.SetMergeFence(true)
	_, err := o.Set("key1", []byte("value1"), nil)
	require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
	// reads stay available while the merge lock is held
	_, _, err = o.Get("key1")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	o.SetMergeFence(false)
	_, err = o.Set("key1", []byte("value1"), nil)
	require.NoError(t, err)
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
	_, err := o.Set(moveKey, []byte("value"), nil)
	require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
	_, err = o.Set(keepKey, []byte("value"), nil)
	require.NoError(t, err)

	o.SetSplitting(nil)
	_, err = o.Set(moveKey, []byte("value"), nil)
	require.NoError(t, err)
}

func TestOperator_timeout(t *testing.T) {
	// the handler drops the proposal: no apply ever happens
	o := newTestOperator(&handlerHelper{proposeF: func(operation *proto.Operation) {}})
	o.operationTimeout = 100 * time.Millisecond
	require.NoError(t, o.SetRange(o.head))

	_, err := o.Set("key1", []byte("value1"), nil)
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
	go func() {
		_, err := o.Set(movedKey, []byte("value"), nil)
		result <- err
	}()
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
	go func() {
		_, err := o.Set("key1", []byte("value1"), nil)
		result <- err
	}()
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
	// the drained write is visible to the export that follows (the export
	// carries opaque record envelopes)
	records, _, err := o.ExportAllRecords()
	require.NoError(t, err)
	require.Len(t, records, 1)
	record, err := decodeRecord(records["key1"])
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), record.Value)
}

func TestOperator_cas(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	require.NoError(t, o.SetRange(o.head)) // whole ring

	// creation guarded by absence succeeds once...
	revision1, err := o.Set("key1", []byte("value1"), &CasCondition{Absent: true})
	require.NoError(t, err)
	require.NotZero(t, revision1)

	// ...and conflicts once the record exists
	_, err = o.Set("key1", []byte("value2"), &CasCondition{Absent: true})
	require.ErrorIs(t, err, kvsTypes.ErrorCasConflict)

	// conditional overwrite with the current revision succeeds
	revision2, err := o.Set("key1", []byte("value2"), &CasCondition{Revision: revision1})
	require.NoError(t, err)
	require.Greater(t, revision2, revision1)

	// the stale revision now conflicts, and the store is left untouched
	_, err = o.Set("key1", []byte("value3"), &CasCondition{Revision: revision1})
	require.ErrorIs(t, err, kvsTypes.ErrorCasConflict)
	value, revision, err := o.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value2"), value)
	require.Equal(t, revision2, revision)

	// conditional delete: stale revision conflicts, current succeeds
	require.ErrorIs(t, o.Delete("key1", &CasCondition{Revision: revision1}), kvsTypes.ErrorCasConflict)
	require.NoError(t, o.Delete("key1", &CasCondition{Revision: revision2}))
	_, _, err = o.Get("key1")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	// a revision-conditioned write against an absent record conflicts
	_, err = o.Set("key1", []byte("value"), &CasCondition{Revision: revision2})
	require.ErrorIs(t, err, kvsTypes.ErrorCasConflict)
}

func TestOperator_importMaxMergesCounter(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	require.NoError(t, o.SetRange(o.head))

	// a record migrated in carries revision 40 from its source sector
	imported, err := encodeRecord([]byte("imported"), 40)
	require.NoError(t, err)
	require.NoError(t, o.ImportRecords(map[string][]byte{"moved": imported}, 40))

	// the next local write must jump past every imported revision, so a CAS
	// chain built on the migrated record cannot be fooled by a reused number
	revision, err := o.Set("fresh", []byte("value"), nil)
	require.NoError(t, err)
	require.Equal(t, uint64(41), revision)

	// importing an older counter must not move the counter backwards
	require.NoError(t, o.ImportRecords(map[string][]byte{}, 10))
	revision, err = o.Set("fresh", []byte("value"), nil)
	require.NoError(t, err)
	require.Equal(t, uint64(42), revision)

	// the imported record is readable with its original revision
	value, importedRevision, err := o.Get("moved")
	require.NoError(t, err)
	require.Equal(t, []byte("imported"), value)
	require.Equal(t, uint64(40), importedRevision)
}

func TestOperator_replaceRecordsAssignsCounter(t *testing.T) {
	var o *Operator
	o = newTestOperator(echoHandler(&o))
	require.NoError(t, o.SetRange(o.head))

	// local state that the snapshot must fully replace
	_, err := o.Set("stale", []byte("stale"), nil)
	require.NoError(t, err)

	restored, err := encodeRecord([]byte("restored"), 99)
	require.NoError(t, err)
	require.NoError(t, o.ReplaceRecords(map[string][]byte{"kept": restored}, 100))

	// replaced, not merged
	_, _, err = o.Get("stale")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	// the counter continues from the snapshot value
	revision, err := o.Set("next", []byte("value"), nil)
	require.NoError(t, err)
	require.Equal(t, uint64(101), revision)
}

// testAppendPatcher appends the patch document to the current value.
type testAppendPatcher struct{}

func (testAppendPatcher) Apply(current []byte, patch []byte) ([]byte, error) {
	result := make([]byte, 0, len(current)+len(patch))
	result = append(result, current...)
	return append(result, patch...), nil
}

// testFailPatcher deterministically rejects every patch.
type testFailPatcher struct{}

func (testFailPatcher) Apply(current []byte, patch []byte) ([]byte, error) {
	return nil, fmt.Errorf("rejected")
}

func newTestOperatorWithPatchers(handler Handler) *Operator {
	o := newTestOperator(handler)
	o.patchers = map[string]kvsTypes.Patcher{
		"append": testAppendPatcher{},
		"fail":   testFailPatcher{},
	}
	return o
}

func TestOperator_patch(t *testing.T) {
	var o *Operator
	o = newTestOperatorWithPatchers(echoHandler(&o))
	require.NoError(t, o.SetRange(o.head)) // whole ring

	revision1, err := o.Set("key1", []byte("base"), nil)
	require.NoError(t, err)

	// happy path: the patcher transforms the stored value, revision grows
	revision2, err := o.Patch("key1", "append", []byte("+p"), nil)
	require.NoError(t, err)
	require.Greater(t, revision2, revision1)
	value, revision, err := o.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("base+p"), value)
	require.Equal(t, revision2, revision)

	// unregistered patcher: rejected before proposing (no waiter left behind)
	_, err = o.Patch("key1", "nope", []byte("x"), nil)
	require.ErrorIs(t, err, kvsTypes.ErrorPatchFailed)
	o.mtx.RLock()
	require.Empty(t, o.waiters)
	o.mtx.RUnlock()

	// patcher rejection: waiter-level failure, store untouched
	_, err = o.Patch("key1", "fail", []byte("x"), nil)
	require.ErrorIs(t, err, kvsTypes.ErrorPatchFailed)
	value, revision, err = o.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("base+p"), value)
	require.Equal(t, revision2, revision)

	// absent record: a miss, creation is Set's job
	_, err = o.Patch("absent", "append", []byte("x"), nil)
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	// conditional patch: stale revision conflicts, current succeeds
	_, err = o.Patch("key1", "append", []byte("+q"), &CasCondition{Revision: revision1})
	require.ErrorIs(t, err, kvsTypes.ErrorCasConflict)
	revision3, err := o.Patch("key1", "append", []byte("+q"), &CasCondition{Revision: revision2})
	require.NoError(t, err)
	require.Greater(t, revision3, revision2)
	value, _, err = o.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("base+p+q"), value)
}
