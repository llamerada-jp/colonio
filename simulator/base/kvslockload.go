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
package base

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	colonioNode "github.com/llamerada-jp/colonio/node"
	nodeKvs "github.com/llamerada-jp/colonio/node/kvs"
)

// Lease-lock load for the Stage D verification (spec/kvs/lock.md,
// spec/kvs/api.md): every node runs the double-start-prevention scenario —
// compete for a shared lock key, and only while holding the lease write the
// guarded record. Enabled together with the KVS load (COLONIO_SIM_KVS_INTERVAL_MS).
//
// Mutual-exclusion audit (offline, over the collected cluster logs):
//   - `@@ kvs lock ok: <key> <generation>` at acquisition and
//     `@@ kvs lock end: <key> <generation> <reason>` at the end of the hold.
//     Two overlapping [ok, end] intervals of the same key = a double grant.
//     (key, generation) must also be unique cluster-wide — same caveat as the
//     CAS audit: a sector counter reset after data loss is a rare false
//     positive source.
//   - `guard conf/locked` counters: a guarded write failing while WE believe
//     we hold the lease means the lease moved under us — expected only around
//     churn-driven lease loss, paired with a `lost` count (Done fired).
var kvsLockKeys = envInt("COLONIO_SIM_KVS_LOCK_KEYS", 32)

const (
	kvsLockAcquireTimeout = 45 * time.Second
	kvsLockHoldMin        = 5 * time.Second
	kvsLockHoldMax        = 15 * time.Second
	kvsLockWriteInterval  = 2 * time.Second
)

type kvsLockStats struct {
	acquired, acquireHeld, acquirePrep, acquireUnknown, acquireErr int
	guardOk, guardConflict, guardLocked, guardErr                  int
	lost, released, releaseErr                                     int
}

// startKvsLockLoad launches the lock workload for one node run, alongside
// startKvsLoad (same enable gate, same lifecycle rules — see the col capture
// note there).
func (n *Node) startKvsLockLoad(ctx context.Context) {
	if kvsLoadInterval <= 0 {
		return
	}
	col := n.Col

	go func() {
		localNodeID := col.GetLocalNodeID()
		stats := &kvsLockStats{}
		lastDump := time.Now()

		for {
			// pause between cycles so ~kvsLockKeys holders rotate through
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(1+rand.Intn(4)) * time.Second):
			}

			kvsLockCycle(ctx, col, stats)

			if time.Since(lastDump) >= time.Minute {
				fmt.Println(time.Now(), localNodeID, "@@ kvs lock:",
					"acq", stats.acquired, "/held", stats.acquireHeld, "/prep", stats.acquirePrep,
					"/unk", stats.acquireUnknown, "/err", stats.acquireErr, ",",
					"guard", stats.guardOk, "/conf", stats.guardConflict, "/locked", stats.guardLocked,
					"/err", stats.guardErr, ",",
					"lost", stats.lost, "rel", stats.released, "/err", stats.releaseErr)
				*stats = kvsLockStats{}
				lastDump = time.Now()
			}
		}
	}()
}

// kvsLockCycle runs one double-start-prevention round: acquire the lease of a
// shared key, write the guarded record while holding it, release.
func kvsLockCycle(ctx context.Context, col colonioNode.Node, stats *kvsLockStats) {
	key := fmt.Sprintf("kvs-lock-%d", rand.Intn(kvsLockKeys))
	kv := col.KVS()

	acquireCtx, cancel := context.WithTimeout(ctx, kvsLockAcquireTimeout)
	lock, err := kv.Lock(acquireCtx, key)
	cancel()
	switch {
	case err == nil:
		stats.acquired++
	case errors.Is(err, nodeKvs.ErrLockHeld):
		stats.acquireHeld++ // deadline hit while another owner held it: contention, not a bug
		return
	case errors.Is(err, nodeKvs.ErrPreparing):
		stats.acquirePrep++
		return
	case errors.Is(err, nodeKvs.ErrResultUnknown):
		stats.acquireUnknown++
		return
	default:
		stats.acquireErr++
		return
	}

	fmt.Println(time.Now(), col.GetLocalNodeID(), "@@ kvs lock ok:", key, lock.Token())
	endReason := "released"

	holdUntil := time.Now().Add(kvsLockHoldMin +
		time.Duration(rand.Int63n(int64(kvsLockHoldMax-kvsLockHoldMin))))
hold:
	for time.Now().Before(holdUntil) {
		select {
		case <-ctx.Done():
			endReason = "shutdown"
			break hold
		case <-lock.Done():
			// self-fencing fired: stop the guarded activity immediately
			stats.lost++
			endReason = fmt.Sprintf("lost(%v)", lock.Err())
			break hold
		case <-time.After(kvsLockWriteInterval):
		}

		writeCtx, cancel := context.WithTimeout(ctx, kvsLoadOpTimeout)
		_, err := lock.Set(writeCtx, kvsLoadValue(key))
		cancel()
		switch {
		case err == nil:
			stats.guardOk++
		case errors.Is(err, nodeKvs.ErrConflict):
			// stale token: the lease moved under us — fencing did its job
			stats.guardConflict++
			endReason = "guard-conflict"
			break hold
		case errors.Is(err, nodeKvs.ErrLockHeld):
			stats.guardLocked++
			endReason = "guard-locked"
			break hold
		case errors.Is(err, nodeKvs.ErrPreparing), errors.Is(err, nodeKvs.ErrResultUnknown):
			// transient; the lease itself is watched via Done()
		default:
			stats.guardErr++
		}
	}

	fmt.Println(time.Now(), col.GetLocalNodeID(), "@@ kvs lock end:", key, lock.Token(), endReason)

	releaseCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := lock.Release(releaseCtx); err != nil {
		stats.releaseErr++
		return
	}
	stats.released++
}
