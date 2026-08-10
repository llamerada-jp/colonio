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
	"sync/atomic"
	"time"

	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

const (
	// watchKeepaliveInterval paces the subscription renewal. Every renewal is
	// also a state resync, so this bounds how long a lost push event (or a
	// silent host change) can keep the watcher stale. The host drops
	// subscriptions after three missed renewals (operator watchLeaseTTL).
	watchKeepaliveInterval = 5 * time.Second

	// watchEventBuffer is the capacity of the Events() channel. A slow
	// consumer does not stall the watcher: older undelivered events are
	// coalesced away (dropped) in favor of newer ones, which is exactly the
	// documented delivery semantic.
	watchEventBuffer = 4

	// watchPushBuffer absorbs bursts of pushed events between loop
	// iterations; overflow drops the oldest (recovered by the next resync).
	watchPushBuffer = 16
)

// nextWatchID hands out process-unique watch ids: the id dispatches pushed
// events to Watcher instances of this node, and stale host-side subscriptions
// from earlier instances must not alias a live one.
var nextWatchID atomic.Uint64

// WatchEvent is one observed state of the watched record.
type WatchEvent struct {
	Key   string
	Value []byte // nil when Deleted
	// Revision identifies the state for idempotent consumption: for a value
	// event the record's revision, for a Deleted event the deleted record's
	// last revision when known (0 otherwise).
	Revision uint64
	Deleted  bool
}

// WatchOption adjusts a single Watch call.
type WatchOption func(*watchOptions)

type watchOptions struct {
	sinceRevision uint64
}

// WithSinceRevision suppresses the initial state event when the record still
// has exactly this revision (a value previously returned by Get/Set or a
// prior WatchEvent): the watcher then reports changes only. Without it the
// current state — including "absent", delivered as a Deleted event — is
// always the first event.
func WithSinceRevision(revision uint64) WatchOption {
	return func(o *watchOptions) { o.sinceRevision = revision }
}

// Watcher observes one key. Consume state changes from Events(); the channel
// closes when the context given to Watch ends, and Err() tells why.
//
// Delivery is coalesced and at-least-once (spec/kvs/api.md「Watch」): every
// change of the record is pushed by its host, but a watcher that was cut off
// (host change, packet loss, slow consumer) converges to the LATEST state
// instead of replaying the missed history. Duplicate states are suppressed
// per (Revision, Deleted), and events never go backwards except after a
// sector data loss (a documented rare case), where the periodic resync
// re-delivers the then-current state.
type Watcher struct {
	client  *Client
	key     string
	watchID uint64

	events chan WatchEvent
	pushCh chan *kvsTypes.WatchPush

	// lockFree wakes internal lock waiters whenever an observed state has no
	// held lease (capacity 1, collapsing signal); see lock.go.
	lockFree chan struct{}

	// last delivered state; owned by the run goroutine
	lastKnown   bool
	lastRev     uint64
	lastDeleted bool

	mtx sync.Mutex
	err error
}

// Watch starts observing the key and returns immediately; the subscription is
// established (and re-established around node churn) in the background. The
// first event delivers the record's current state — a Deleted event when the
// record does not exist — unless WithSinceRevision says the caller already
// has it. ctx bounds the watcher's lifetime: when it ends, Events() closes.
//
// While the key's range is unreachable (node churn, PREPARING) the watcher
// silently keeps retrying; there is no error reporting short of ctx ending,
// events just resume — with a state resync — once the host answers again.
func (c *Client) Watch(ctx context.Context, key string, opts ...WatchOption) (*Watcher, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	options := &watchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	w := &Watcher{
		client:   c,
		key:      key,
		watchID:  nextWatchID.Add(1),
		events:   make(chan WatchEvent, watchEventBuffer),
		pushCh:   make(chan *kvsTypes.WatchPush, watchPushBuffer),
		lockFree: make(chan struct{}, 1),
	}
	if options.sinceRevision != 0 {
		// behave as if the state (sinceRevision, exists) had been delivered
		w.lastKnown = true
		w.lastRev = options.sinceRevision
	}

	c.backend.WatchRegisterSink(w.watchID, w.sink)
	go w.run(ctx)
	return w, nil
}

// Events returns the state-change stream. Closed when the Watch context ends.
func (w *Watcher) Events() <-chan WatchEvent { return w.events }

// Err returns why the event stream ended; meaningful after Events() closed.
func (w *Watcher) Err() error {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.err
}

// sink receives pushed events on the network goroutine: forward without
// blocking, dropping the oldest pending push on overflow (the state converges
// again via the newer pushes or the next keepalive resync).
func (w *Watcher) sink(push *kvsTypes.WatchPush) {
	select {
	case w.pushCh <- push:
	default:
		select {
		case <-w.pushCh:
		default:
		}
		select {
		case w.pushCh <- push:
		default:
		}
	}
}

