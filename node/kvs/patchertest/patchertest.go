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

// Package patchertest checks a kvs Patcher against its determinism contract.
// A Patcher runs inside the raft apply on every replica of a record's sector,
// so a non-deterministic implementation does not just misbehave — it silently
// diverges the replicated state (see types/kvs.Patcher). Run
// AssertDeterministic over representative (current, patch) inputs in the
// application's tests before registering a Patcher.
//
// The check is necessarily heuristic: it catches randomness, iteration-order
// dependence, hidden mutable state and input mutation, but it cannot prove
// purity (e.g. clock dependence that changes slower than the test runs).
package patchertest

import (
	"bytes"
	"sync"
	"testing"

	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

const (
	sequentialRounds = 20
	concurrentCalls  = 8
)

// AssertDeterministic applies the patcher to the same (current, patch) input
// repeatedly — sequentially and concurrently — and fails the test unless
// every call returns identical bytes (or the identical error) and the inputs
// are left unmodified.
func AssertDeterministic(t testing.TB, patcher kvsTypes.Patcher, current, patch []byte) {
	t.Helper()

	// Defensive copies to detect input mutation afterwards.
	currentOrig := bytes.Clone(current)
	patchOrig := bytes.Clone(patch)

	baseResult, baseErr := patcher.Apply(current, patch)

	check := func(result []byte, err error) {
		t.Helper()
		if (err == nil) != (baseErr == nil) ||
			(err != nil && err.Error() != baseErr.Error()) {
			t.Errorf("patcher error is not deterministic: %v vs %v", err, baseErr)
			return
		}
		if !bytes.Equal(result, baseResult) {
			t.Errorf("patcher result is not deterministic:\n first: %q\n later: %q", baseResult, result)
		}
	}

	for range sequentialRounds {
		result, err := patcher.Apply(current, patch)
		check(result, err)
	}

	var wg sync.WaitGroup
	results := make([][]byte, concurrentCalls)
	errs := make([]error, concurrentCalls)
	for i := range concurrentCalls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = patcher.Apply(current, patch)
		}()
	}
	wg.Wait()
	for i := range concurrentCalls {
		check(results[i], errs[i])
	}

	if !bytes.Equal(current, currentOrig) {
		t.Errorf("patcher mutated the current value input")
	}
	if !bytes.Equal(patch, patchOrig) {
		t.Errorf("patcher mutated the patch input")
	}
}
