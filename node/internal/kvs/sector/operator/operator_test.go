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

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/require"
)

// storeHelper keeps the records of one sector.
type storeHelper struct {
	mtx     sync.Mutex
	records map[string][]byte
}

var _ kvsTypes.Store = &storeHelper{}

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

func (s *storeHelper) Patch(sectorKey *kvsTypes.SectorKey, key string, value []byte) error {
	return fmt.Errorf("patch is not supported")
}

func (s *storeHelper) Delete(sectorKey *kvsTypes.SectorKey, key string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	delete(s.records, key)
	return nil
}

// handlerHelper lets each test decide what happens to a proposed operation.
type handlerHelper struct {
	proposeF func(operation *proto.Operation)
}

var _ Handler = &handlerHelper{}

func (h *handlerHelper) OperatorProposeOperation(operation *proto.Operation) {
	h.proposeF(operation)
}

// The head of the operator, and the middle of the ring from it.
var (
	testHead   = types.NewNormalNodeID(0x4000000000000000, 0)
	testMiddle = types.NewNormalNodeID(0xc000000000000000, 0)
)

func newTestOperator(handler Handler) *Operator {
	return NewOperator(&Config{
		SectorKey: &kvsTypes.SectorKey{
			SectorID: testUtil.UniqueSectorIDs(1)[0],
			SectorNo: kvsTypes.HostNodeSectorNo,
		},
		Handler: handler,
		Store:   &storeHelper{records: map[string][]byte{}},
		Head:    testHead,
	})
}

// newEchoOperator returns an operator whose proposals are applied at once,
// as in a raft group that commits immediately.
func newEchoOperator() *Operator {
	var o *Operator
	o = newTestOperator(&handlerHelper{
		proposeF: func(operation *proto.Operation) {
			_ = o.ApplyProposal(operation)
		},
	})
	return o
}

// newPendingOperator returns an operator whose proposals are not applied
// until the test applies them. They are sent to the returned channel.
func newPendingOperator() (*Operator, chan *proto.Operation) {
	proposals := make(chan *proto.Operation, 10)
	return newTestOperator(&handlerHelper{
		proposeF: func(operation *proto.Operation) {
			proposals <- operation
		},
	}), proposals
}

// findKey returns a key whose hash is in [from, to).
func findKey(t *testing.T, from, to *types.NodeID) string {
	t.Helper()
	for i := 0; i < 100000; i++ {
		key := fmt.Sprintf("key-%d", i)
		if types.NewHashedNodeID([]byte(key)).IsBetween(from, to) {
			return key
		}
	}
	require.FailNow(t, "no key found in the range")
	return ""
}

func TestOperator_setGetDelete(t *testing.T) {
	o := newEchoOperator()
	// the tail equal to the head: the sector covers the whole ring
	require.NoError(t, o.SetRange(*testHead))

	// a write returns after it is applied, so the next read sees it
	require.NoError(t, o.Set("key", []byte("value1")))
	value, err := o.Get("key")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)

	require.NoError(t, o.Set("key", []byte("value2")))
	value, err = o.Get("key")
	require.NoError(t, err)
	require.Equal(t, []byte("value2"), value)

	_, err = o.Get("missing")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	require.NoError(t, o.Delete("key"))
	_, err = o.Get("key")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)
	require.ErrorIs(t, o.Delete("key"), kvsTypes.ErrorStoreKeyNotFound)
}

// TestOperator_notReady covers the cases where the sector does not accept an
// operation and tells the sender to send it again.
func TestOperator_notReady(t *testing.T) {
	// the first half of the ring from the head, and the second half
	nearKey := findKey(t, testHead, testMiddle)
	farKey := findKey(t, testMiddle, testHead)

	tests := []struct {
		name  string
		setup func(o *Operator)
		key   string
		// expectRead is whether a read of the key is accepted.
		expectRead bool
		// expectWrite is whether a write of the key is accepted.
		expectWrite bool
	}{
		{
			name:  "the sector is not active",
			setup: func(o *Operator) {},
			key:   nearKey,
		},
		{
			name: "the key is in the range",
			setup: func(o *Operator) {
				require.NoError(t, o.SetRange(*testMiddle))
			},
			key:         nearKey,
			expectRead:  true,
			expectWrite: true,
		},
		{
			name: "the key is out of the range",
			setup: func(o *Operator) {
				require.NoError(t, o.SetRange(*testMiddle))
			},
			key: farKey,
		},
		{
			// the records can still be read while the sector is being merged
			name: "the sector has a prepare_merge claim",
			setup: func(o *Operator) {
				require.NoError(t, o.SetRange(*testHead))
				o.SetMergeFence(true)
			},
			key:        nearKey,
			expectRead: true,
		},
		{
			name: "the prepare_merge claim is released",
			setup: func(o *Operator) {
				require.NoError(t, o.SetRange(*testHead))
				o.SetMergeFence(true)
				o.SetMergeFence(false)
			},
			key:         nearKey,
			expectRead:  true,
			expectWrite: true,
		},
		{
			name: "the key is in the range that a split hands over",
			setup: func(o *Operator) {
				require.NoError(t, o.SetRange(*testHead))
				o.SetSplitting(testMiddle)
			},
			key:        farKey,
			expectRead: true,
		},
		{
			name: "the key is in the range that a split keeps",
			setup: func(o *Operator) {
				require.NoError(t, o.SetRange(*testHead))
				o.SetSplitting(testMiddle)
			},
			key:         nearKey,
			expectRead:  true,
			expectWrite: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := newEchoOperator()
			tt.setup(o)

			_, err := o.Get(tt.key)
			if tt.expectRead {
				require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)
			} else {
				require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
			}

			err = o.Set(tt.key, []byte("value"))
			if tt.expectWrite {
				require.NoError(t, err)
				require.NoError(t, o.Delete(tt.key))
			} else {
				require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
				require.ErrorIs(t, o.Delete(tt.key), kvsTypes.ErrorSectorNotReady)
			}
		})
	}
}

