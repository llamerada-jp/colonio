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
	proto3 "google.golang.org/protobuf/proto"
)

// ErrOperationTimeout is returned when a proposed operation was not applied
// within operationTimeout. The outcome is UNKNOWN: the proposal may still
// commit later (raft gives no negative acknowledgment), so the caller must
// not assume the write was lost.
var ErrOperationTimeout = errors.New("operation was not applied within timeout")

// WriteCondition guards a write; nil means unconditional. Every check runs at
// apply time against the record's replicated state, so it is deterministic
// across replicas (spec/kvs/lock.md).
type WriteCondition struct {
	// Revision, when non-zero, requires the record to exist with exactly this
	// revision (CAS). Revisions are assigned from the sector counter and are
	// never 0.
	Revision uint64
	// Absent requires the record to not exist.
	Absent bool
	// LockOwner + LockGeneration form the guarded-write token: when the
	// record is locked, a write must present the holder's identity and the
	// current fencing generation (spec/kvs/lock.md「guarded write」).
	LockOwner      *types.NodeID
	LockGeneration uint64
}

type Operations interface {
	Get(key string) ([]byte, uint64, error)
	Set(key string, value []byte, cond *WriteCondition) (uint64, error)
	Patch(key string, patcherName string, patch []byte, cond *WriteCondition) (uint64, error)
	Delete(key string, cond *WriteCondition) error
	LockAcquire(key string, owner *types.NodeID, ttl time.Duration) (*kvsTypes.LockResult, error)
	LockRelease(key string, owner *types.NodeID, generation uint64) error
}

// recordMarshal serializes the KvsRecord envelope. Deterministic marshaling
// is required: the encoded bytes are replicated state (stored on every
// replica and carried by snapshots), so replicas must produce identical
// bytes for identical logical records.
var recordMarshal = proto3.MarshalOptions{Deterministic: true}

func encodeRecord(value []byte, revision uint64, lock *proto.KvsLock) ([]byte, error) {
	return recordMarshal.Marshal(&proto.KvsRecord{
		Value:    value,
		Revision: revision,
		Lock:     lock,
	})
}

func decodeRecord(data []byte) (*proto.KvsRecord, error) {
	record := &proto.KvsRecord{}
	if err := proto3.Unmarshal(data, record); err != nil {
		return nil, fmt.Errorf("failed to decode record envelope: %w", err)
	}
	return record, nil
}

// applyResult is what ApplyProposal reports back to the proposing waiter.
type applyResult struct {
	err error
	// revision is the newly assigned revision for an applied SET/PATCH,
	// 0 otherwise.
	revision uint64
	// lockGeneration / lockDeadlineMS carry the granted lease of an applied
	// LOCK_ACQUIRE.
	lockGeneration uint64
	lockDeadlineMS int64
}

// lockIndexEntry is the operator's derived (non-replicated) view of one held
// lease, kept for the host's expiry scan. Rebuilt deterministically from the
// same applies on every replica; only the hosting operator acts on it.
type lockIndexEntry struct {
	owner      *proto.NodeID
	generation uint64
	deadlineMS int64
}

type Handler interface {
	OperatorProposeOperation(operation *proto.Operation)
}

type Config struct {
	SectorKey *kvsTypes.SectorKey
	Handler   Handler
	Store     kvsTypes.Store
	Head      *types.NodeID
	// Patchers is the node-wide Patcher registry (name → implementation).
	// Immutable after construction; the same map instance is shared by every
	// operator of the node.
	Patchers map[string]kvsTypes.Patcher
}

