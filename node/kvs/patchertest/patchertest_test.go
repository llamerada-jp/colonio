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
package patchertest

import (
	"fmt"
	"sync/atomic"
	"testing"
)

type appendPatcher struct{}

func (appendPatcher) Apply(current []byte, patch []byte) ([]byte, error) {
	result := make([]byte, 0, len(current)+len(patch))
	result = append(result, current...)
	return append(result, patch...), nil
}

type failPatcher struct{}

func (failPatcher) Apply(current []byte, patch []byte) ([]byte, error) {
	return nil, fmt.Errorf("always rejected")
}

// statefulPatcher violates the purity contract: the result depends on hidden
// mutable state.
type statefulPatcher struct{ calls atomic.Int64 }

func (p *statefulPatcher) Apply(current []byte, patch []byte) ([]byte, error) {
	return fmt.Appendf(nil, "%s-%d", current, p.calls.Add(1)), nil
}

// recordingTB captures failures instead of failing the real test.
type recordingTB struct {
	testing.TB
	failed bool
}

func (r *recordingTB) Errorf(format string, args ...any) { r.failed = true }
func (r *recordingTB) Helper()                           {}

func TestAssertDeterministicPasses(t *testing.T) {
	AssertDeterministic(t, appendPatcher{}, []byte("value"), []byte("+p"))
	// a deterministic error is also a valid, stable outcome
	AssertDeterministic(t, failPatcher{}, []byte("value"), []byte("+p"))
}

func TestAssertDeterministicCatchesState(t *testing.T) {
	recorder := &recordingTB{TB: t}
	AssertDeterministic(recorder, &statefulPatcher{}, []byte("value"), []byte("+p"))
	if !recorder.failed {
		t.Fatal("a stateful patcher must be reported as non-deterministic")
	}
}
