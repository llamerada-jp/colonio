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
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	colonioNode "github.com/llamerada-jp/colonio/node"
	nodeKvs "github.com/llamerada-jp/colonio/node/kvs"
)

// KVS write-load generator for the Stage 6 verification of the raft snapshot
// (spec/kvs/dataplane.md, spec/kvs/snapshot.md): large values × frequent
// overwrites make the data plane the dominant source of raft log entries, so
// snapshot/compaction must bound the memory while the churn keeps
// splitting/merging the sectors underneath.
//
// Enabled by COLONIO_SIM_KVS_INTERVAL_MS > 0. The key space is shared by all
// nodes (keys hash across the whole ring), values embed the key so a read can
// detect cross-key corruption, and a successful Set is immediately probed
// with a Get to catch acknowledged-but-lost writes (rare concurrent Deletes
// by other nodes are the only legitimate cause of a probe miss).
//
// PREPARING retries are handled by the public client (node/kvs) itself; each
// operation is bounded by kvsLoadOpTimeout through the context deadline, so a
// range stuck in preparing longer than that shows up as the "prep" counter.
var (
	// kvsLoadInterval is the delay between operations per node; 0 disables the load.
	kvsLoadInterval = time.Duration(envInt("COLONIO_SIM_KVS_INTERVAL_MS", 0)) * time.Millisecond
	// kvsLoadKeys is the size of the shared key space.
	kvsLoadKeys = envInt("COLONIO_SIM_KVS_KEYS", 256)
	// kvsLoadValueSize is the value payload size in bytes.
	kvsLoadValueSize = envInt("COLONIO_SIM_KVS_VALUE_SIZE", 4096)
)

// kvsLoadOpTimeout bounds one operation including the client's built-in
// PREPARING retries. Longer than the churn fence windows the retry is meant
// to ride out (十数秒, spec/kvs/dataplane.md), yet short enough to keep the
// per-node load loop from stalling across runs.
const kvsLoadOpTimeout = 15 * time.Second

// kvsLoadPatcherName is the reference Patcher registered on every simulator
// node (renewColonio).
const kvsLoadPatcherName = "sim-inc"

// kvsLoadIncPatcher increments the middle field of the "key|N|padding" value
// format in place. A deterministic pure function on the value bytes (the
// patch document is unused), so replicas stay identical — which is exactly
// what the run verifies: a divergence would surface as verify corrupt after
// host changes or snapshot restores.
type kvsLoadIncPatcher struct{}

func (p *kvsLoadIncPatcher) Apply(current []byte, patch []byte) ([]byte, error) {
	parts := bytes.SplitN(current, []byte("|"), 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("value is not in key|N|padding format")
	}
	n, err := strconv.ParseUint(string(parts[1]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("counter field is not a number: %w", err)
	}
	return bytes.Join([][]byte{parts[0], []byte(strconv.FormatUint(n+1, 10)), parts[2]}, []byte("|")), nil
}

func envInt(name string, defaultValue int) int {
	value := os.Getenv(name)
	if len(value) == 0 {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		panic(fmt.Sprintf("invalid %s: %s", name, value))
	}
	return parsed
}

// kvsLoadStats accumulates per-node counters, dumped every minute with the
// "@@" marker so the simulator's log collection keeps them. "prep" counts
// operations whose deadline expired while the range was still preparing
// (nothing was accepted); "unk" counts writes with an unknown outcome
// (timeout in flight — the write may still apply); "conf" counts CAS
// conflicts (expected under contention: the shared key space makes nodes
// race on the same records).
type kvsLoadStats struct {
	set, setPreparing, setUnknown, setError              int
	cas, casConflict, casPreparing, casUnknown, casError int
	patch, patchNotFound, patchPreparing, patchUnknown   int
	patchError                                           int
	get, getNotFound, getPreparing, getError             int
	del, delNotFound, delPreparing, delUnknown, delErr   int
	verifyMiss, verifyCorrupt                            int
}

// memReporterOnce starts one memory reporter per process: the raft logs of
// every sector replica hosted by this process live on its heap, so a bounded
// heap under sustained writes is the pass signal for snapshot/compaction.
// Runs for the process lifetime (node runs are 1–19 minutes each, and the
// sync.Once must not tie the reporter to the first run's context).
var memReporterOnce sync.Once

func startKvsMemReporter() {
	memReporterOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				fmt.Println(time.Now(), "== kvs mem:",
					"heapAlloc", ms.HeapAlloc/1024/1024, "MiB,",
					"heapSys", ms.HeapSys/1024/1024, "MiB,",
					"numGC", ms.NumGC)
			}
		}()
	})
}

