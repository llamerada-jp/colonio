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
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefaultLockTTL is the default lease duration of Client.Lock. Long enough
// that the renewal loop rides out the churn windows in which a range stays
// PREPARING for ten-plus seconds (spec/kvs/lock.md「TTL の下限」).
const DefaultLockTTL = 30 * time.Second

// minLockTTL rejects leases too short to survive one bad renewal round trip.
const minLockTTL = 10 * time.Second

// lockHeldPollInterval paces Lock() while another owner holds the lease.
// Lease changes are only observable by asking the host again (Watch will
// replace this polling, spec/kvs/api.md Stage E).
const lockHeldPollInterval = time.Second

// LockOption adjusts a Client.Lock call.
type LockOption func(*lockOptions)

type lockOptions struct {
	ttl     time.Duration
	tryOnce bool
}

// WithTTL sets the lease duration (default DefaultLockTTL, minimum 10s; the
// host additionally clamps it server-side). The renewal period is TTL/3, so a
// larger TTL tolerates longer disruptions at the cost of a slower takeover
// after the holder dies.
func WithTTL(ttl time.Duration) LockOption {
	return func(o *lockOptions) { o.ttl = ttl }
}

// WithTryOnce makes Lock return ErrLockHeld immediately instead of waiting
// for the current lease to end.
func WithTryOnce() LockOption {
	return func(o *lockOptions) { o.tryOnce = true }
}

// Lock is a held lease on one record, managed by the client: it renews itself
// every TTL/3 and self-fences — Done() is closed — when it can no longer
// prove the lease within the TTL. Obtain it from Client.Lock, stop it with
// Release.
//
// Done() is advisory: the process holding the lock should stop its guarded
// activity when it fires, but the actual protection is the fencing token —
// writes with WithLockToken(Token()) fail with ErrConflict/ErrLockHeld once
// the lease is lost, no matter how late the holder notices
// (spec/kvs/lock.md).
//
// The lock protects exactly its own key. Keys hash to different sectors, so
// one lease cannot guard other records; keep data guarded by one lock inside
// one record (spec/kvs/api.md).
type Lock struct {
	client     *Client
	key        string
	generation uint64
	ttl        time.Duration

	stopRenewal context.CancelFunc

	mtx      sync.Mutex
	done     chan struct{}
	closed   bool
	lossErr  error
	released bool
}

// Lock acquires the lease of the key, waiting for a current holder unless
// WithTryOnce is given, and returns a managed Lock that keeps renewing it.
// Acquiring a lease on an absent key creates an empty record carrying it.
// The context bounds the acquisition only; the returned Lock lives until
// Release or loss.
func (c *Client) Lock(ctx context.Context, key string, opts ...LockOption) (*Lock, error) {
	options := &lockOptions{ttl: DefaultLockTTL}
	for _, opt := range opts {
		opt(options)
	}
	if options.ttl < minLockTTL {
		return nil, fmt.Errorf("kvs lock %q: TTL %v is below the minimum %v (it could expire inside an ordinary churn window)",
			key, options.ttl, minLockTTL)
	}

	backoff := retryInitialBackoff
	sawHeld := false
	for {
		result, err := c.lockAcquireOnce(ctx, key, options.ttl)
		switch {
		case err == nil:
			lock := &Lock{
				client:     c,
				key:        key,
				generation: result.generation,
				ttl:        options.ttl,
				done:       make(chan struct{}),
			}
			renewalCtx, cancel := context.WithCancel(context.Background())
			lock.stopRenewal = cancel
			go lock.renewalLoop(renewalCtx, time.Now())
			return lock, nil

		case errors.Is(err, ErrLockHeld):
			if options.tryOnce {
				return nil, fmt.Errorf("kvs lock %q: %w", key, err)
			}
			sawHeld = true
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("kvs lock %q: deadline expired while waiting for the holder: %w: %w",
					key, ErrLockHeld, ctx.Err())
			case <-time.After(lockHeldPollInterval):
			}
			backoff = retryInitialBackoff

		case errors.Is(err, ErrPreparing), errors.Is(err, ErrResultUnknown):
			// PREPARING is a pre-acceptance rejection; an unknown-outcome
			// acquire is safe to re-send because acquiring is owner-idempotent
			// (an applied one turns the retry into a renewal with the same
			// generation).
			select {
			case <-ctx.Done():
				if sawHeld && errors.Is(err, ErrResultUnknown) {
					// The deadline cut a poll mid-flight, but the last definite
					// answer was "held": report contention, not an unknown
					// outcome (the deadline lands in flight for a fair share of
					// contended waits — run 2026-07-16: unk/(held+unk) ≈ 19%
					// was mostly this).
					return nil, fmt.Errorf("kvs lock %q: deadline expired while waiting for the holder: %w: %w",
						key, ErrLockHeld, ctx.Err())
				}
				return nil, fmt.Errorf("kvs lock %q: deadline expired while retrying: %w: %w",
					key, err, ctx.Err())
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, retryMaxBackoff)

		default:
			return nil, fmt.Errorf("kvs lock %q: %w", key, err)
		}
	}
}

type lockGrant struct {
	generation uint64
	deadlineMS int64
}