type Operator struct {
	sectorKey        kvsTypes.SectorKey
	handler          Handler
	mtx              sync.RWMutex
	store            kvsTypes.Store
	patchers         map[string]kvsTypes.Patcher
	head             types.NodeID
	tail             *types.NodeID
	splittingAddress *types.NodeID
	keys             map[string]any
	// locks is the derived index of held leases (key → lease), maintained on
	// every path that mutates record state (applies, import, replace, range
	// shrink) and consumed by the host's expiry scan (ExpiredLockRevocations).
	locks map[string]*lockIndexEntry

	// revisionCounter is replicated state: it grows by one on every applied
	// SET (the new value becomes the record's revision), max-merges with the
	// source counter on Import, and is assigned verbatim on snapshot restore.
	// It never decreases within a sector lineage, which gives per-key revision
	// monotonicity across overwrites, delete/re-create and split/merge
	// migration (spec/kvs/lock.md「revision counter」). Mutated only on the
	// apply paths, under s.mtx.
	revisionCounter uint64

	// operationTimeout bounds the wait between proposing an operation and its
	// apply. A quorum-lost group commits nothing, so the wait must not hold
	// the client's request forever.
	operationTimeout time.Duration
	// nextOperationID / waiters implement the propose→apply acknowledgment:
	// writes are only acknowledged once the local replica APPLIED the
	// committed operation, which also gives read-your-writes on the host.
	// operationIDs are instance-local: each raft group has exactly one
	// operation proposer (the hosting sector's operator, sectorNo 1), so an
	// applied ID either belongs to this map or to no waiter at all (replicas
	// and replaying joiners have empty maps).
	nextOperationID uint32
	waiters         map[uint32]chan *applyResult
	// mergeFenced rejects writes while this sector's mergeBy lock is held: the
	// range is about to be absorbed by a neighbor, and a write applied after
	// the absorber exported the records would be silently lost. Driven by the
	// sector's prepare/release-merge applies (and snapshot restore), so it is
	// deterministic across replicas even though only the host takes writes.
	mergeFenced bool
}

var _ Operations = &Operator{}

func NewOperator(config *Config) *Operator {
	return &Operator{
		sectorKey:        *config.SectorKey,
		handler:          config.Handler,
		store:            config.Store,
		patchers:         config.Patchers,
		head:             *config.Head,
		keys:             make(map[string]any),
		locks:            make(map[string]*lockIndexEntry),
		operationTimeout: 10 * time.Second,
		waiters:          make(map[uint32]chan *applyResult),
	}
}

// inRangeLocked reports whether the key belongs to the sector's current range
// [head, tail). head == tail means the sector covers the whole ring (single
// node), which IsBetween cannot express (it panics on equal bounds).
func (s *Operator) inRangeLocked(keyHash *types.NodeID) bool {
	if s.tail == nil {
		return false
	}
	if s.head.Equal(s.tail) {
		return true
	}
	return keyHash.IsBetween(&s.head, s.tail)
}

// writableLocked gates writes with the retryable ErrorSectorNotReady:
//   - the sector is not activated, or the key is out of range (stale routing,
//     or the range moved) → the client retries and lands on the right host;
//   - the key falls into the range being exported by an ongoing split
//     (SetSplitting fence) → a write accepted here could be applied after the
//     export and silently dropped by the following PreCommitSplit;
//   - the sector's merge lock is held (mergeFenced) → same hazard on the
//     merge path, the whole range is about to be absorbed.
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

// Get serves reads from the local store of the hosting replica. Writes are
// acknowledged only after the local apply, so the host observes its own
// acknowledged writes; reads may still be stale relative to operations
// committed but not yet applied locally (see spec/kvs/dataplane.md).
// It returns the record's value and revision.
func (s *Operator) Get(key string) ([]byte, uint64, error) {
	keyHash := types.NewHashedNodeID([]byte(key))

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if !s.inRangeLocked(keyHash) {
		return nil, 0, kvsTypes.ErrorSectorNotReady
	}
	if _, ok := s.keys[key]; !ok {
		return nil, 0, kvsTypes.ErrorStoreKeyNotFound
	}
	data, err := s.store.Get(&s.sectorKey, key)
	if err != nil {
		return nil, 0, err
	}
	record, err := decodeRecord(data)
	if err != nil {
		return nil, 0, err
	}
	return record.Value, record.Revision, nil
}