// startKvsLoad launches the workload for one node run. It returns immediately
// when the load is disabled. The loop stops with the node's run context, so
// the load follows the node's start/stop churn.
func (n *Node) startKvsLoad(ctx context.Context) {
	if kvsLoadInterval <= 0 {
		return
	}
	startKvsMemReporter()

	// Capture the colonio instance of THIS run. n.Col is replaced by
	// renewColonio for the next run while this goroutine may still be inside
	// an operation, and calling into the swapped-in, not-yet-started instance
	// crashes in the routing layer (routing1D is created by Start; SIGSEGV
	// via the KVS Set path, 2026-07-12 run). The captured instance was
	// already started when startKvsLoad runs, so it stays safe to call after
	// Stop — requests then resolve as errors or hit the context deadline.
	col := n.Col

	go func() {
		localNodeID := col.GetLocalNodeID()
		stats := &kvsLoadStats{}
		lastDump := time.Now()

		ticker := time.NewTicker(kvsLoadInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			kvsLoadOperation(ctx, col, stats)

			if time.Since(lastDump) >= time.Minute {
				fmt.Println(time.Now(), localNodeID, "@@ kvs load:",
					"set", stats.set, "/prep", stats.setPreparing, "/unk", stats.setUnknown, "/err", stats.setError, ",",
					"cas", stats.cas, "/conf", stats.casConflict, "/prep", stats.casPreparing, "/unk", stats.casUnknown, "/err", stats.casError, ",",
					"patch", stats.patch, "/nf", stats.patchNotFound, "/prep", stats.patchPreparing, "/unk", stats.patchUnknown, "/err", stats.patchError, ",",
					"get", stats.get, "/nf", stats.getNotFound, "/prep", stats.getPreparing, "/err", stats.getError, ",",
					"del", stats.del, "/nf", stats.delNotFound, "/prep", stats.delPreparing, "/unk", stats.delUnknown, "/err", stats.delErr, ",",
					"miss", stats.verifyMiss, "corrupt", stats.verifyCorrupt)
				*stats = kvsLoadStats{}
				lastDump = time.Now()
			}
		}
	}()
}

