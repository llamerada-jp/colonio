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
package types

import (
	"fmt"
)

var (
	ErrorStoreInvalidNodeKey = fmt.Errorf("invalid node key")
	ErrorStoreKeyNotFound    = fmt.Errorf("key not found")
	// ErrorSectorNotReady is a retryable rejection of a KVS operation: the
	// sector is not activated yet, the key is outside the sector's current
	// range (stale routing, or a split/merge just moved it), or the range is
	// being handed over (split export in progress / merge lock held). Mapped
	// to KvsOperationResponse ERROR_PREPARING so the client can retry.
	ErrorSectorNotReady = fmt.Errorf("sector is not ready for the operation")
	// ErrorOperationResultUnknown marks a write whose outcome is genuinely
	// unknown: the proposal timed out on the host (or the response carried an
	// unexpected code), but raft gives no negative acknowledgment, so it may
	// still commit later. Retrying blindly can apply the write twice; the
	// public client (node/kvs) only auto-retries this class for conditional
	// (CAS) operations.
	ErrorOperationResultUnknown = fmt.Errorf("operation result is unknown")
	// ErrorCasConflict means a conditional write (expected revision / expected
	// absence) found a different state at apply time. The store was left
	// untouched. Not blindly retryable: re-read the record and decide.
	ErrorCasConflict = fmt.Errorf("compare-and-swap conflict")
	// ErrorPatchFailed means a Patch could not be applied: the named Patcher
	// is not registered on the handling node, or it rejected the patch
	// document. The store was left untouched; this is a definite outcome.
	ErrorPatchFailed = fmt.Errorf("patch failed")
	// ErrorLockHeld means the record's lease lock is held by another owner:
	// a lock acquisition or an unguarded write was rejected. Definite
	// outcome; wait (for the lease to be released or revoked) and retry.
	ErrorLockHeld = fmt.Errorf("lock is held by another owner")
)

// Patcher applies a partial-update document to a record value. Applications
// register Patchers per format name (node.WithKvsPatcher); the patch document
// travels through raft and Apply runs INSIDE the apply on every replica of
// the record's sector. That imposes two hard requirements (spec/kvs/api.md
// 「Patch」):
//
//   - Apply must be a deterministic pure function: identical (current, patch)
//     must produce identical bytes on every replica and on every call. No
//     clocks, randomness, environment, or global state; beware of
//     re-serialization that does not guarantee canonical output. A
//     non-deterministic Patcher silently diverges the replicated state.
//     Use patchertest.AssertDeterministic in the application's tests.
//   - Every node of the cluster must register the same name with the same
//     behavior. Roll out a new or changed Patcher to every node BEFORE the
//     first use.
//
// A returned error rejects the patch for the requesting client and leaves the
// store untouched; the error must be just as deterministic as the result.
type Patcher interface {
	Apply(current []byte, patch []byte) ([]byte, error)
}

type Store interface {
	AllocateSector(sectorKey *SectorKey) error
	ReleaseSector(sectorKey *SectorKey) error

	Get(sectorKey *SectorKey, key string) ([]byte, error)
	Set(sectorKey *SectorKey, key string, value []byte) error
	Delete(sectorKey *SectorKey, key string) error
}

type GetResult struct {
	Data []byte
	// Revision is the record's revision, used as the expected value of a
	// conditional write (CAS). Assigned from the sector's revision counter;
	// never 0 for an existing record.
	Revision uint64
	Err      error
}

type SetResult struct {
	// Revision is the newly assigned revision of the written record.
	Revision uint64
	Err      error
}

// WatchState is the record state returned by a watch subscription
// (registration and keepalive alike): the client synthesizes a WatchEvent
// from it when it differs from the state it delivered last.
type WatchState struct {
	Exists   bool
	Revision uint64
	// Locked reflects the record's lease-lock state. Lock waiters use it (and
	// the lock transitions pushed as events) to wake up without polling.
	Locked bool
	// ValueOmitted is set when the record still has exactly the revision the
	// subscriber reported as already delivered, so the (potentially large)
	// value was not transferred back.
	ValueOmitted bool
	Value        []byte
}

type WatchSubscribeResult struct {
	State WatchState
	Err   error
}

// WatchPush is one change notification pushed by the key's host. Delivery is
// best-effort: lost pushes are recovered by the periodic keepalive resync.
type WatchPush struct {
	Key      string
	Value    []byte
	Revision uint64
	Deleted  bool
	Locked   bool
}

type LockResult struct {
	// Generation is the granted fencing token: guarded writes present it and
	// the apply rejects a stale one, so a holder that lost the lease cannot
	// corrupt the record.
	Generation uint64
	// DeadlineMS is the lease deadline in unix milliseconds on the HOST's
	// clock — informational; the client self-fences on its own monotonic
	// clock instead of trusting it.
	DeadlineMS int64
	Err        error
}