// Set writes the value under the key and returns the newly assigned revision.
// An existing lease lock rides along unchanged (the write must satisfy the
// guarded-write check to get here at all).
func (s *Operator) Set(key string, value []byte, cond *WriteCondition) (uint64, error) {
	result, err := s.proposeOperation(proto.Operation_COMMAND_SET, key, value, cond)
	if err != nil {
		return 0, err
	}
	return result.revision, nil
}

// Patch applies the named node-registered Patcher to the record inside the
// raft apply (the patch document is what travels, not the value). The
// registry gate here rejects an unregistered name before proposing — a
// definite misconfiguration answer; the apply re-checks it deterministically.
func (s *Operator) Patch(key string, patcherName string, patch []byte, cond *WriteCondition) (uint64, error) {
	if _, ok := s.patchers[patcherName]; !ok {
		return 0, fmt.Errorf("patcher %q is not registered on this node: %w",
			patcherName, kvsTypes.ErrorPatchFailed)
	}
	result, err := s.proposeOperationFull(&proto.Operation{
		Command: proto.Operation_COMMAND_PATCH,
		Key:     key,
		Value:   patch,
		Patcher: patcherName,
	}, cond)
	if err != nil {
		return 0, err
	}
	return result.revision, nil
}

func (s *Operator) Delete(key string, cond *WriteCondition) error {
	_, err := s.proposeOperation(proto.Operation_COMMAND_DELETE, key, nil, cond)
	return err
}

// Lease TTL clamps: the floor keeps a lease from expiring inside the ordinary
// churn windows (PREPARING can last 十数秒), the ceiling bounds how long a
// dead client can block a record.
const (
	lockTTLMin = 5 * time.Second
	lockTTLMax = time.Hour
)

// LockAcquire takes (or, for the current holder, renews) the record's lease
// lock. The deadline is computed HERE on the host's clock and travels inside
// the proposal, so the apply never reads a clock (spec/kvs/lock.md). Acquiring
// an absent key creates an empty record carrying the lock. Owner-idempotent:
// a retry of an applied acquire is a renewal that keeps the generation, so
// unknown-outcome retries are safe.
func (s *Operator) LockAcquire(key string, owner *types.NodeID, ttl time.Duration) (*kvsTypes.LockResult, error) {
	ttl = min(max(ttl, lockTTLMin), lockTTLMax)

	// Fast-path rejection without a raft proposal: while another owner's
	// unexpired lease is applied locally, the proposal could only come back
	// as LOCKED anyway. Every waiter polls its lock key about once a second,
	// so under contention these polls otherwise become a proposal storm on
	// the key's raft group (run 2026-07-12: acquire unknown-outcome 13-20%
	// steady from proposal pile-up). The applied view may lag — a lease just
	// released still looks held for one poll — which only delays the waiter
	// by a round; past the deadline (plus the revoke margin) the request
	// falls through so a takeover never depends on this gate.
	ownerProto := owner.Proto()
	s.mtx.RLock()
	if entry, ok := s.locks[key]; ok &&
		!proto3.Equal(entry.owner, ownerProto) &&
		time.Now().UnixMilli() <= entry.deadlineMS+lockRevokeMargin.Milliseconds() {
		s.mtx.RUnlock()
		return nil, kvsTypes.ErrorLockHeld
	}
	s.mtx.RUnlock()

	result, err := s.proposeOperationFull(&proto.Operation{
		Command:        proto.Operation_COMMAND_LOCK_ACQUIRE,
		Key:            key,
		LockOwner:      ownerProto,
		LockDeadlineMs: time.Now().Add(ttl).UnixMilli(),
	}, nil)
	if err != nil {
		return nil, err
	}
	return &kvsTypes.LockResult{
		Generation: result.lockGeneration,
		DeadlineMS: result.lockDeadlineMS,
	}, nil
}

