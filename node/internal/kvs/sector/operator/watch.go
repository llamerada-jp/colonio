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
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

// Watch subscriptions are host-local DERIVED state (spec/kvs/api.md「Watch」):
// they are never replicated, migrated or snapshotted. Only the hosting
// operator ever holds entries — subscriptions are registered through
// kvsOperate-style requests that route to the key's host — so the apply-path
// emission below is naturally silent on replicas. A host change simply drops
// the subscriptions with the replica; the client re-subscribes through its
// keepalive and recovers the missed changes from the state resync.

// watchLeaseTTL is how long a subscription survives without a keepalive.
// Three client keepalive periods (node/kvs watchKeepaliveInterval = 5s), the
// same missed-round tolerance as the link keepalive.
const watchLeaseTTL = 15 * time.Second

// watcherKey identifies one Watcher instance: the watching node plus its
// client-generated id (echoed on pushed events for dispatch).
type watcherKey struct {
	node    types.NodeID
	watchID uint64
}

// watchPush is one pending event delivery, collected under s.mtx by the apply
// path and sent after the lock is released (sending under a held mutex is the
// Transferer self-deadlock pattern, 2026-07-11).
type watchPush struct {
	dst   types.NodeID
	event *proto.KvsWatchEvent
}

// WatchSubscribe registers (or renews — registration doubles as the
// keepalive) a subscription and returns the record's current state so the
// client can resync after missed events or a host change. The value is
// omitted when the record still has exactly sinceRevision, so keepalives stay
// cheap for large values. Out-of-range keys are rejected with the retryable
// ErrorSectorNotReady, exactly like reads: the client re-subscribes and lands
// on the current host.
func (s *Operator) WatchSubscribe(key string, watcher *types.NodeID, watchID uint64, sinceRevision uint64) (*kvsTypes.WatchState, error) {
	keyHash := types.NewHashedNodeID([]byte(key))

	s.mtx.Lock()
	defer s.mtx.Unlock()

	if !s.inRangeLocked(keyHash) {
		return nil, kvsTypes.ErrorSectorNotReady
	}

	subscribers := s.watches[key]
	if subscribers == nil {
		subscribers = make(map[watcherKey]time.Time)
		s.watches[key] = subscribers
	}
	subscribers[watcherKey{node: *watcher, watchID: watchID}] = time.Now().Add(watchLeaseTTL)

	state := &kvsTypes.WatchState{}
	if _, ok := s.keys[key]; !ok {
		return state, nil
	}
	data, err := s.store.Get(&s.sectorKey, key)
	if err != nil {
		return nil, err
	}
	record, err := decodeRecord(data)
	if err != nil {
		return nil, err
	}
	state.Exists = true
	state.Revision = record.Revision
	state.Locked = record.Lock != nil
	if record.Revision == sinceRevision {
		state.ValueOmitted = true
	} else {
		state.Value = record.Value
	}
	return state, nil
}

// WatchCancel drops one subscription. Best-effort: a missing entry (already
// expired, or the range moved) is a successful no-op.
func (s *Operator) WatchCancel(key string, watcher *types.NodeID, watchID uint64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	subscribers := s.watches[key]
	if subscribers == nil {
		return
	}
	delete(subscribers, watcherKey{node: *watcher, watchID: watchID})
	if len(subscribers) == 0 {
		delete(s.watches, key)
	}
}

// PurgeExpiredWatches drops subscriptions whose lease ran out (the watcher
// died or lost connectivity — its keepalives stopped). Driven by the hosting
// sector's tick; reads the clock, which is fine outside the apply path.
func (s *Operator) PurgeExpiredWatches() {
	now := time.Now()

	s.mtx.Lock()
	defer s.mtx.Unlock()

	for key, subscribers := range s.watches {
		for wk, deadline := range subscribers {
			if now.After(deadline) {
				delete(subscribers, wk)
			}
		}
		if len(subscribers) == 0 {
			delete(s.watches, key)
		}
	}
}

// queueWatchEventLocked collects one change notification for every subscriber
// of the key. Called by the apply helpers with s.mtx held; ApplyProposal
// flushes the queue after releasing the lock. Emitted only for observable
// state transitions: a changed (revision, exists) — SET/PATCH/DELETE and the
// record created by a lock on an absent key — and a cleared lease
// (release/revoke), which keeps the revision but wakes lock waiters. Lease
// grants and renewals are NOT emitted: nobody consumes them and renewals
// would spam every watcher of a locked key.
func (s *Operator) queueWatchEventLocked(key string, value []byte, revision uint64, deleted, locked bool) {
	subscribers := s.watches[key]
	if len(subscribers) == 0 {
		return
	}
	for wk := range subscribers {
		s.pendingWatchPushes = append(s.pendingWatchPushes, watchPush{
			dst: wk.node,
			event: &proto.KvsWatchEvent{
				WatchId:  wk.watchID,
				Key:      key,
				Value:    value,
				Revision: revision,
				Deleted:  deleted,
				Locked:   locked,
			},
		})
	}
}

// takeWatchPushesLocked hands the collected pushes to the caller for sending
// outside the lock. Call with s.mtx held.
func (s *Operator) takeWatchPushesLocked() []watchPush {
	pushes := s.pendingWatchPushes
	s.pendingWatchPushes = nil
	return pushes
}

// purgeWatchesOutOfRangeLocked drops subscriptions of keys outside [head,
// tail) after a range shrink. The keys were migrated, not deleted, so no
// Deleted event is emitted; the clients' keepalives re-subscribe with the new
// host. Call with s.mtx held.
func (s *Operator) purgeWatchesOutOfRangeLocked() {
	for key := range s.watches {
		keyHash := types.NewHashedNodeID([]byte(key))
		if !s.inRangeLocked(keyHash) {
			delete(s.watches, key)
		}
	}
}
