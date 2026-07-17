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
	"testing"
	"time"

	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func watchStates(states ...*kvsTypes.WatchState) []*kvsTypes.WatchSubscribeResult {
	results := make([]*kvsTypes.WatchSubscribeResult, 0, len(states))
	for _, state := range states {
		if state == nil {
			results = append(results, nil) // hang
			continue
		}
		results = append(results, &kvsTypes.WatchSubscribeResult{State: *state})
	}
	return results
}

func recvEvent(t *testing.T, w *Watcher) WatchEvent {
	t.Helper()
	select {
	case event, ok := <-w.Events():
		require.True(t, ok, "events channel closed unexpectedly")
		return event
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for a watch event")
		return WatchEvent{}
	}
}

func expectNoEvent(t *testing.T, w *Watcher, wait time.Duration) {
	t.Helper()
	select {
	case event, ok := <-w.Events():
		if ok {
			require.FailNowf(t, "unexpected event", "%+v", event)
		}
	case <-time.After(wait):
	}
}

func TestWatcherInitialState(t *testing.T) {
	backend := &fakeBackend{subscribeResults: watchStates(
		&kvsTypes.WatchState{Exists: true, Revision: 5, Value: []byte("v5")},
	)}
	client := NewClient(backend)

	w, err := client.Watch(t.Context(), "key")
	require.NoError(t, err)

	event := recvEvent(t, w)
	assert.Equal(t, "key", event.Key)
	assert.Equal(t, []byte("v5"), event.Value)
	assert.Equal(t, uint64(5), event.Revision)
	assert.False(t, event.Deleted)
}

func TestWatcherInitialAbsentDeliversDeleted(t *testing.T) {
	backend := &fakeBackend{subscribeResults: watchStates(
		&kvsTypes.WatchState{Exists: false},
	)}
	client := NewClient(backend)

	w, err := client.Watch(t.Context(), "key")
	require.NoError(t, err)

	event := recvEvent(t, w)
	assert.True(t, event.Deleted)
	assert.Nil(t, event.Value)
}

func TestWatcherSinceRevisionSuppressesUnchanged(t *testing.T) {
	backend := &fakeBackend{subscribeResults: watchStates(
		&kvsTypes.WatchState{Exists: true, Revision: 5, ValueOmitted: true},
	)}
	client := NewClient(backend)

	w, err := client.Watch(t.Context(), "key", WithSinceRevision(5))
	require.NoError(t, err)

	// the caller already has revision 5: nothing to deliver
	expectNoEvent(t, w, 300*time.Millisecond)

	// a change arrives as a pushed event
	backend.push(&kvsTypes.WatchPush{Key: "key", Value: []byte("v6"), Revision: 6})
	event := recvEvent(t, w)
	assert.Equal(t, uint64(6), event.Revision)
	assert.Equal(t, []byte("v6"), event.Value)
}

func TestWatcherPushDedupAndOrdering(t *testing.T) {
	backend := &fakeBackend{subscribeResults: watchStates(
		&kvsTypes.WatchState{Exists: true, Revision: 5, Value: []byte("v5")},
	)}
	client := NewClient(backend)

	w, err := client.Watch(t.Context(), "key")
	require.NoError(t, err)
	require.Equal(t, uint64(5), recvEvent(t, w).Revision)

	// duplicate of the delivered state and an older push: both dropped
	backend.push(&kvsTypes.WatchPush{Key: "key", Value: []byte("v5"), Revision: 5})
	backend.push(&kvsTypes.WatchPush{Key: "key", Value: []byte("v4"), Revision: 4})
	expectNoEvent(t, w, 300*time.Millisecond)

	// deletion of the current revision IS newer (same revision, deleted flag)
	backend.push(&kvsTypes.WatchPush{Key: "key", Revision: 5, Deleted: true})
	event := recvEvent(t, w)
	assert.True(t, event.Deleted)
	assert.Equal(t, uint64(5), event.Revision)

	// duplicated deletion: dropped
	backend.push(&kvsTypes.WatchPush{Key: "key", Revision: 5, Deleted: true})
	expectNoEvent(t, w, 300*time.Millisecond)
}