// run drives the subscription: an immediate first subscribe (with backoff
// while the range is preparing), then keepalive renewals that double as state
// resyncs, with pushed events processed in between.
func (w *Watcher) run(ctx context.Context) {
	defer func() {
		w.client.backend.WatchUnregisterSink(w.watchID)
		w.client.backend.WatchCancel(w.key, w.watchID)
		w.mtx.Lock()
		w.err = ctx.Err()
		w.mtx.Unlock()
		close(w.events)
	}()

	backoff := retryInitialBackoff
	timer := time.NewTimer(0) // subscribe immediately
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case push := <-w.pushCh:
			w.processPush(push)

		case <-timer.C:
			if w.subscribeOnce(ctx) {
				backoff = retryInitialBackoff
				timer.Reset(watchKeepaliveInterval)
			} else {
				timer.Reset(backoff)
				backoff = min(backoff*2, retryMaxBackoff)
			}
		}
	}
}

// subscribeOnce sends one subscription/keepalive round trip and applies the
// returned state. It reports whether the host answered (any definite answer
// resets the retry backoff; PREPARING and transport errors do not).
func (w *Watcher) subscribeOnce(ctx context.Context) bool {
	// Send since_revision only when the last delivered state is a live
	// record: the host then omits the value if nothing changed. After a
	// Deleted state (or before any) the full value is always wanted.
	var since uint64
	if w.lastKnown && !w.lastDeleted {
		since = w.lastRev
	}

	select {
	case result := <-w.client.backend.WatchSubscribe(w.key, w.watchID, since):
		if result.Err != nil {
			return false
		}
		w.processState(&result.State)
		return true
	case <-ctx.Done():
		return false
	case <-time.After(watchKeepaliveInterval):
		// request lost in flight; retry
		return false
	}
}

// processState applies the authoritative state of a subscribe response. It
// delivers on any difference from the last delivered state — including a
// revision that went BACKWARDS (sector data loss resets the counter; the
// resync is the recovery path for watchers stuck on a stale high revision).
func (w *Watcher) processState(state *kvsTypes.WatchState) {
	w.signalLockState(state.Locked && state.Exists)

	if !state.Exists {
		// Absent covers both "deleted while we were cut off" and "never
		// existed" (no tombstones — the ambiguity is resolved by
		// over-notifying, which the dedup below keeps to one event).
		if !w.lastKnown || !w.lastDeleted {
			w.deliver(WatchEvent{Key: w.key, Deleted: true, Revision: w.lastRev})
			w.lastKnown = true
			w.lastDeleted = true
		}
		return
	}

	if w.lastKnown && !w.lastDeleted && state.Revision == w.lastRev {
		return // unchanged (this is also the only case the value is omitted)
	}
	if state.ValueOmitted {
		// Defensive: an omitted value we cannot deliver. Treat as unchanged;
		// the next renewal (sent without since, as lastKnown stays put)
		// fetches the full value.
		return
	}
	w.deliver(WatchEvent{Key: w.key, Value: state.Value, Revision: state.Revision})
	w.lastKnown = true
	w.lastRev = state.Revision
	w.lastDeleted = false
}

// processPush applies one pushed event. Pushes are trusted only forwards:
// (revision, deleted) must be newer than the last delivered state — an older
// or duplicate push (at-least-once delivery, reordering) is dropped. A pure
// lock transition arrives with the unchanged revision and only feeds the
// lock-waiter signal.
func (w *Watcher) processPush(push *kvsTypes.WatchPush) {
	w.signalLockState(push.Locked && !push.Deleted)

	newer := !w.lastKnown ||
		push.Revision > w.lastRev ||
		(push.Revision == w.lastRev && push.Deleted && !w.lastDeleted)
	if !newer {
		return
	}
	value := push.Value
	if push.Deleted {
		value = nil
	}
	w.deliver(WatchEvent{Key: w.key, Value: value, Revision: push.Revision, Deleted: push.Deleted})
	w.lastKnown = true
	w.lastRev = push.Revision
	w.lastDeleted = push.Deleted
}

// deliver hands one event to the consumer, coalescing when it is slow: on a
// full channel the oldest undelivered event is dropped for the newer state.
// Single writer (the run goroutine), so the drain-then-send cannot race
// another sender.
func (w *Watcher) deliver(event WatchEvent) {
	select {
	case w.events <- event:
		return
	default:
	}
	select {
	case <-w.events:
	default:
	}
	w.events <- event
}

// signalLockState feeds the lock waiters: any observed state whose lease is
// not held means an acquire attempt could succeed now. Collapsing channel —
// waiters that miss a signal are also paced by their fallback interval.
func (w *Watcher) signalLockState(locked bool) {
	if locked {
		return
	}
	select {
	case w.lockFree <- struct{}{}:
	default:
	}
}