// LockRelease clears the lease when (owner, generation) still matches — a
// CAS, so a release racing a newer acquisition never clears the newer lease.
// Releasing an unlocked record succeeds as a no-op (retry safety); a
// mismatch reports ErrorCasConflict (the lease is not the caller's anymore).
func (s *Operator) LockRelease(key string, owner *types.NodeID, generation uint64) error {
	_, err := s.proposeOperationFull(&proto.Operation{
		Command:        proto.Operation_COMMAND_LOCK_RELEASE,
		Key:            key,
		LockOwner:      owner.Proto(),
		LockGeneration: generation,
	}, nil)
	return err
}

// proposeOperation runs a write through the raft group and blocks until the
// committed operation is applied on this replica (or times out). The gate
// check and the waiter registration share one critical section, so a write
// accepted before a fence (SetSplitting) is always visible to the fence's
// drain loop. The propose itself runs outside the lock — sending under a held
// mutex is the Transferer self-deadlock pattern (2026-07-11).
func (s *Operator) proposeOperation(command proto.Operation_Command, key string, value []byte, cond *WriteCondition) (*applyResult, error) {
	return s.proposeOperationFull(&proto.Operation{
		Command: command,
		Key:     key,
		Value:   value,
	}, cond)
}

// proposeOperationFull is proposeOperation for a caller-built Operation
// (operation_id and the write-condition fields are filled in here).
func (s *Operator) proposeOperationFull(operation *proto.Operation, cond *WriteCondition) (*applyResult, error) {
	keyHash := types.NewHashedNodeID([]byte(operation.Key))

	s.mtx.Lock()
	if err := s.writableLocked(keyHash); err != nil {
		s.mtx.Unlock()
		return nil, err
	}
	s.nextOperationID++
	operationID := s.nextOperationID
	waiter := make(chan *applyResult, 1)
	s.waiters[operationID] = waiter
	s.mtx.Unlock()

	operation.OperationId = operationID
	if cond != nil {
		operation.CasRevision = cond.Revision
		operation.CasAbsent = cond.Absent
		if cond.LockOwner != nil {
			operation.LockOwner = cond.LockOwner.Proto()
		}
		operation.LockGeneration = cond.LockGeneration
	}
	s.handler.OperatorProposeOperation(operation)

	select {
	case result := <-waiter:
		return result, result.err

	case <-time.After(s.operationTimeout):
		s.mtx.Lock()
		delete(s.waiters, operationID)
		s.mtx.Unlock()
		// The apply may have signaled between the timeout and the delete.
		select {
		case result := <-waiter:
			return result, result.err
		default:
		}
		return nil, ErrOperationTimeout
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
		delete(s.locks, key)
	}

	s.tail = &tail
	return nil
}

func (s *Operator) ClearRange() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.tail = nil
}

