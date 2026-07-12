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

	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
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
var (
	// kvsLoadInterval is the delay between operations per node; 0 disables the load.
	kvsLoadInterval = time.Duration(envInt("COLONIO_SIM_KVS_INTERVAL_MS", 0)) * time.Millisecond
	// kvsLoadKeys is the size of the shared key space.
	kvsLoadKeys = envInt("COLONIO_SIM_KVS_KEYS", 256)
	// kvsLoadValueSize is the value payload size in bytes.
	kvsLoadValueSize = envInt("COLONIO_SIM_KVS_VALUE_SIZE", 4096)
)

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
// "@@" marker so the simulator's log collection keeps them.
type kvsLoadStats struct {
	set, setPreparing, setTimeout, setError    int
	get, getNotFound, getPreparing, getError   int
	del, delNotFound, delPreparing, delError   int
	verifyMiss, verifyCorrupt, retryExhausted  int
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

	go func() {
		localNodeID := n.Col.GetLocalNodeID()
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

			n.kvsLoadOperation(ctx, stats)

			if time.Since(lastDump) >= time.Minute {
				fmt.Println(time.Now(), localNodeID, "@@ kvs load:",
					"set", stats.set, "/prep", stats.setPreparing, "/timeout", stats.setTimeout, "/err", stats.setError, ",",
					"get", stats.get, "/nf", stats.getNotFound, "/prep", stats.getPreparing, "/err", stats.getError, ",",
					"del", stats.del, "/nf", stats.delNotFound, "/prep", stats.delPreparing, "/err", stats.delError, ",",
					"miss", stats.verifyMiss, "corrupt", stats.verifyCorrupt, "exhausted", stats.retryExhausted)
				*stats = kvsLoadStats{}
				lastDump = time.Now()
			}
		}
	}()
}

// kvsLoadOperation runs one randomly chosen operation: mostly overwrites
// (they grow the raft log without growing the live data set — exactly the
// case snapshots must bound), some reads, few deletes.
func (n *Node) kvsLoadOperation(ctx context.Context, stats *kvsLoadStats) {
	key := fmt.Sprintf("kvs-load-%d", rand.Intn(kvsLoadKeys))

	switch r := rand.Intn(100); {
	case r < 70: // Set + verify probe
		err := n.kvsRetry(ctx, func() error { return <-n.Col.KvsSet(key, kvsLoadValue(key)) },
			&stats.setPreparing, &stats.retryExhausted)
		switch {
		case err == nil:
			stats.set++
			n.kvsVerifyProbe(ctx, key, stats)
		case errors.Is(err, kvsTimeout):
			stats.setTimeout++
		default:
			stats.setError++
		}

	case r < 95: // Get
		err := n.kvsRetry(ctx, func() error {
			result := <-n.Col.KvsGet(key)
			if result.Err == nil && !kvsValueMatches(key, result.Data) {
				stats.verifyCorrupt++
				fmt.Println(time.Now(), n.Col.GetLocalNodeID(), "@@ kvs verify corrupt:", key)
			}
			return result.Err
		}, &stats.getPreparing, &stats.retryExhausted)
		switch {
		case err == nil:
			stats.get++
		case errors.Is(err, kvsTypes.ErrorStoreKeyNotFound):
			stats.getNotFound++
		default:
			stats.getError++
		}

	default: // Delete
		err := n.kvsRetry(ctx, func() error { return <-n.Col.KvsDelete(key) },
			&stats.delPreparing, &stats.retryExhausted)
		switch {
		case err == nil:
			stats.del++
		case errors.Is(err, kvsTypes.ErrorStoreKeyNotFound):
			stats.delNotFound++
		default:
			stats.delError++
		}
	}
}

// kvsVerifyProbe reads back a key right after its acknowledged Set. A miss
// means the write disappeared: legitimate only when another node's Delete
// interleaved (deletes are 5% of the mix, so a sustained miss rate points at
// a lost-write bug — the fences of spec/kvs/dataplane.md).
func (n *Node) kvsVerifyProbe(ctx context.Context, key string, stats *kvsLoadStats) {
	err := n.kvsRetry(ctx, func() error {
		result := <-n.Col.KvsGet(key)
		if result.Err == nil && !kvsValueMatches(key, result.Data) {
			stats.verifyCorrupt++
			fmt.Println(time.Now(), n.Col.GetLocalNodeID(), "@@ kvs verify corrupt:", key)
		}
		return result.Err
	}, &stats.getPreparing, &stats.retryExhausted)
	if errors.Is(err, kvsTypes.ErrorStoreKeyNotFound) {
		stats.verifyMiss++
		fmt.Println(time.Now(), n.Col.GetLocalNodeID(), "@@ kvs verify miss:", key)
	}
}

// kvsTimeout marks retry exhaustion on the retryable (PREPARING) class; the
// operation may or may not have taken effect.
var kvsTimeout = errors.New("kvs operation retries exhausted")

// kvsRetry retries the operation while it fails with the retryable
// ErrorSectorNotReady (routing not settled, range mid-split/merge). Bounded:
// the load must not pile up goroutines against a stuck range.
func (n *Node) kvsRetry(ctx context.Context, operation func() error, preparing *int, exhausted *int) error {
	const attempts = 5
	for i := 0; ; i++ {
		err := operation()
		if !errors.Is(err, kvsTypes.ErrorSectorNotReady) {
			return err
		}
		*preparing++
		if i >= attempts-1 {
			*exhausted++
			return kvsTimeout
		}
		select {
		case <-ctx.Done():
			return kvsTimeout
		case <-time.After(time.Duration(200*(i+1)) * time.Millisecond):
		}
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