func TestWatcherLockTransitionIsNotDeliveredButSignals(t *testing.T) {
	backend := &fakeBackend{subscribeResults: watchStates(
		&kvsTypes.WatchState{Exists: true, Revision: 5, Value: []byte("v5"), Locked: true},
	)}
	client := NewClient(backend)

	w, err := client.Watch(t.Context(), "key")
	require.NoError(t, err)
	require.Equal(t, uint64(5), recvEvent(t, w).Revision)

	// a pure lease release keeps the revision: no value event, but the lock
	// waiters' signal fires
	backend.push(&kvsTypes.WatchPush{Key: "key", Value: []byte("v5"), Revision: 5, Locked: false})
	select {
	case <-w.lockFree:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "lock-free signal did not fire")
	}
	expectNoEvent(t, w, 300*time.Millisecond)
}

func TestWatcherResyncRecoversFromCounterReset(t *testing.T) {
	backend := &fakeBackend{subscribeResults: watchStates(
		&kvsTypes.WatchState{Exists: true, Revision: 100, Value: []byte("high")},
		// the next keepalive resync reports a REGRESSED revision (sector data
		// loss reset the counter); the authoritative state must be delivered
		&kvsTypes.WatchState{Exists: true, Revision: 3, Value: []byte("reset")},
	)}
	client := NewClient(backend)

	w, err := client.Watch(t.Context(), "key")
	require.NoError(t, err)
	require.Equal(t, uint64(100), recvEvent(t, w).Revision)

	// pushes older than the delivered state are dropped...
	backend.push(&kvsTypes.WatchPush{Key: "key", Value: []byte("reset"), Revision: 3})
	expectNoEvent(t, w, 300*time.Millisecond)

	// ...but the periodic resync (watchKeepaliveInterval) recovers.
	event := recvEvent(t, w)
	assert.Equal(t, uint64(3), event.Revision)
	assert.Equal(t, []byte("reset"), event.Value)
}

func TestWatcherAbsentResyncAfterDeletedPushIsSuppressed(t *testing.T) {
	backend := &fakeBackend{subscribeResults: watchStates(
		&kvsTypes.WatchState{Exists: true, Revision: 5, Value: []byte("v5")},
		&kvsTypes.WatchState{Exists: false},
	)}
	client := NewClient(backend)

	w, err := client.Watch(t.Context(), "key")
	require.NoError(t, err)
	require.Equal(t, uint64(5), recvEvent(t, w).Revision)

	backend.push(&kvsTypes.WatchPush{Key: "key", Revision: 5, Deleted: true})
	require.True(t, recvEvent(t, w).Deleted)

	// keepalive resync confirming the absence must not re-deliver it
	expectNoEvent(t, w, watchKeepaliveInterval+time.Second)
}

func TestWatcherContextEndClosesEvents(t *testing.T) {
	backend := &fakeBackend{subscribeResults: watchStates(
		&kvsTypes.WatchState{Exists: false},
	)}
	client := NewClient(backend)

	ctx, cancel := context.WithCancel(t.Context())
	w, err := client.Watch(ctx, "key")
	require.NoError(t, err)
	require.True(t, recvEvent(t, w).Deleted)

	cancel()
	select {
	case _, ok := <-w.Events():
		require.False(t, ok)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "events channel did not close")
	}
	assert.ErrorIs(t, w.Err(), context.Canceled)

	// the subscription was cancelled and the sink unregistered
	backend.mtx.Lock()
	defer backend.mtx.Unlock()
	assert.NotEmpty(t, backend.cancelCalls)
	assert.Empty(t, backend.sinks)
}

func TestWatcherKeepsRetryingWhilePreparing(t *testing.T) {
	backend := &fakeBackend{subscribeResults: []*kvsTypes.WatchSubscribeResult{
		{Err: kvsTypes.ErrorSectorNotReady},
		{Err: kvsTypes.ErrorSectorNotReady},
		{State: kvsTypes.WatchState{Exists: true, Revision: 2, Value: []byte("v2")}},
	}}
	client := NewClient(backend)

	w, err := client.Watch(t.Context(), "key")
	require.NoError(t, err)

	event := recvEvent(t, w)
	assert.Equal(t, uint64(2), event.Revision)
	backend.mtx.Lock()
	defer backend.mtx.Unlock()
	assert.GreaterOrEqual(t, len(backend.subscribeCalls), 3)
}