// SetSplitting fences writes into the range being handed over by a split
// (see writableLocked) and then drains the operations that were accepted
// before the fence: they may still be uncommitted, and the caller (Migrate)
// exports the records right after this returns — an operation applied after
// the export would be dropped by the following PreCommitSplit even though it
// was acknowledged. The drain is bounded by operationTimeout: every accepted
// operation resolves within that bound anyway (apply or timeout), and a
// longer stall means the group cannot commit, in which case the split's own
// proposals fail too.
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
		if pending == 0 {
			return
		}
		if time.Now().After(deadline) {
			// the "==" / "@@" markers are what the simulator's log collection keeps
			fmt.Println(time.Now(), s.head.String(), "@@ split fence: pending operations not drained:", pending)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// SetMergeFence toggles the merge write fence (see writableLocked). Driven by
// the sector's apply handlers: prepare_merge sets it, release_merge clears it,
// and a snapshot restore syncs it to the restored mergeBy.
func (s *Operator) SetMergeFence(fenced bool) {
	s.mtx.Lock()
	s.mergeFenced = fenced
	s.mtx.Unlock()
}

// casConflict checks the operation's CAS condition against the current
// record. Deterministic across replicas: the record (and the revision inside
// its envelope) is replicated state.
func casConflict(current *proto.KvsRecord, operation *proto.Operation) error {
	if operation.CasRevision == 0 && !operation.CasAbsent {
		return nil // unconditional
	}
	var revision uint64 // 0 = absent (revisions are never assigned 0)
	if current != nil {
		revision = current.Revision
	}
	if operation.CasAbsent {
		if revision != 0 {
			return kvsTypes.ErrorCasConflict
		}
		return nil
	}
	if revision != operation.CasRevision {
		return kvsTypes.ErrorCasConflict
	}
	return nil
}

// lockGuard enforces the guarded-write rule of the lease lock
// (spec/kvs/lock.md「guarded write」) for SET/PATCH/DELETE:
//   - unlocked record + no token   → allowed
//   - unlocked record + token      → ErrorCasConflict (the lease was lost —
//     released, revoked, or the record is gone)
//   - locked + no/foreign token    → ErrorLockHeld (wait for the lease)
//   - locked + holder, stale gen   → ErrorCasConflict (fencing: a previous
//     lease's writes must not land)
//   - locked + holder, current gen → allowed
func lockGuard(current *proto.KvsRecord, operation *proto.Operation) error {
	var lock *proto.KvsLock
	if current != nil {
		lock = current.Lock
	}
	hasToken := operation.LockGeneration != 0
	if lock == nil {
		if hasToken {
			return kvsTypes.ErrorCasConflict
		}
		return nil
	}
	if !hasToken || !proto3.Equal(lock.Owner, operation.LockOwner) {
		return kvsTypes.ErrorLockHeld
	}
	if lock.Generation != operation.LockGeneration {
		return kvsTypes.ErrorCasConflict
	}
	return nil
}

// ApplyProposal applies a committed operation to the store and, on the
// proposing replica, resolves the waiter. Runs on the consensus loop
// goroutine of every replica. The range gate re-checks against the CURRENT
// tail — a split/merge committed between propose and apply may have moved the
// key out — and is deterministic across replicas because tail is replicated
// state. A skipped operation fails only the local waiter (the client retries
// against the new owner); it is not an apply divergence, so the consensus
// layer still gets nil. The same holds for CAS conflicts, lock-guard
// rejections and patch failures: the store is left untouched
// deterministically and only the waiter learns the outcome.
func (s *Operator) ApplyProposal(operation *proto.Operation) error {
	keyHash := types.NewHashedNodeID([]byte(operation.Key))

	s.mtx.Lock()

	var waiterErr error // outcome reported to the local waiter
	var storeErr error  // real store failure, reported to the consensus layer
	result := &applyResult{}
	if !s.inRangeLocked(keyHash) {
		waiterErr = kvsTypes.ErrorSectorNotReady
	} else {
		// Load and decode the current record once; every check and command
		// below works on this consistent view. A store failure is a real
		// apply problem; a decode failure is deterministic (identical bytes
		// on every replica) and fails only the waiter.
		var current *proto.KvsRecord
		if _, ok := s.keys[operation.Key]; ok {
			data, err := s.store.Get(&s.sectorKey, operation.Key)
			if err != nil {
				storeErr = err
			} else if current, err = decodeRecord(data); err != nil {
				waiterErr = err
			}
		}

		if waiterErr == nil && storeErr == nil {
			switch operation.Command {
			case proto.Operation_COMMAND_SET,
				proto.Operation_COMMAND_PATCH,
				proto.Operation_COMMAND_DELETE:
				if err := lockGuard(current, operation); err != nil {
					waiterErr = err
				} else if err := casConflict(current, operation); err != nil {
					waiterErr = err
				} else {
					waiterErr, storeErr = s.applyWriteLocked(operation, current, result)
				}

			case proto.Operation_COMMAND_LOCK_ACQUIRE:
				waiterErr, storeErr = s.applyLockAcquireLocked(operation, current, result)

			case proto.Operation_COMMAND_LOCK_RELEASE,
				proto.Operation_COMMAND_LOCK_REVOKE:
				waiterErr, storeErr = s.applyLockClearLocked(operation, current)

			default:
				waiterErr = fmt.Errorf("unsupported operation command: %d", operation.Command)
			}
		}
	}
	if storeErr != nil {
		waiterErr = storeErr
	}
	result.err = waiterErr

	waiter := s.waiters[operation.OperationId]
	delete(s.waiters, operation.OperationId)

	s.mtx.Unlock()

	if waiter != nil {
		waiter <- result
	}

	return storeErr
}

// applyWriteLocked lands SET/PATCH/DELETE after the guard checks passed. An
// existing lease lock rides along unchanged on SET/PATCH; DELETE removes the
// record together with its lease (a holder-guarded delete is the atomic
// release+delete). Call with s.mtx held.
func (s *Operator) applyWriteLocked(operation *proto.Operation, current *proto.KvsRecord, result *applyResult) (error, error) {
	var lock *proto.KvsLock
	if current != nil {
		lock = current.Lock
	}

	switch operation.Command {
	case proto.Operation_COMMAND_SET:
		// The counter advances only when the write lands, so a store failure
		// leaves the replicated state untouched.
		newRevision := s.revisionCounter + 1
		data, err := encodeRecord(operation.Value, newRevision, lock)
		if err != nil {
			return err, nil
		}
		if err := s.store.Set(&s.sectorKey, operation.Key, data); err != nil {
			return nil, err
		}
		s.revisionCounter = newRevision
		s.keys[operation.Key] = struct{}{}
		result.revision = newRevision
		return nil, nil

	case proto.Operation_COMMAND_PATCH:
		// The patcher runs inside the apply: deterministic by the contract of
		// kvsTypes.Patcher, homogeneous by the registration rule.
		if current == nil {
			// partial update of a nonexistent record; creation is Set's job
			return kvsTypes.ErrorStoreKeyNotFound, nil
		}
		patcher, ok := s.patchers[operation.Patcher]
		if !ok {
			return fmt.Errorf("patcher %q is not registered on this node: %w",
				operation.Patcher, kvsTypes.ErrorPatchFailed), nil
		}
		patched, err := patcher.Apply(current.Value, operation.Value)
		if err != nil {
			return fmt.Errorf("%w: %w", kvsTypes.ErrorPatchFailed, err), nil
		}
		newRevision := s.revisionCounter + 1
		encoded, err := encodeRecord(patched, newRevision, lock)
		if err != nil {
			return err, nil
		}
		if err := s.store.Set(&s.sectorKey, operation.Key, encoded); err != nil {
			return nil, err
		}
		s.revisionCounter = newRevision
		result.revision = newRevision
		return nil, nil

	case proto.Operation_COMMAND_DELETE:
		if current == nil {
			// deleting an absent key is a client-level miss, not an apply
			// failure; keys is derived from the same replicated operations on
			// every replica, so the check is deterministic
			return kvsTypes.ErrorStoreKeyNotFound, nil
		}
		if err := s.store.Delete(&s.sectorKey, operation.Key); err != nil {
			return nil, err
		}
		delete(s.keys, operation.Key)
		delete(s.locks, operation.Key)
		return nil, nil
	}
	return fmt.Errorf("unsupported write command: %d", operation.Command), nil
}

// applyLockAcquireLocked grants or renews the record's lease
// (spec/kvs/lock.md). Absent record → create an empty one carrying the lease;
// unlocked → grant with a fresh generation from the counter; same owner →
// renewal keeping the generation (this is what makes acquire retries and the
// client's renewal loop idempotent); other owner → ErrorLockHeld. Expiry is
// NOT evaluated here — no clock reads in the apply; the host proposes
// LOCK_REVOKE instead. Call with s.mtx held.
func (s *Operator) applyLockAcquireLocked(operation *proto.Operation, current *proto.KvsRecord, result *applyResult) (error, error) {
	if operation.LockOwner == nil {
		return fmt.Errorf("lock acquire without an owner"), nil
	}

	grant := func(value []byte, revision uint64, lock *proto.KvsLock, advanceCounter uint64, createKey bool) (error, error) {
		data, err := encodeRecord(value, revision, lock)
		if err != nil {
			return err, nil
		}
		if err := s.store.Set(&s.sectorKey, operation.Key, data); err != nil {
			return nil, err
		}
		if advanceCounter != 0 {
			s.revisionCounter = advanceCounter
		}
		if createKey {
			s.keys[operation.Key] = struct{}{}
		}
		s.locks[operation.Key] = &lockIndexEntry{
			owner:      lock.Owner,
			generation: lock.Generation,
			deadlineMS: lock.DeadlineMs,
		}
		result.lockGeneration = lock.Generation
		result.lockDeadlineMS = lock.DeadlineMs
		return nil, nil
	}

	switch {
	case current == nil:
		// lock on an absent key creates an empty record carrying the lease
		newCounter := s.revisionCounter + 1
		lock := &proto.KvsLock{
			Owner:      operation.LockOwner,
			Generation: newCounter,
			DeadlineMs: operation.LockDeadlineMs,
		}
		waiterErr, storeErr := grant(nil, newCounter, lock, newCounter, true)
		if waiterErr == nil && storeErr == nil {
			result.revision = newCounter
		}
		return waiterErr, storeErr

	case current.Lock == nil:
		newCounter := s.revisionCounter + 1
		lock := &proto.KvsLock{
			Owner:      operation.LockOwner,
			Generation: newCounter,
			DeadlineMs: operation.LockDeadlineMs,
		}
		return grant(current.Value, current.Revision, lock, newCounter, false)

	case proto3.Equal(current.Lock.Owner, operation.LockOwner):
		// renewal: extend the deadline, keep the generation
		lock := &proto.KvsLock{
			Owner:      current.Lock.Owner,
			Generation: current.Lock.Generation,
			DeadlineMs: operation.LockDeadlineMs,
		}
		return grant(current.Value, current.Revision, lock, 0, false)

	default:
		return kvsTypes.ErrorLockHeld, nil
	}
}

// applyLockClearLocked applies LOCK_RELEASE / LOCK_REVOKE: a CAS on
// (owner, generation), so a clear racing a newer acquisition never clears the
// newer lease — the ReleaseMerge pattern. Clearing an unlocked record is a
// successful no-op (retry safety). On a mismatch, RELEASE reports
// ErrorCasConflict (the lease is not the caller's anymore) while REVOKE stays
// silent (the host's stale revocation must simply not fire). Call with s.mtx
// held.
func (s *Operator) applyLockClearLocked(operation *proto.Operation, current *proto.KvsRecord) (error, error) {
	if current == nil || current.Lock == nil {
		delete(s.locks, operation.Key)
		return nil, nil
	}
	if !proto3.Equal(current.Lock.Owner, operation.LockOwner) ||
		current.Lock.Generation != operation.LockGeneration {
		if operation.Command == proto.Operation_COMMAND_LOCK_REVOKE {
			return nil, nil
		}
		return kvsTypes.ErrorCasConflict, nil
	}

	data, err := encodeRecord(current.Value, current.Revision, nil)
	if err != nil {
		return err, nil
	}
	if err := s.store.Set(&s.sectorKey, operation.Key, data); err != nil {
		return nil, err
	}
	delete(s.locks, operation.Key)
	return nil, nil
}

// ExportRecords exports the records in [head, tail) as opaque envelope bytes,
// together with the sector's revision counter taken in the same critical
// section (the importer max-merges it; exporting the records without the
// counter would let the destination assign revisions the records have already
// passed).
func (s *Operator) ExportRecords(head, tail *types.NodeID) (map[string][]byte, uint64, error) {
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
			return nil, 0, err
		}
		records[key] = value
	}

	return records, s.revisionCounter, nil
}

