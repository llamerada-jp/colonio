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

// fakeBackend scripts one response per call in order; when the script is
// exhausted or an entry is nil/hang, the response channel never resolves
// (emulating a request lost in flight).
type fakeBackend struct {
	mtx        sync.Mutex
	getResults []*kvsTypes.GetResult
	setErrors  []*error // nil entry = never respond
	delErrors  []*error
	getCalls   int
	setCalls   int
	delCalls   int
}

func (f *fakeBackend) Get(key string) chan *kvsTypes.GetResult {
	c := make(chan *kvsTypes.GetResult, 1)
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.getCalls++
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

func respondErr(script *[]*error, calls *int, mtx *sync.Mutex) chan error {
	c := make(chan error, 1)
	mtx.Lock()
	defer mtx.Unlock()
	*calls++
	if len(*script) == 0 {
		return c // hang
	}
	entry := (*script)[0]
	*script = (*script)[1:]
	if entry == nil {
		return c // hang
	}
	c <- *entry
	close(c)
	return c
}

func (f *fakeBackend) Set(key string, value []byte) chan error {
	return respondErr(&f.setErrors, &f.setCalls, &f.mtx)
}

func (f *fakeBackend) Delete(key string) chan error {
	return respondErr(&f.delErrors, &f.delCalls, &f.mtx)
}

func errP(err error) *error { return &err }

func TestClientGet(t *testing.T) {
	backend := &fakeBackend{getResults: []*kvsTypes.GetResult{
		{Data: []byte("value")},
	}}
	client := NewClient(backend)

	res, err := client.Get(t.Context(), "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), res.Value)
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

func TestClientSetRetriesPreparing(t *testing.T) {
	backend := &fakeBackend{setErrors: []*error{
		errP(kvsTypes.ErrorSectorNotReady),
		errP(kvsTypes.ErrorSectorNotReady),
		errP(nil),
	}}
	client := NewClient(backend)

	_, err := client.Set(t.Context(), "key", []byte("value"))
	require.NoError(t, err)
	assert.Equal(t, 3, backend.setCalls)
}

func TestClientSetPreparingDeadline(t *testing.T) {
	backend := &fakeBackend{setErrors: []*error{
		errP(kvsTypes.ErrorSectorNotReady),
		errP(kvsTypes.ErrorSectorNotReady),
		errP(kvsTypes.ErrorSectorNotReady),
		errP(kvsTypes.ErrorSectorNotReady),
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
	backend := &fakeBackend{setErrors: []*error{
		errP(kvsTypes.ErrorSectorNotReady),
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

func TestClientDeleteNotFound(t *testing.T) {
	backend := &fakeBackend{delErrors: []*error{
		errP(kvsTypes.ErrorStoreKeyNotFound),
	}}
	client := NewClient(backend)

	err := client.Delete(t.Context(), "key")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, 1, backend.delCalls)
}

func TestClientGetRetriesPreparing(t *testing.T) {
	backend := &fakeBackend{getResults: []*kvsTypes.GetResult{
		{Err: kvsTypes.ErrorSectorNotReady},
		{Data: []byte("value")},
	}}
	client := NewClient(backend)

	res, err := client.Get(t.Context(), "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), res.Value)
	assert.Equal(t, 2, backend.getCalls)
}

func TestClientUnknownResponseCode(t *testing.T) {
	backend := &fakeBackend{setErrors: []*error{
		errP(kvsTypes.ErrorOperationResultUnknown),
	}}
	client := NewClient(backend)

	_, err := client.Set(t.Context(), "key", []byte("value"))
	// result-unknown must never be blindly retried without CAS
	assert.ErrorIs(t, err, ErrResultUnknown)
	assert.Equal(t, 1, backend.setCalls)
}