// kvsLoadOperation runs one randomly chosen operation: mostly overwrites
// (they grow the raft log without growing the live data set — exactly the
// case snapshots must bound), some conditional read-modify-writes (CAS),
// some reads, few deletes.
func kvsLoadOperation(ctx context.Context, col colonioNode.Node, stats *kvsLoadStats) {
	key := fmt.Sprintf("kvs-load-%d", rand.Intn(kvsLoadKeys))
	kv := col.KVS()

	opCtx, cancel := context.WithTimeout(ctx, kvsLoadOpTimeout)
	defer cancel()

	switch r := rand.Intn(100); {
	case r < 50: // Set + verify probe
		_, err := kv.Set(opCtx, key, kvsLoadValue(key))
		switch {
		case err == nil:
			stats.set++
			kvsVerifyProbe(ctx, col, key, stats)
		case errors.Is(err, nodeKvs.ErrPreparing):
			stats.setPreparing++
		case errors.Is(err, nodeKvs.ErrResultUnknown):
			stats.setUnknown++
		default:
			stats.setError++
		}

	case r < 65: // CAS read-modify-write
		// Read the current revision, then write conditionally on it. Under
		// contention (the key space is shared cluster-wide) conflicts are the
		// expected correct outcome; a lost update would show up as two
		// successful conditional writes built on the same base revision.
		result, err := kv.Get(opCtx, key)
		var baseRevision uint64
		switch {
		case err == nil:
			baseRevision = result.Revision
		case errors.Is(err, nodeKvs.ErrNotFound):
			baseRevision = 0 // create guarded by absence
		case errors.Is(err, nodeKvs.ErrPreparing):
			stats.casPreparing++
			return
		default:
			stats.casError++
			return
		}

		condition := nodeKvs.WithAbsent()
		if baseRevision != 0 {
			condition = nodeKvs.WithRevision(baseRevision)
		}
		_, err = kv.Set(opCtx, key, kvsLoadValue(key), condition)
		switch {
		case err == nil:
			stats.cas++
			// Lost-update audit line: CAS correctness means at most one
			// success per (key, base revision) across the whole cluster —
			// revisions are never reused within a sector lineage, so a
			// duplicated pair in the collected logs is a lost update. The
			// base==0 (absence) case is excluded: delete/re-create makes
			// absence legitimately winnable more than once. (Sector data
			// loss on majority failure resets the counter and can produce a
			// rare false positive; correlate with force-terminate storms.)
			if baseRevision != 0 {
				fmt.Println(time.Now(), col.GetLocalNodeID(), "@@ kvs cas ok:", key, baseRevision)
			}
		case errors.Is(err, nodeKvs.ErrConflict):
			stats.casConflict++
		case errors.Is(err, nodeKvs.ErrPreparing):
			stats.casPreparing++
		case errors.Is(err, nodeKvs.ErrResultUnknown):
			stats.casUnknown++
		default:
			stats.casError++
		}

	case r < 70: // Patch (server-side partial update via the registered Patcher)
		// Only the patch intent travels; every replica increments the value's
		// counter field locally. The read-back probe checks the value is
		// still well-formed for its key — a non-deterministic patcher or a
		// divergence on migration would surface as verify corrupt.
		_, err := kv.Patch(opCtx, key, kvsLoadPatcherName, nil)
		switch {
		case err == nil:
			stats.patch++
			kvsVerifyProbe(ctx, col, key, stats)
		case errors.Is(err, nodeKvs.ErrNotFound):
			stats.patchNotFound++ // patching an absent record; creation is Set's job
		case errors.Is(err, nodeKvs.ErrPreparing):
			stats.patchPreparing++
		case errors.Is(err, nodeKvs.ErrResultUnknown):
			stats.patchUnknown++
		default:
			// includes ErrPatchFailed, which must not happen: the patcher is
			// registered on every node and the value format is fixed
			stats.patchError++
			fmt.Println(time.Now(), col.GetLocalNodeID(), "@@ kvs patch err:", key, err)
		}

	case r < 95: // Get
		result, err := kv.Get(opCtx, key)
		switch {
		case err == nil:
			stats.get++
			if !kvsValueMatches(key, result.Value) {
				stats.verifyCorrupt++
				fmt.Println(time.Now(), col.GetLocalNodeID(), "@@ kvs verify corrupt:", key)
			}
		case errors.Is(err, nodeKvs.ErrNotFound):
			stats.getNotFound++
		case errors.Is(err, nodeKvs.ErrPreparing):
			stats.getPreparing++
		default:
			stats.getError++
		}

	default: // Delete
		err := kv.Delete(opCtx, key)
		switch {
		case err == nil:
			stats.del++
		case errors.Is(err, nodeKvs.ErrNotFound):
			stats.delNotFound++
		case errors.Is(err, nodeKvs.ErrPreparing):
			stats.delPreparing++
		case errors.Is(err, nodeKvs.ErrResultUnknown):
			stats.delUnknown++
		default:
			stats.delErr++
		}
	}
}

// kvsVerifyProbe reads back a key right after its acknowledged Set. A miss
// means the write disappeared: legitimate only when another node's Delete
// interleaved (deletes are 5% of the mix, so a sustained miss rate points at
// a lost-write bug — the fences of spec/kvs/dataplane.md). Preparing/other
// probe failures are not counted as misses: the probe is best-effort.
func kvsVerifyProbe(ctx context.Context, col colonioNode.Node, key string, stats *kvsLoadStats) {
	opCtx, cancel := context.WithTimeout(ctx, kvsLoadOpTimeout)
	defer cancel()

	result, err := col.KVS().Get(opCtx, key)
	switch {
	case err == nil:
		if !kvsValueMatches(key, result.Value) {
			stats.verifyCorrupt++
			fmt.Println(time.Now(), col.GetLocalNodeID(), "@@ kvs verify corrupt:", key)
		}
	case errors.Is(err, nodeKvs.ErrNotFound):
		stats.verifyMiss++
		fmt.Println(time.Now(), col.GetLocalNodeID(), "@@ kvs verify miss:", key)
	case errors.Is(err, nodeKvs.ErrPreparing):
		stats.getPreparing++
	default:
		stats.getError++
	}
}

// kvsLoadValue builds a value that identifies its key (corruption check) and
// changes on every write (overwrite churn), padded to kvsLoadValueSize.
func kvsLoadValue(key string) []byte {
	header := fmt.Sprintf("%s|%d|", key, time.Now().UnixNano())
	value := make([]byte, 0, kvsLoadValueSize)
	value = append(value, header...)
	for len(value) < kvsLoadValueSize {
		value = append(value, 'x')
	}
	return value
}

func kvsValueMatches(key string, value []byte) bool {
	return bytes.HasPrefix(value, []byte(key+"|"))
}
