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
package kvs

import (
	"context"
	"sync"
	"testing"
	"time"

	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// casParams records the CAS / guard condition a backend call carried.
type casParams struct {
	revision uint64
	absent   bool
	lockGen  uint64
}

// fakeBackend scripts one response per call in order; when the script is
// exhausted or an entry is nil, the response channel never resolves
// (emulating a request lost in flight).
type fakeBackend struct {
	mtx             sync.Mutex
	getResults      []*kvsTypes.GetResult
	setResults      []*kvsTypes.SetResult
	patchResults    []*kvsTypes.SetResult
	delErrors       []*error
	getCalls        int
	setCalls        int
	patchCalls      int
	delCalls        int
	setCas          []casParams
	patchCas        []casParams
	delCas          []casParams
	gotWithoutValue []bool
	gotPatcher      []string
	acquireResults  []*kvsTypes.LockResult
	releaseErrors   []*error
	acquireCalls    []lockCall
	releaseCalls    []lockCall
}

func (f *fakeBackend) Get(key string, withoutValue bool) chan *kvsTypes.GetResult {
	c := make(chan *kvsTypes.GetResult, 1)
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.getCalls++
	f.gotWithoutValue = append(f.gotWithoutValue, withoutValue)
	if len(f.getResults) == 0 {
		return c // hang
	}
	result := f.getResults[0]
	f.getResults = f.getResults[1:]
	if result == nil {
		return c // hang
	}
	c <- result
	close(c)
	return c
}

func (f *fakeBackend) Patch(key string, patcher string, patch []byte, casRevision uint64, casAbsent bool, lockGeneration uint64) chan *kvsTypes.SetResult {
	c := make(chan *kvsTypes.SetResult, 1)
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.patchCalls++
	f.gotPatcher = append(f.gotPatcher, patcher)
	f.patchCas = append(f.patchCas, casParams{revision: casRevision, absent: casAbsent, lockGen: lockGeneration})
	if len(f.patchResults) == 0 {
		return c // hang
	}
	result := f.patchResults[0]
	f.patchResults = f.patchResults[1:]
	if result == nil {
		return c // hang
	}
	c <- result
	close(c)
	return c
}

func (f *fakeBackend) Set(key string, value []byte, casRevision uint64, casAbsent bool, lockGeneration uint64) chan *kvsTypes.SetResult {
	c := make(chan *kvsTypes.SetResult, 1)
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.setCalls++
	f.setCas = append(f.setCas, casParams{revision: casRevision, absent: casAbsent, lockGen: lockGeneration})
	if len(f.setResults) == 0 {
		return c // hang
	}
	result := f.setResults[0]
	f.setResults = f.setResults[1:]
	if result == nil {
		return c // hang
	}
	c <- result
	close(c)
	return c
}

func (f *fakeBackend) Delete(key string, casRevision uint64, casAbsent bool, lockGeneration uint64) chan error {
	c := make(chan error, 1)
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.delCalls++
	f.delCas = append(f.delCas, casParams{revision: casRevision, absent: casAbsent, lockGen: lockGeneration})
	if len(f.delErrors) == 0 {
		return c // hang
	}
	entry := f.delErrors[0]
	f.delErrors = f.delErrors[1:]
	if entry == nil {
		return c // hang
	}
	c <- *entry
	close(c)
	return c
}

type lockCall struct {
	key   string
	ttlMS uint64
	gen   uint64
}

func (f *fakeBackend) LockAcquire(key string, ttlMS uint64) chan *kvsTypes.LockResult {
	c := make(chan *kvsTypes.LockResult, 1)
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.acquireCalls = append(f.acquireCalls, lockCall{key: key, ttlMS: ttlMS})
	if len(f.acquireResults) == 0 {
		return c // hang
	}
	result := f.acquireResults[0]
	f.acquireResults = f.acquireResults[1:]
	if result == nil {
		return c // hang
	}
	c <- result
	close(c)
	return c
}

func (f *fakeBackend) LockRelease(key string, generation uint64) chan error {
	c := make(chan error, 1)
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.releaseCalls = append(f.releaseCalls, lockCall{key: key, gen: generation})
	if len(f.releaseErrors) == 0 {
		return c // hang
	}
	entry := f.releaseErrors[0]
	f.releaseErrors = f.releaseErrors[1:]
	if entry == nil {
		return c // hang
	}
	c <- *entry
	close(c)
	return c
}

func errP(err error) *error { return &err }

func TestClientGet(t *testing.T) {
	backend := &fakeBackend{getResults: []*kvsTypes.GetResult{
		{Data: []byte("value"), Revision: 7},
	}}
	client := NewClient(backend)

	res, err := client.Get(t.Context(), "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), res.Value)
	assert.Equal(t, uint64(7), res.Revision)
	assert.Equal(t, 1, backend.getCalls)
}

func TestClientGetNotFound(t *testing.T) {
	backend := &fakeBackend{getResults: []*kvsTypes.GetResult{
		{Err: kvsTypes.ErrorStoreKeyNotFound},
	}}
	client := NewClient(backend)

	_, err := client.Get(t.Context(), "key")
	assert.ErrorIs(t, err, ErrNotFound)
	// a miss is a final answer, not retried
	assert.Equal(t, 1, backend.getCalls)
}

func TestClientGetRetriesPreparing(t *testing.T) {
	backend := &fakeBackend{getResults: []*kvsTypes.GetResult{
		{Err: kvsTypes.ErrorSectorNotReady},
		{Data: []byte("value"), Revision: 1},
	}}
	client := NewClient(backend)

	res, err := client.Get(t.Context(), "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), res.Value)
	assert.Equal(t, 2, backend.getCalls)
}

func TestClientSetReturnsRevision(t *testing.T) {
	backend := &fakeBackend{setResults: []*kvsTypes.SetResult{
		{Revision: 42},
	}}
	client := NewClient(backend)

	res, err := client.Set(t.Context(), "key", []byte("value"))
	require.NoError(t, err)
	assert.Equal(t, uint64(42), res.Revision)
	// unconditional write carries no CAS condition
	assert.Equal(t, casParams{}, backend.setCas[0])
}

func TestClientSetRetriesPreparing(t *testing.T) {
	backend := &fakeBackend{setResults: []*kvsTypes.SetResult{
		{Err: kvsTypes.ErrorSectorNotReady},
		{Err: kvsTypes.ErrorSectorNotReady},
		{Revision: 1},
	}}
	client := NewClient(backend)

	_, err := client.Set(t.Context(), "key", []byte("value"))
	require.NoError(t, err)
	assert.Equal(t, 3, backend.setCalls)
}

func TestClientSetPreparingDeadline(t *testing.T) {
	backend := &fakeBackend{setResults: []*kvsTypes.SetResult{
		{Err: kvsTypes.ErrorSectorNotReady},
		{Err: kvsTypes.ErrorSectorNotReady},
		{Err: kvsTypes.ErrorSectorNotReady},
		{Err: kvsTypes.ErrorSectorNotReady},
	}}
	client := NewClient(backend)

	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	_, err := client.Set(ctx, "key", []byte("value"))
	// PREPARING is a pre-acceptance rejection: the caller can tell nothing
	// was applied (ErrPreparing) and that the deadline cut the retry short.
	assert.ErrorIs(t, err, ErrPreparing)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotErrorIs(t, err, ErrResultUnknown)
}

func TestClientSetWithoutRetry(t *testing.T) {
	backend := &fakeBackend{setResults: []*kvsTypes.SetResult{
		{Err: kvsTypes.ErrorSectorNotReady},
	}}
	client := NewClient(backend)

	_, err := client.Set(t.Context(), "key", []byte("value"), WithoutRetry())
	assert.ErrorIs(t, err, ErrPreparing)
	assert.Equal(t, 1, backend.setCalls)
}

func TestClientSetInFlightDeadline(t *testing.T) {
	// no scripted response: the request hangs in flight
	backend := &fakeBackend{}
	client := NewClient(backend)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := client.Set(ctx, "key", []byte("value"))
	// the proposal may still commit → outcome must be marked unknown
	assert.ErrorIs(t, err, ErrResultUnknown)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, backend.setCalls)
}

func TestClientSetUnconditionalUnknownNotRetried(t *testing.T) {
	backend := &fakeBackend{setResults: []*kvsTypes.SetResult{
		{Err: kvsTypes.ErrorOperationResultUnknown},
	}}
	client := NewClient(backend)

	_, err := client.Set(t.Context(), "key", []byte("value"))
	// without a CAS condition, an unknown-outcome retry could double-apply
	assert.ErrorIs(t, err, ErrResultUnknown)
	assert.Equal(t, 1, backend.setCalls)
}

func TestClientSetCasRetriesUnknown(t *testing.T) {
	backend := &fakeBackend{setResults: []*kvsTypes.SetResult{
		{Err: kvsTypes.ErrorOperationResultUnknown},
		{Revision: 6},
	}}
	client := NewClient(backend)

	res, err := client.Set(t.Context(), "key", []byte("value"), WithRevision(5))
	// the CAS condition makes the retry at-most-once, so unknown is retried
	require.NoError(t, err)
	assert.Equal(t, uint64(6), res.Revision)
	assert.Equal(t, 2, backend.setCalls)
	assert.Equal(t, casParams{revision: 5}, backend.setCas[0])
	assert.Equal(t, casParams{revision: 5}, backend.setCas[1])
}

func TestClientSetConflictNotRetried(t *testing.T) {
	backend := &fakeBackend{setResults: []*kvsTypes.SetResult{
		{Err: kvsTypes.ErrorCasConflict},
	}}
	client := NewClient(backend)

	_, err := client.Set(t.Context(), "key", []byte("value"), WithRevision(5))
	// a conflict is a definite answer: re-read and decide, never blind-retry
	assert.ErrorIs(t, err, ErrConflict)
	assert.Equal(t, 1, backend.setCalls)
}

func TestClientSetWithAbsent(t *testing.T) {
	backend := &fakeBackend{setResults: []*kvsTypes.SetResult{
		{Revision: 1},
	}}
	client := NewClient(backend)

	_, err := client.Set(t.Context(), "key", []byte("value"), WithAbsent())
	require.NoError(t, err)
	assert.Equal(t, casParams{absent: true}, backend.setCas[0])
}

func TestClientWriteOptionValidation(t *testing.T) {
	backend := &fakeBackend{}
	client := NewClient(backend)

	// a zero revision would silently turn the condition off on the wire
	_, err := client.Set(t.Context(), "key", []byte("value"), WithRevision(0))
	assert.Error(t, err)

	_, err = client.Set(t.Context(), "key", []byte("value"), WithRevision(1), WithAbsent())
	assert.Error(t, err)

	err = client.Delete(t.Context(), "key", WithRevision(0))
	assert.Error(t, err)

	// no attempt must reach the backend on a misuse error
	assert.Equal(t, 0, backend.setCalls)
	assert.Equal(t, 0, backend.delCalls)
}

func TestClientDeleteNotFound(t *testing.T) {
	backend := &fakeBackend{delErrors: []*error{
		errP(kvsTypes.ErrorStoreKeyNotFound),
	}}
	client := NewClient(backend)

	err := client.Delete(t.Context(), "key")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, 1, backend.delCalls)
}

func TestClientDeleteCasConflict(t *testing.T) {
	backend := &fakeBackend{delErrors: []*error{
		errP(kvsTypes.ErrorCasConflict),
	}}
	client := NewClient(backend)

	err := client.Delete(t.Context(), "key", WithRevision(3))
	assert.ErrorIs(t, err, ErrConflict)
	assert.Equal(t, casParams{revision: 3}, backend.delCas[0])
}

func TestClientGetWithoutValue(t *testing.T) {
	backend := &fakeBackend{getResults: []*kvsTypes.GetResult{
		{Revision: 9}, // the host already omitted the value
	}}
	client := NewClient(backend)

	res, err := client.Get(t.Context(), "key", WithoutValue())
	require.NoError(t, err)
	assert.Nil(t, res.Value)
	assert.Equal(t, uint64(9), res.Revision)
	assert.Equal(t, []bool{true}, backend.gotWithoutValue)
}

func TestClientPatch(t *testing.T) {
	backend := &fakeBackend{patchResults: []*kvsTypes.SetResult{
		{Revision: 12},
	}}
	client := NewClient(backend)

	res, err := client.Patch(t.Context(), "key", "inc", []byte("patch"))
	require.NoError(t, err)
	assert.Equal(t, uint64(12), res.Revision)
	assert.Equal(t, []string{"inc"}, backend.gotPatcher)
	assert.Equal(t, casParams{}, backend.patchCas[0])
}

func TestClientPatchValidation(t *testing.T) {
	backend := &fakeBackend{}
	client := NewClient(backend)

	// a patch requires the record to exist, so WithAbsent is a misuse
	_, err := client.Patch(t.Context(), "key", "inc", []byte("patch"), WithAbsent())
	assert.Error(t, err)

	_, err = client.Patch(t.Context(), "key", "", []byte("patch"))
	assert.Error(t, err)

	assert.Equal(t, 0, backend.patchCalls)
}

func TestClientPatchFailedNotRetried(t *testing.T) {
	backend := &fakeBackend{patchResults: []*kvsTypes.SetResult{
		{Err: kvsTypes.ErrorPatchFailed},
	}}
	client := NewClient(backend)

	_, err := client.Patch(t.Context(), "key", "inc", []byte("patch"))
	assert.ErrorIs(t, err, ErrPatchFailed)
	assert.Equal(t, 1, backend.patchCalls)
}

func TestClientPatchUnknownRetryOnlyWithRevision(t *testing.T) {
	// bare patch: non-idempotent, unknown outcome must surface
	backend := &fakeBackend{patchResults: []*kvsTypes.SetResult{
		{Err: kvsTypes.ErrorOperationResultUnknown},
	}}
	client := NewClient(backend)
	_, err := client.Patch(t.Context(), "key", "inc", []byte("patch"))
	assert.ErrorIs(t, err, ErrResultUnknown)
	assert.Equal(t, 1, backend.patchCalls)

	// conditional patch: the CAS makes the retry at-most-once
	backend = &fakeBackend{patchResults: []*kvsTypes.SetResult{
		{Err: kvsTypes.ErrorOperationResultUnknown},
		{Revision: 8},
	}}
	client = NewClient(backend)
	res, err := client.Patch(t.Context(), "key", "inc", []byte("patch"), WithRevision(7))
	require.NoError(t, err)
	assert.Equal(t, uint64(8), res.Revision)
	assert.Equal(t, 2, backend.patchCalls)
}