// TestOperator_timeout covers a write whose operation is never applied, as
// in a group that lost its quorum: it must return, and leave no waiter.
func TestOperator_timeout(t *testing.T) {
	o, _ := newPendingOperator()
	o.operationTimeout = 100 * time.Millisecond
	require.NoError(t, o.SetRange(*testHead))

	require.ErrorIs(t, o.Set("key", []byte("value")), ErrOperationTimeout)

	o.mtx.RLock()
	defer o.mtx.RUnlock()
	require.Empty(t, o.waiters)
}

// TestOperator_applyChecksRange covers an operation that was accepted, and
// then applied after a split moved its key out of the range: it must not be
// stored, and the write must be told to send it again.
func TestOperator_applyChecksRange(t *testing.T) {
	o, proposals := newPendingOperator()
	require.NoError(t, o.SetRange(*testHead))
	movedKey := findKey(t, testMiddle, testHead)

	result := make(chan error, 1)
	go func() { result <- o.Set(movedKey, []byte("value")) }()
	operation := <-proposals

	require.NoError(t, o.SetRange(*testMiddle))
	require.NoError(t, o.ApplyProposal(operation))

	require.ErrorIs(t, <-result, kvsTypes.ErrorSectorNotReady)
	records, err := o.ExportAllRecords()
	require.NoError(t, err)
	require.Empty(t, records)
}

// TestOperator_applyChecksMergeFence covers an operation that was accepted,
// and then applied after the sector got a prepare_merge claim. The node that
// merges the sector exports the records as of the claim, so the operation
// must not be stored, and the write must be told to send it again.
func TestOperator_applyChecksMergeFence(t *testing.T) {
	o, proposals := newPendingOperator()
	require.NoError(t, o.SetRange(*testHead))

	result := make(chan error, 1)
	go func() { result <- o.Set("key", []byte("value")) }()
	operation := <-proposals

	o.SetMergeFence(true)
	require.NoError(t, o.ApplyProposal(operation))

	require.ErrorIs(t, <-result, kvsTypes.ErrorSectorNotReady)
	records, err := o.ExportAllRecords()
	require.NoError(t, err)
	require.Empty(t, records)
}

// TestOperator_setSplittingWaitsForPending covers the writes accepted before
// a split: the split exports the records right after SetSplitting returns, so
// SetSplitting must wait until those writes are applied.
func TestOperator_setSplittingWaitsForPending(t *testing.T) {
	o, proposals := newPendingOperator()
	require.NoError(t, o.SetRange(*testHead))
	movedKey := findKey(t, testMiddle, testHead)

	result := make(chan error, 1)
	go func() { result <- o.Set(movedKey, []byte("value")) }()
	operation := <-proposals

	returned := make(chan struct{})
	go func() {
		o.SetSplitting(testMiddle)
		close(returned)
	}()

	select {
	case <-returned:
		require.FailNow(t, "SetSplitting returned while a write was pending")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, o.ApplyProposal(operation))
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "SetSplitting did not return after the write was applied")
	}

	require.NoError(t, <-result)
	records, err := o.ExportAllRecords()
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{movedKey: []byte("value")}, records)
}

// TestOperator_setSplittingIgnoresLaterWrites covers the writes accepted
// after a split started. They are out of the range that the split exports,
// so SetSplitting must not wait for them: a sector that is written all the
// time would otherwise hold every split for the whole timeout.
func TestOperator_setSplittingIgnoresLaterWrites(t *testing.T) {
	o, proposals := newPendingOperator()
	require.NoError(t, o.SetRange(*testHead))
	movedKey := findKey(t, testMiddle, testHead)
	keptKey := findKey(t, testHead, testMiddle)

	movedResult := make(chan error, 1)
	go func() { movedResult <- o.Set(movedKey, []byte("value")) }()
	movedOperation := <-proposals

	returned := make(chan struct{})
	go func() {
		o.SetSplitting(testMiddle)
		close(returned)
	}()
	require.Eventually(t, func() bool {
		o.mtx.RLock()
		defer o.mtx.RUnlock()
		return o.splittingAddress != nil
	}, 5*time.Second, time.Millisecond)

	// a write accepted after the split started, and still pending
	keptResult := make(chan error, 1)
	go func() { keptResult <- o.Set(keptKey, []byte("value")) }()
	keptOperation := <-proposals

	require.NoError(t, o.ApplyProposal(movedOperation))
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "SetSplitting waited for a write accepted after it")
	}

	require.NoError(t, o.ApplyProposal(keptOperation))
	require.NoError(t, <-movedResult)
	require.NoError(t, <-keptResult)
}
