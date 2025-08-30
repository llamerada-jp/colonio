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
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/llamerada-jp/colonio/internal/shared"
)

// make unique nodeIDs
func UniqueNodeIDs(count int) []*shared.NodeID {
	nodeIDs := make([]*shared.NodeID, count)
	exists := make(map[shared.NodeID]struct{})

	for i := range nodeIDs {
		for {
			nodeID := shared.NewRandomNodeID()
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
func UniqueNodeIDsWithRange(min, max *shared.NodeID, count int) []*shared.NodeID {
	if !min.Smaller(max) {
		panic("min must be smaller than max")
	}
	nodeIDs := make([]*shared.NodeID, count)
	exists := make(map[shared.NodeID]struct{})

	for i := range nodeIDs {
		for {
			nodeID := shared.NewRandomNodeID()
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

func UniqueUUIDs(count int) []uuid.UUID {
	uuids := make([]uuid.UUID, count)
	exists := make(map[uuid.UUID]struct{})
	for i := range uuids {
		for {
			id := uuid.New()
			if _, ok := exists[id]; !ok {
				uuids[i] = id
				exists[id] = struct{}{}
				break
			}
		}
	}
	return uuids
}

func UniqueNumbers[V ~int | ~int32 | ~int64 | ~uint | ~uint32 | ~uint64](count int) []V {
	nums := make([]V, count)
	exists := make(map[V]struct{})
	for i := range nums {
		for {
			var n V
			switch any(n).(type) {
			case int:
				n = V(rand.Int())
			case int32:
				n = V(rand.Int31())
			case int64:
				n = V(rand.Int63())
			case uint:
				n = V(rand.Uint32())
			case uint32:
				n = V(rand.Uint32())
			case uint64:
				n = V(rand.Uint64())
			}
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

func CompareNodeIDsOrdered(a, b []*shared.NodeID) bool {
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

func CompareNodeIDsUnordered(a, b []*shared.NodeID) bool {
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
	t *testing.T
}

func (w *testingLogWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}

func Logger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(&testingLogWriter{
		t: t,
	}, &slog.HandlerOptions{}))
}
