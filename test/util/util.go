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
package util

import (
	"crypto/tls"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

// make unique nodeIDs
func UniqueNodeIDs(count int) []*types.NodeID {
	nodeIDs := make([]*types.NodeID, count)
	exists := make(map[types.NodeID]struct{})

	for i := range nodeIDs {
		for {
			nodeID := types.NewRandomNodeID()
			_, ok := exists[*nodeID]
			if !ok {
				nodeIDs[i] = nodeID
				exists[*nodeID] = struct{}{}
				break
			}
		}
	}

	return nodeIDs
}

// make unique nodeIDs with min max range (min <= nodeID < max)
func UniqueNodeIDsWithRange(min, max *types.NodeID, count int) []*types.NodeID {
	if !min.Smaller(max) {
		panic("min must be smaller than max")
	}
	nodeIDs := make([]*types.NodeID, count)
	exists := make(map[types.NodeID]struct{})

	for i := range nodeIDs {
		for {
			nodeID := types.NewRandomNodeID()
			if nodeID.Smaller(min) || !nodeID.Smaller(max) {
				continue
			}
			_, ok := exists[*nodeID]
			if !ok {
				nodeIDs[i] = nodeID
				exists[*nodeID] = struct{}{}
				break
			}
		}
	}

	return nodeIDs
}

func UniqueSectorIDs(count int) []kvsTypes.SectorID {
	uuids := make([]kvsTypes.SectorID, count)
	exists := make(map[kvsTypes.SectorID]struct{})
	for i := range uuids {
		for {
			id := kvsTypes.SectorID(uuid.New())
			if _, ok := exists[id]; !ok {
				uuids[i] = id
				exists[id] = struct{}{}
				break
			}
		}
	}
	return uuids
}

func UniqueNumbersU[V ~uint | ~uint32 | ~uint64](count int) []V {
	nums := make([]V, count)
	exists := make(map[V]struct{})
	for i := range nums {
		for {
			n := V(rand.Uint64())
			if _, ok := exists[n]; !ok {
				nums[i] = n
				exists[n] = struct{}{}
				break
			}
		}
	}
	return nums
}

// create client to accept self-signed certificate & cookie
func NewInsecureHttpClient() *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Jar: jar,
	}
}

func NearTime(base, actual time.Time) bool {
	// Allow a 3-second margin for time differences
	margin := 3 * time.Second
	return actual.After(base.Add(-margin)) && actual.Before(base.Add(margin))
}

func CompareNodeIDsOrdered(a, b []*types.NodeID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

func CompareNodeIDsUnordered(a, b []*types.NodeID) bool {
	if len(a) != len(b) {
		return false
	}
	c := slices.Clone(b)
	for _, idA := range a {
		for i, idC := range c {
			if idA.Equal(idC) {
				c = append(c[:i], c[i+1:]...) // remove matched idC
				break
			}
		}
	}
	return len(c) == 0
}

type testingLogWriter struct {
	mu   sync.Mutex
	t    *testing.T
	done bool
}

func (w *testingLogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.done {
		w.t.Log(string(p))
	}
	return len(p), nil
}

func (w *testingLogWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.done = true
}

func Logger(t *testing.T) *slog.Logger {
	w := &testingLogWriter{t: t}
	t.Cleanup(w.close)
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{}))
}