func (c *Client) lockAcquireOnce(ctx context.Context, key string, ttl time.Duration) (*lockGrant, error) {
	select {
	case result := <-c.backend.LockAcquire(key, uint64(ttl.Milliseconds())):
		if result.Err != nil {
			return nil, result.Err
		}
		return &lockGrant{generation: result.Generation, deadlineMS: result.DeadlineMS}, nil
	case <-ctx.Done():
		// the acquire may still land; the caller treats this as retryable
		return nil, fmt.Errorf("%w: %w", ErrResultUnknown, ctx.Err())
	}
}

// Token returns the fencing token of the lease: pass it to WithLockToken on
// every write the lock guards.
func (l *Lock) Token() uint64 { return l.generation }

// Key returns the locked key.
func (l *Lock) Key() string { return l.key }

// Done is closed when the client considers the lease lost: renewals kept
// failing for a full TTL (measured on the local monotonic clock —
// conservative: the lease may in fact still be alive), the lease turned out
// to be held by someone else, or Release was called. Guarded activity should
// stop when it fires; Err() tells why.
func (l *Lock) Done() <-chan struct{} { return l.done }

// Err returns why the lease ended: nil after a voluntary Release, otherwise
// the loss reason. Meaningful once Done() is closed.
func (l *Lock) Err() error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.lossErr
}

// Release stops the renewal and clears the lease. Safe to call on a lost
// lock; a lease that meanwhile belongs to someone else is left untouched
// (the clear is a CAS) and Release reports success — either way the caller
// no longer holds it.
func (l *Lock) Release(ctx context.Context) error {
	l.stopRenewal()
	l.close(nil, true)

	backoff := retryInitialBackoff
	for {
		var err error
		select {
		case err = <-l.client.backend.LockRelease(l.key, l.generation):
		case <-ctx.Done():
			return fmt.Errorf("kvs lock %q: release result unknown: %w", l.key, ctx.Err())
		}
		switch {
		case err == nil:
			return nil
		case errors.Is(err, ErrConflict):
			// the lease is not ours anymore (revoked / re-granted): released
			// as far as this holder is concerned
			return nil
		case errors.Is(err, ErrPreparing), errors.Is(err, ErrResultUnknown):
			// releasing is idempotent, keep trying within the context
			select {
			case <-ctx.Done():
				return fmt.Errorf("kvs lock %q: release result unknown: %w", l.key, ctx.Err())
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, retryMaxBackoff)
		default:
			return fmt.Errorf("kvs lock %q: release: %w", l.key, err)
		}
	}
}

// Set writes the locked record itself, guarded by this lease's token.
func (l *Lock) Set(ctx context.Context, value []byte, opts ...WriteOption) (*SetResponse, error) {
	return l.client.Set(ctx, l.key, value, append(opts, WithLockToken(l.generation))...)
}

// Patch patches the locked record itself, guarded by this lease's token.
func (l *Lock) Patch(ctx context.Context, patcher string, patch []byte, opts ...WriteOption) (*SetResponse, error) {
	return l.client.Patch(ctx, l.key, patcher, patch, append(opts, WithLockToken(l.generation))...)
}

// Delete removes the locked record — the atomic release-and-delete: the lease
// disappears with the record. The renewal stops; Done() fires with a nil Err.
func (l *Lock) Delete(ctx context.Context, opts ...WriteOption) error {
	err := l.client.Delete(ctx, l.key, append(opts, WithLockToken(l.generation))...)
	if err == nil {
		l.stopRenewal()
		l.close(nil, true)
	}
	return err
}

func (l *Lock) close(reason error, voluntary bool) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	l.released = voluntary
	l.lossErr = reason
	close(l.done)
}

// renewalLoop re-acquires the lease every TTL/3 and self-fences when it
// cannot: after a full TTL without a proven renewal (local monotonic clock,
// counted from when the successful request was SENT — the host granted the
// lease no earlier than that), the loop must assume the host has revoked the
// lease and someone else may hold it, even if it is in fact still alive.
func (l *Lock) renewalLoop(ctx context.Context, acquiredAt time.Time) {
	interval := l.ttl / 3
	lastProven := acquiredAt

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		initiated := time.Now()
		opCtx, cancel := context.WithTimeout(ctx, interval)
		grant, err := l.client.lockAcquireOnce(opCtx, l.key, l.ttl)
		cancel()

		switch {
		case err == nil && grant.generation == l.generation:
			lastProven = initiated

		case err == nil:
			// The lease lapsed and this renewal re-acquired it as a NEW lease.
			// The old fencing token is dead, and guarded state may have been
			// touched in between — surface the loss instead of silently
			// continuing, and give the accidental fresh lease back.
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), interval)
			select {
			case <-l.client.backend.LockRelease(l.key, grant.generation):
			case <-releaseCtx.Done():
			}
			releaseCancel()
			l.close(fmt.Errorf("kvs lock %q: lease lapsed and was re-granted: %w", l.key, ErrConflict), false)
			return

		case errors.Is(err, ErrLockHeld):
			l.close(fmt.Errorf("kvs lock %q: lease taken over: %w", l.key, err), false)
			return

		default:
			// preparing / unknown / transport trouble: keep trying below,
			// bounded by the self-fencing deadline
		}

		if time.Since(lastProven) > l.ttl-interval {
			// One renewal period before the lease can actually expire, stop
			// trusting it: the next ticks could not save it anyway.
			l.close(fmt.Errorf("kvs lock %q: lease not renewed within TTL (last proven %v ago): %w",
				l.key, time.Since(lastProven).Round(time.Millisecond), ErrResultUnknown), false)
			return
		}
	}
}