// ImportRecords merges migrated records in and max-merges the source sector's
// revision counter: after the import, every next revision assigned here is
// greater than any revision the imported records carry. Runs on the apply
// path (deterministic: the counter arrives inside the committed proposal).
func (s *Operator) ImportRecords(records map[string][]byte, revisionCounter uint64) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for key, value := range records {
		if err := s.store.Set(&s.sectorKey, key, value); err != nil {
			return err
		}
		s.keys[key] = struct{}{}
		s.indexLockLocked(key, value)
	}
	s.revisionCounter = max(s.revisionCounter, revisionCounter)

	return nil
}

// indexLockLocked syncs the derived lease index for one record from its
// envelope bytes. A record that does not decode (possible only in tests that
// import raw bytes) simply carries no lease. Call with s.mtx held.
func (s *Operator) indexLockLocked(key string, envelope []byte) {
	record, err := decodeRecord(envelope)
	if err != nil || record.Lock == nil {
		delete(s.locks, key)
		return
	}
	s.locks[key] = &lockIndexEntry{
		owner:      record.Lock.Owner,
		generation: record.Lock.Generation,
		deadlineMS: record.Lock.DeadlineMs,
	}
}

// ExportAllRecords dumps every record of the sector without range filtering,
// plus the revision counter for the snapshot. Snapshots must use this instead
// of ExportRecords: a not-yet-activated sector has no tail to filter by but
// may already hold records (split imports into the inactive frontward sector
// before CommitSplit activates it).
func (s *Operator) ExportAllRecords() (map[string][]byte, uint64, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	records := make(map[string][]byte)

	for key := range s.keys {
		value, err := s.store.Get(&s.sectorKey, key)
		if err != nil {
			return nil, 0, err
		}
		records[key] = value
	}

	return records, s.revisionCounter, nil
}

