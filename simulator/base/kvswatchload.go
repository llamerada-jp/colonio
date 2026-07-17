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
	"sync"
	"time"

	colonioNode "github.com/llamerada-jp/colonio/node"
	nodeKvs "github.com/llamerada-jp/colonio/node/kvs"
)

// Watch load for the Stage E verification (spec/kvs/api.md「Watch」): every
// node watches a few keys of the shared kvs-load key space (so the ordinary
// data-plane churn generates the events) and audits what arrives. Enabled
// together with the KVS load (COLONIO_SIM_KVS_INTERVAL_MS).
//
// Audits (offline, over the collected cluster logs, all local to one node):
//   - value integrity: every delivered value must match its key
//     (`@@ kvs watch corrupt` = kvsValueMatches failed — a mis-keyed or
//     corrupted event).
//   - monotonicity: within one watcher, delivered revisions must not go
//     backwards except after a sector counter reset (`@@ kvs watch back` —
//     correlate with force-terminate storms, same caveat as the CAS audit).
//   - convergence (the at-least-once check): the node occasionally writes the
//     watched key itself; the watcher must observe a state with
//     revision >= the acknowledged one within kvsWatchConvergeTimeout
//     (`@@ kvs watch lost` on failure). Coalescing makes ">=" the correct
//     predicate: later writes may have merged over ours.
var kvsWatchKeys = envInt("COLONIO_SIM_KVS_WATCH_KEYS", 4)

const (
	// kvsWatchConvergeTimeout must ride out a host change (subscription lease
	// 15s + keepalive resync) and PREPARING windows (十数秒).
	kvsWatchConvergeTimeout = 45 * time.Second
	// kvsWatchProbeInterval paces the self-write convergence probes.
	kvsWatchProbeInterval = 30 * time.Second
)

// kvsWatchCounters is separated from the mutex so the periodic dump can reset
// it wholesale while holding the lock (zeroing a struct that embeds its own
// sync.Mutex clobbers the held lock — the 2026-07-16 run crashed every node
// process with "unlock of unlocked mutex" on the first dump).
type kvsWatchCounters struct {
	events, deleted              int
	corrupt, back                int
	probes, converged, lost      int
	probePreparing, probeUnknown int
}

type kvsWatchStats struct {
	mtx sync.Mutex
	kvsWatchCounters
}

// startKvsWatchLoad launches the watch workload for one node run, alongside
// startKvsLoad (same enable gate, same lifecycle rules — see the col capture
// note there).
func (n *Node) startKvsWatchLoad(ctx context.Context) {
	if kvsLoadInterval <= 0 || kvsWatchKeys <= 0 {
		return
	}
	col := n.Col

	stats := &kvsWatchStats{}

	// distinct random keys of the shared load key space
	keys := map[string]struct{}{}
	for len(keys) < kvsWatchKeys {
		keys[fmt.Sprintf("kvs-load-%d", rand.Intn(kvsLoadKeys))] = struct{}{}
	}
	for key := range keys {
		go kvsWatchOne(ctx, col, key, stats)
	}

	go func() {
		localNodeID := col.GetLocalNodeID()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			stats.mtx.Lock()
			fmt.Println(time.Now(), localNodeID, "@@ kvs watch:",
				"ev", stats.events, "/del", stats.deleted,
				"/corrupt", stats.corrupt, "/back", stats.back, ",",
				"probe", stats.probes, "/ok", stats.converged, "/lost", stats.lost,
				"/prep", stats.probePreparing, "/unk", stats.probeUnknown)
			stats.kvsWatchCounters = kvsWatchCounters{}
			stats.mtx.Unlock()
		}
	}()
}

// kvsWatchOne consumes one watcher for the node's whole run and injects the
// convergence probes.
func kvsWatchOne(ctx context.Context, col colonioNode.Node, key string, stats *kvsWatchStats) {
	localNodeID := col.GetLocalNodeID()

	watcher, err := col.KVS().Watch(ctx, key)
	if err != nil {
		return // ctx already over
	}

	// probeRevision is the revision our own acknowledged Set returned; the
	// watcher must observe revision >= it before the deadline.
	var probeMtx sync.Mutex
	var probeRevision uint64
	var probeDeadline time.Time

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(kvsWatchProbeInterval +
				time.Duration(rand.Int63n(int64(kvsWatchProbeInterval)))):
			}

			probeMtx.Lock()
			pending := !probeDeadline.IsZero()
			probeMtx.Unlock()
			if pending {
				continue // previous probe still in flight
			}

			opCtx, cancel := context.WithTimeout(ctx, kvsLoadOpTimeout)
			res, err := col.KVS().Set(opCtx, key, kvsLoadValue(key))
			cancel()
			switch {
			case err == nil:
				stats.mtx.Lock()
				stats.probes++
				stats.mtx.Unlock()
				probeMtx.Lock()
				probeRevision = res.Revision
				probeDeadline = time.Now().Add(kvsWatchConvergeTimeout)
				probeMtx.Unlock()
			case ctx.Err() != nil:
				return
			default:
				stats.mtx.Lock()
				if errors.Is(err, nodeKvs.ErrPreparing) {
					stats.probePreparing++
				} else {
					stats.probeUnknown++
				}
				stats.mtx.Unlock()
			}
		}
	}()

	lastRevision := uint64(0)
	checkTicker := time.NewTicker(time.Second)
	defer checkTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-watcher.Events():
			if !ok {
				return
			}
			stats.mtx.Lock()
			stats.events++
			if event.Deleted {
				stats.deleted++
			} else if !kvsValueMatches(key, event.Value) {
				stats.corrupt++
				fmt.Println(time.Now(), localNodeID, "@@ kvs watch corrupt:", key, event.Revision)
			}
			// Revisions regress only after a sector counter reset (data loss);
			// anything else is a delivery-order bug. Deleted events reuse the
			// removed record's revision, so equality is fine.
			if event.Revision < lastRevision {
				stats.back++
				fmt.Println(time.Now(), localNodeID, "@@ kvs watch back:", key, lastRevision, "->", event.Revision)
			}
			stats.mtx.Unlock()
			lastRevision = event.Revision

			probeMtx.Lock()
			if !probeDeadline.IsZero() && event.Revision >= probeRevision {
				probeDeadline = time.Time{}
				stats.mtx.Lock()
				stats.converged++
				stats.mtx.Unlock()
			}
			probeMtx.Unlock()

		case <-checkTicker.C:
			probeMtx.Lock()
			if !probeDeadline.IsZero() && time.Now().After(probeDeadline) {
				probeDeadline = time.Time{}
				stats.mtx.Lock()
				stats.lost++
				stats.mtx.Unlock()
				fmt.Println(time.Now(), localNodeID, "@@ kvs watch lost:", key, probeRevision)
			}
			probeMtx.Unlock()
		}
	}
}
