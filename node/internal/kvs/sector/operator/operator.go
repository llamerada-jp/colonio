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

// CasCondition guards a write; nil means unconditional. The check runs at
// apply time against the record's replicated revision, so it is deterministic
// across replicas (spec/kvs/lock.md「CAS 操作」).
type CasCondition struct {
	// Revision, when non-zero, requires the record to exist with exactly this
	// revision. Revisions are assigned from the sector counter and are never 0.
	Revision uint64
	// Absent requires the record to not exist.
	Absent bool
}

type Operations interface {
	Get(key string) ([]byte, uint64, error)
	Set(key string, value []byte, cas *CasCondition) (uint64, error)
	Patch(key string, patcherName string, patch []byte, cas *CasCondition) (uint64, error)
	Delete(key string, cas *CasCondition) error
}

// recordMarshal serializes the KvsRecord envelope. Deterministic marshaling
// is required: the encoded bytes are replicated state (stored on every
// replica and carried by snapshots), so replicas must produce identical
// bytes for identical logical records.
var recordMarshal = proto3.MarshalOptions{Deterministic: true}

func encodeRecord(value []byte, revision uint64) ([]byte, error) {
	return recordMarshal.Marshal(&proto.KvsRecord{
		Value:    value,
		Revision: revision,
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
	// revision is the newly assigned revision for an applied SET, 0 otherwise.
	revision uint64
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
func (s *Operator) Set(key string, value []byte, cas *CasCondition) (uint64, error) {
	return s.proposeOperation(proto.Operation_COMMAND_SET, key, value, cas)
}

// Patch applies the named node-registered Patcher to the record inside the
// raft apply (the patch document is what travels, not the value). The
// registry gate here rejects an unregistered name before proposing — a
// definite misconfiguration answer; the apply re-checks it deterministically.
func (s *Operator) Patch(key string, patcherName string, patch []byte, cas *CasCondition) (uint64, error) {
	if _, ok := s.patchers[patcherName]; !ok {
		return 0, fmt.Errorf("patcher %q is not registered on this node: %w",
			patcherName, kvsTypes.ErrorPatchFailed)
	}
	return s.proposeOperationFull(&proto.Operation{
		Command: proto.Operation_COMMAND_PATCH,
		Key:     key,
		Value:   patch,
		Patcher: patcherName,
	}, cas)
}

func (s *Operator) Delete(key string, cas *CasCondition) error {
	_, err := s.proposeOperation(proto.Operation_COMMAND_DELETE, key, nil, cas)
	return err
}

// proposeOperation runs a write through the raft group and blocks until the
// committed operation is applied on this replica (or times out). The gate
// check and the waiter registration share one critical section, so a write
// accepted before a fence (SetSplitting) is always visible to the fence's
// drain loop. The propose itself runs outside the lock — sending under a held
// mutex is the Transferer self-deadlock pattern (2026-07-11).
// It returns the newly assigned revision for an applied SET/PATCH (0 otherwise).
func (s *Operator) proposeOperation(command proto.Operation_Command, key string, value []byte, cas *CasCondition) (uint64, error) {
	return s.proposeOperationFull(&proto.Operation{
		Command: command,
		Key:     key,
		Value:   value,
	}, cas)
}

// proposeOperationFull is proposeOperation for a caller-built Operation
// (operation_id and the CAS fields are filled in here).
func (s *Operator) proposeOperationFull(operation *proto.Operation, cas *CasCondition) (uint64, error) {
	keyHash := types.NewHashedNodeID([]byte(operation.Key))

	s.mtx.Lock()
	if err := s.writableLocked(keyHash); err != nil {
		s.mtx.Unlock()
		return 0, err
	}
	s.nextOperationID++
	operationID := s.nextOperationID
	waiter := make(chan *applyResult, 1)
	s.waiters[operationID] = waiter
	s.mtx.Unlock()

	operation.OperationId = operationID
	if cas != nil {
		operation.CasRevision = cas.Revision
		operation.CasAbsent = cas.Absent
	}
	s.handler.OperatorProposeOperation(operation)

	select {
	case result := <-waiter:
		return result.revision, result.err

	case <-time.After(s.operationTimeout):
		s.mtx.Lock()
		delete(s.waiters, operationID)
		s.mtx.Unlock()
		// The apply may have signaled between the timeout and the delete.
		select {
		case result := <-waiter:
			return result.revision, result.err
		default:
		}
		return 0, ErrOperationTimeout
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

// casConflictLocked checks the operation's CAS condition against the record's
// current revision. Deterministic across replicas: keys, the stored envelopes
// and the revision inside them are all replicated state. A decode failure is
// also deterministic (the stored bytes are identical on every replica).
// Call with s.mtx held.
func (s *Operator) casConflictLocked(operation *proto.Operation) error {
	if operation.CasRevision == 0 && !operation.CasAbsent {
		return nil // unconditional
	}

	var current uint64 // 0 = absent (revisions are never assigned 0)
	if _, ok := s.keys[operation.Key]; ok {
		data, err := s.store.Get(&s.sectorKey, operation.Key)
		if err != nil {
			return err
		}
		record, err := decodeRecord(data)
		if err != nil {
			return err
		}
		current = record.Revision
	}

	if operation.CasAbsent {
		if current != 0 {
			return kvsTypes.ErrorCasConflict
		}
		return nil
	}
	if current != operation.CasRevision {
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
// layer still gets nil. The same holds for a CAS conflict: the store is left
// untouched deterministically and only the waiter learns the conflict.
func (s *Operator) ApplyProposal(operation *proto.Operation) error {
	keyHash := types.NewHashedNodeID([]byte(operation.Key))

	s.mtx.Lock()

	var waiterErr error // outcome reported to the local waiter
	var storeErr error  // real store failure, reported to the consensus layer
	var appliedRevision uint64
	if !s.inRangeLocked(keyHash) {
		waiterErr = kvsTypes.ErrorSectorNotReady
	} else if casErr := s.casConflictLocked(operation); casErr != nil {
		waiterErr = casErr
	} else {
		switch operation.Command {
		case proto.Operation_COMMAND_SET:
			// The counter advances only when the write lands, so a store
			// failure leaves the replicated state untouched.
			newRevision := s.revisionCounter + 1
			data, err := encodeRecord(operation.Value, newRevision)
			if err != nil {
				waiterErr = err
			} else {
				storeErr = s.store.Set(&s.sectorKey, operation.Key, data)
				if storeErr == nil {
					s.revisionCounter = newRevision
					s.keys[operation.Key] = struct{}{}
					appliedRevision = newRevision
				}
			}

		case proto.Operation_COMMAND_PATCH:
			// store.Get → Patcher.Apply → store.Set, all inside the apply.
			// Every failure below is a waiter-level rejection that leaves the
			// store untouched, and each is deterministic across replicas: the
			// record bytes and the operation are replicated state, the
			// registry is required to be cluster-homogeneous, and the Patcher
			// is required to be a deterministic pure function (the contract
			// of kvsTypes.Patcher).
			if _, ok := s.keys[operation.Key]; !ok {
				// partial update of a nonexistent record; creation is Set's job
				waiterErr = kvsTypes.ErrorStoreKeyNotFound
				break
			}
			patcher, ok := s.patchers[operation.Patcher]
			if !ok {
				waiterErr = fmt.Errorf("patcher %q is not registered on this node: %w",
					operation.Patcher, kvsTypes.ErrorPatchFailed)
				break
			}
			data, err := s.store.Get(&s.sectorKey, operation.Key)
			if err != nil {
				storeErr = err
				break
			}
			record, err := decodeRecord(data)
			if err != nil {
				waiterErr = err
				break
			}
			patched, err := patcher.Apply(record.Value, operation.Value)
			if err != nil {
				waiterErr = fmt.Errorf("%w: %w", kvsTypes.ErrorPatchFailed, err)
				break
			}
			newRevision := s.revisionCounter + 1
			encoded, err := encodeRecord(patched, newRevision)
			if err != nil {
				waiterErr = err
				break
			}
			storeErr = s.store.Set(&s.sectorKey, operation.Key, encoded)
			if storeErr == nil {
				s.revisionCounter = newRevision
				appliedRevision = newRevision
			}

		case proto.Operation_COMMAND_DELETE:
			if _, ok := s.keys[operation.Key]; !ok {
				// deleting an absent key is a client-level miss, not an apply
				// failure; keys is derived from the same replicated operations
				// on every replica, so the check is deterministic
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
		waiter <- &applyResult{err: waiterErr, revision: appliedRevision}
	}

	return storeErr
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
	}
	s.revisionCounter = max(s.revisionCounter, revisionCounter)

	return nil
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
	for key, value := range records {
		if err := s.store.Set(&s.sectorKey, key, value); err != nil {
			return err
		}
		s.keys[key] = struct{}{}
	}
	s.revisionCounter = revisionCounter

	return nil
}