// ReplaceRecords swaps the whole record set for the snapshot's one. Unlike
// ImportRecords (a merge), applying a snapshot must delete local records that
// the snapshot does not contain — a replica that fell behind may hold keys
// the group has since deleted. The revision counter is assigned verbatim for
// the same reason: the snapshot is the full replicated state at its index.
// The caller resets the store sector (ReleaseSector/AllocateSector) before
// calling this.
func (s *Operator) ReplaceRecords(records map[string][]byte, revisionCounter uint64) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.keys = make(map[string]any)
	s.locks = make(map[string]*lockIndexEntry)
	for key, value := range records {
		if err := s.store.Set(&s.sectorKey, key, value); err != nil {
			return err
		}
		s.keys[key] = struct{}{}
		s.indexLockLocked(key, value)
	}
	s.revisionCounter = revisionCounter

	return nil
}

// lockRevokeMargin is how far past a lease deadline the host waits before
// proposing the revocation, absorbing clock skew between successive hosts
// (the deadline was stamped by whichever host handled the acquire). NTP-level
// sync assumed; the margin errs on the safe side — a late revoke only delays
// the next acquirer, an early one merely restores the fencing-guarded
// interleaving.
const lockRevokeMargin = 3 * time.Second

// ProposeExpiredLockRevocations proposes LOCK_REVOKE for every lease whose
// deadline passed more than lockRevokeMargin ago. Driven by the hosting
// sector's tick (only the host proposes — replicas keep the same derived
// index but stay quiet). The clock is read HERE, never in the apply; the
// apply is a CAS on (owner, generation), so a stale revocation racing a
// renewal or a newer lease never clears it. Re-proposing every tick until the
// clear lands is harmless for the same reason.
func (s *Operator) ProposeExpiredLockRevocations() {
	now := time.Now().UnixMilli()

	var revokes []*proto.Operation
	s.mtx.RLock()
	if s.tail != nil { // only an active sector serves (and thus revokes) leases
		for key, entry := range s.locks {
			if now > entry.deadlineMS+lockRevokeMargin.Milliseconds() {
				revokes = append(revokes, &proto.Operation{
					Command:        proto.Operation_COMMAND_LOCK_REVOKE,
					Key:            key,
					LockOwner:      entry.owner,
					LockGeneration: entry.generation,
				})
			}
		}
	}
	s.mtx.RUnlock()

	// propose outside the lock (the Transferer contract)
	for _, operation := range revokes {
		// the "@@" marker is kept by the simulator's log collection
		fmt.Println(time.Now(), s.head.String(), "@@ lock revoke:", operation.Key,
			"generation", operation.LockGeneration)
		s.handler.OperatorProposeOperation(operation)
	}
}
