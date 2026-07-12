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

func lockResults(results ...*kvsTypes.LockResult) []*kvsTypes.LockResult { return results }

// startRenewalTestLock builds a held Lock with a tiny TTL and runs its renewal
// loop against the fake backend (Client.Lock enforces minLockTTL, so renewal
// behavior is tested on a directly built Lock).
func startRenewalTestLock(t *testing.T, backend *fakeBackend, ttl time.Duration) *Lock {
	t.Helper()
	lock := &Lock{
		client:     NewClient(backend),
		key:        "key",
		generation: 5,
		ttl:        ttl,
		done:       make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	lock.stopRenewal = cancel
	t.Cleanup(cancel)
	go lock.renewalLoop(ctx, time.Now())
	return lock
}

func TestClientLockAcquireAndRelease(t *testing.T) {
	backend := &fakeBackend{
		acquireResults: lockResults(&kvsTypes.LockResult{Generation: 7, DeadlineMS: 123}),
		releaseErrors:  []*error{errP(nil)},
	}
	client := NewClient(backend)

	lock, err := client.Lock(t.Context(), "key")
	require.NoError(t, err)
	assert.Equal(t, uint64(7), lock.Token())
	assert.Equal(t, "key", lock.Key())
	assert.Equal(t, uint64(DefaultLockTTL.Milliseconds()), backend.acquireCalls[0].ttlMS)
	select {
	case <-lock.Done():
		t.Fatal("a healthy lock must not be done")
	default:
	}

	require.NoError(t, lock.Release(t.Context()))
	select {
	case <-lock.Done():
	default:
		t.Fatal("Done must fire after Release")
	}
	assert.NoError(t, lock.Err()) // voluntary release, not a loss
	assert.Equal(t, uint64(7), backend.releaseCalls[0].gen)
}

func TestClientLockTryOnce(t *testing.T) {
	backend := &fakeBackend{
		acquireResults: lockResults(&kvsTypes.LockResult{Err: kvsTypes.ErrorLockHeld}),
	}
	client := NewClient(backend)

	_, err := client.Lock(t.Context(), "key", WithTryOnce())
	assert.ErrorIs(t, err, ErrLockHeld)
	assert.Len(t, backend.acquireCalls, 1)
}

func TestClientLockWaitsForHolder(t *testing.T) {
	backend := &fakeBackend{
		acquireResults: lockResults(
			&kvsTypes.LockResult{Err: kvsTypes.ErrorLockHeld},
			&kvsTypes.LockResult{Err: kvsTypes.ErrorLockHeld},
			&kvsTypes.LockResult{Generation: 3},
		),
	}
	client := NewClient(backend)

	lock, err := client.Lock(t.Context(), "key")
	require.NoError(t, err)
	defer lock.stopRenewal()
	assert.Equal(t, uint64(3), lock.Token())
	assert.Len(t, backend.acquireCalls, 3)
}

func TestClientLockRetriesUnknown(t *testing.T) {
	// an unknown-outcome acquire is safe to re-send: acquiring is
	// owner-idempotent (an applied one becomes a renewal)
	backend := &fakeBackend{
		acquireResults: lockResults(
			&kvsTypes.LockResult{Err: kvsTypes.ErrorOperationResultUnknown},
			&kvsTypes.LockResult{Generation: 4},
		),
	}
	client := NewClient(backend)

	lock, err := client.Lock(t.Context(), "key")
	require.NoError(t, err)
	defer lock.stopRenewal()
	assert.Equal(t, uint64(4), lock.Token())
}

func TestClientLockTTLValidation(t *testing.T) {
	backend := &fakeBackend{}
	client := NewClient(backend)

	_, err := client.Lock(t.Context(), "key", WithTTL(time.Second))
	assert.Error(t, err)
	assert.Empty(t, backend.acquireCalls)
}

func TestClientLockRenewalKeepsLease(t *testing.T) {
	results := make([]*kvsTypes.LockResult, 0, 16)
	for range 16 {
		results = append(results, &kvsTypes.LockResult{Generation: 5})
	}
	backend := &fakeBackend{acquireResults: results}

	lock := startRenewalTestLock(t, backend, 300*time.Millisecond)
	time.Sleep(400 * time.Millisecond)
	select {
	case <-lock.Done():
		t.Fatalf("lock lost despite successful renewals: %v", lock.Err())
	default:
	}
}

func TestClientLockRenewalDetectsTakeover(t *testing.T) {
	backend := &fakeBackend{
		acquireResults: lockResults(&kvsTypes.LockResult{Err: kvsTypes.ErrorLockHeld}),
	}
	lock := startRenewalTestLock(t, backend, 300*time.Millisecond)

	select {
	case <-lock.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done must fire when the lease was taken over")
	}
	assert.ErrorIs(t, lock.Err(), ErrLockHeld)
}

func TestClientLockRenewalDetectsRegrant(t *testing.T) {
	// the lease lapsed and the renewal re-acquired it as a NEW lease: the old
	// fencing token is dead — report the loss and hand the fresh lease back
	backend := &fakeBackend{
		acquireResults: lockResults(&kvsTypes.LockResult{Generation: 9}),
		releaseErrors:  []*error{errP(nil)},
	}
	lock := startRenewalTestLock(t, backend, 300*time.Millisecond)

	select {
	case <-lock.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done must fire when the lease was re-granted")
	}
	assert.ErrorIs(t, lock.Err(), ErrConflict)
	backend.mtx.Lock()
	defer backend.mtx.Unlock()
	require.Len(t, backend.releaseCalls, 1)
	assert.Equal(t, uint64(9), backend.releaseCalls[0].gen)
}

func TestClientLockSelfFencing(t *testing.T) {
	// the backend never answers: after a full TTL without a proven renewal
	// the lock must consider itself lost, even though nothing was learned
	backend := &fakeBackend{}
	lock := startRenewalTestLock(t, backend, 300*time.Millisecond)

	select {
	case <-lock.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done must fire when renewals cannot be proven within the TTL")
	}
	assert.ErrorIs(t, lock.Err(), ErrResultUnknown)
}

func TestClientLockReleaseConflictMeansReleased(t *testing.T) {
	backend := &fakeBackend{
		acquireResults: lockResults(&kvsTypes.LockResult{Generation: 7}),
		releaseErrors:  []*error{errP(kvsTypes.ErrorCasConflict)},
	}
	client := NewClient(backend)

	lock, err := client.Lock(t.Context(), "key")
	require.NoError(t, err)
	// the lease is not ours anymore — released as far as this holder goes
	assert.NoError(t, lock.Release(t.Context()))
}

func TestClientLockGuardedWrites(t *testing.T) {
	backend := &fakeBackend{
		acquireResults: lockResults(&kvsTypes.LockResult{Generation: 7}),
		setResults:     []*kvsTypes.SetResult{{Revision: 12}},
		delErrors:      []*error{errP(nil)},
	}
	client := NewClient(backend)

	lock, err := client.Lock(t.Context(), "key")
	require.NoError(t, err)

	_, err = lock.Set(t.Context(), []byte("value"))
	require.NoError(t, err)
	assert.Equal(t, uint64(7), backend.setCas[0].lockGen)

	// guarded delete = atomic release-and-delete: Done fires without a loss
	require.NoError(t, lock.Delete(t.Context()))
	assert.Equal(t, uint64(7), backend.delCas[0].lockGen)
	select {
	case <-lock.Done():
	default:
		t.Fatal("Done must fire after the guarded delete")
	}
	assert.NoError(t, lock.Err())
}

func TestClientLockDeadlineInFlightAfterHeldReportsHeld(t *testing.T) {
	// first poll: definitely held; second poll hangs and the deadline cuts it
	// mid-flight — the caller must see contention, not an unknown outcome
	backend := &fakeBackend{
		acquireResults: lockResults(&kvsTypes.LockResult{Err: kvsTypes.ErrorLockHeld}),
	}
	client := NewClient(backend)

	ctx, cancel := context.WithTimeout(t.Context(), 1200*time.Millisecond)
	defer cancel()
	_, err := client.Lock(ctx, "key")
	assert.ErrorIs(t, err, ErrLockHeld)
	assert.NotErrorIs(t, err, ErrResultUnknown)
}
