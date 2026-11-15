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
	"fmt"
	"sync"
	"testing"

	testUtil "github.com/llamerada-jp/colonio/test/util"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/assert"
)

func TestSimpleStore_NewCluster(t *testing.T) {
	sectorIDs := testUtil.UniqueSectorIDs(2)
	sectorNos := testUtil.UniqueNumbersU[kvsTypes.SectorNo](2)

	tests := []struct {
		name      string
		stores    map[kvsTypes.SectorKey]map[string][]byte
		sectorKey *kvsTypes.SectorKey
		expect    map[kvsTypes.SectorKey]any
		wantErr   string
	}{
		{
			name:   "new cluster",
			stores: map[kvsTypes.SectorKey]map[string][]byte{},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			expect: map[kvsTypes.SectorKey]any{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: nil,
			},
		},
		{
			name: "add to existing cluster",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[1],
			},
			expect: map[kvsTypes.SectorKey]any{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: nil,
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[1],
				}: nil,
			},
		},
		{
			name: "add to another cluster",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[1],
				SectorNo: sectorNos[0],
			},
			expect: map[kvsTypes.SectorKey]any{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: nil,
				{
					SectorID: sectorIDs[1],
					SectorNo: sectorNos[0],
				}: nil,
			},
		},
		{
			name: "error: already exists",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			expect: map[kvsTypes.SectorKey]any{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: nil,
			},
			wantErr: "node already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleStore{
				stores: tt.stores,
			}
			err := s.AllocateSector(tt.sectorKey)
			if len(tt.wantErr) > 0 {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.ErrorIs(t, err, kvsTypes.ErrorStoreSectorExists)
			} else {
				assert.NoError(t, err)
			}
			assert.Len(t, s.stores, len(tt.expect))
		})
	}
}

func TestSimpleStore_DeleteCluster(t *testing.T) {
	sectorIDs := testUtil.UniqueSectorIDs(2)
	sectorNos := testUtil.UniqueNumbersU[kvsTypes.SectorNo](2)

	tests := []struct {
		name      string
		stores    map[kvsTypes.SectorKey]map[string][]byte
		sectorKey *kvsTypes.SectorKey
		expect    map[kvsTypes.SectorKey]any
		wantErr   string
	}{
		{
			name: "delete existing cluster",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[1],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			expect: map[kvsTypes.SectorKey]any{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[1],
				}: nil,
			},
		},
		{
			name: "delete last node in cluster",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			expect: map[kvsTypes.SectorKey]any{},
		},
		{
			name: "error: not exists",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[1],
				SectorNo: sectorNos[0],
			},
			expect: map[kvsTypes.SectorKey]any{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: nil,
			},
			wantErr: "node does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleStore{
				stores: tt.stores,
			}
			err := s.ReleaseSector(tt.sectorKey)
			if len(tt.wantErr) > 0 {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.ErrorIs(t, err, kvsTypes.ErrorStoreSectorNotFound)
			} else {
				assert.NoError(t, err)
			}
			assert.Len(t, s.stores, len(tt.expect))
		})
	}
}

func TestSimpleStore_Set(t *testing.T) {
	sectorIDs := testUtil.UniqueSectorIDs(2)
	sectorNos := testUtil.UniqueNumbersU[kvsTypes.SectorNo](2)

	tests := []struct {
		name      string
		stores    map[kvsTypes.SectorKey]map[string][]byte
		sectorKey *kvsTypes.SectorKey
		key       string
		value     []byte
		expect    map[kvsTypes.SectorKey]map[string][]byte
		wantErr   string
	}{
		{
			name: "set new key",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			key:   "foo",
			value: []byte("bar"),
			expect: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
			},
		},
		{
			name: "update existing key",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			key:   "foo",
			value: []byte("baz"),
			expect: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("baz"),
				},
			},
		},
		{
			name: "set key in another node",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[1],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[1],
			},
			key:   "foo",
			value: []byte("baz"),
			expect: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[1],
				}: {
					"foo": []byte("baz"),
				},
			},
		},
		{
			name: "error: node not exists",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[1],
				SectorNo: sectorNos[0],
			},
			key:   "foo",
			value: []byte("bar"),
			expect: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			wantErr: "node does not exist",
		},
		{
			name: "error: nil value",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			key:   "foo",
			value: nil,
			expect: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
			wantErr: "value cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleStore{
				stores: tt.stores,
			}
			err := s.Set(tt.sectorKey, tt.key, tt.value)
			if len(tt.wantErr) > 0 {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expect, s.stores)
		})
	}
}

func TestSimpleStore_Get(t *testing.T) {
	sectorIDs := testUtil.UniqueSectorIDs(2)
	sectorNos := testUtil.UniqueNumbersU[kvsTypes.SectorNo](2)

	tests := []struct {
		name      string
		stores    map[kvsTypes.SectorKey]map[string][]byte
		sectorKey *kvsTypes.SectorKey
		key       string
		expect    []byte
		wantErr   string
	}{
		{
			name: "get existing key",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			key:    "foo",
			expect: []byte("bar"),
		},
		{
			name: "get non-existing key",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			key:    "baz",
			expect: nil,
		},
		{
			name: "get key from another node",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[1],
				}: {
					"foo": []byte("baz"),
				},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[1],
			},
			key:    "foo",
			expect: []byte("baz"),
		},
		{
			name: "error: node not exists",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[1],
				SectorNo: sectorNos[0],
			},
			key:     "foo",
			expect:  nil,
			wantErr: "node does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleStore{
				stores: tt.stores,
			}
			value, err := s.Get(tt.sectorKey, tt.key)
			if len(tt.wantErr) > 0 {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expect, value)
			}
		})
	}
}

func TestSimpleStore_Delete(t *testing.T) {
	sectorIDs := testUtil.UniqueSectorIDs(2)
	sectorNos := testUtil.UniqueNumbersU[kvsTypes.SectorNo](2)

	tests := []struct {
		name      string
		stores    map[kvsTypes.SectorKey]map[string][]byte
		sectorKey *kvsTypes.SectorKey
		key       string
		expect    map[kvsTypes.SectorKey]map[string][]byte
		wantErr   string
	}{
		{
			name: "delete existing key",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
					"baz": []byte("qux"),
				},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			key: "foo",
			expect: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"baz": []byte("qux"),
				},
			},
		},
		{
			name: "delete last key",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			key: "foo",
			expect: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {},
			},
		},
		{
			name: "error: key not exists",
			stores: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &kvsTypes.SectorKey{
				SectorID: sectorIDs[0],
				SectorNo: sectorNos[0],
			},
			key: "baz",
			expect: map[kvsTypes.SectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					SectorNo: sectorNos[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			wantErr: "key does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleStore{
				stores: tt.stores,
			}
			err := s.Delete(tt.sectorKey, tt.key)
			if len(tt.wantErr) > 0 {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expect, s.stores)
		})
	}
}

// Each sector runs its own raft goroutine, so the store is called concurrently
// both for different sectors and for the same sector.
func TestSimpleStore_concurrentAccess(t *testing.T) {
	const sectors = 8
	const keys = 50
	sectorIDs := testUtil.UniqueSectorIDs(sectors)
	shared := &kvsTypes.SectorKey{SectorID: sectorIDs[0], SectorNo: 1}

	s := NewSimpleStore()
	assert.NoError(t, s.AllocateSector(shared))

	var wg sync.WaitGroup
	for i := 1; i < sectors; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sectorKey := &kvsTypes.SectorKey{SectorID: sectorIDs[i], SectorNo: 1}
			for j := 0; j < keys; j++ {
				assert.NoError(t, s.AllocateSector(sectorKey))
				assert.NoError(t, s.Set(sectorKey, "key", []byte("value")))
				_, err := s.Get(sectorKey, "key")
				assert.NoError(t, err)
				assert.NoError(t, s.Delete(sectorKey, "key"))
				assert.NoError(t, s.ReleaseSector(sectorKey))
			}
		}()
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < keys; j++ {
				key := fmt.Sprintf("key-%d-%d", i, j)
				assert.NoError(t, s.Set(shared, key, []byte("value")))
				_, err := s.Get(shared, key)
				assert.NoError(t, err)
				assert.NoError(t, s.Delete(shared, key))
			}
		}()
	}
	wg.Wait()

	assert.Len(t, s.stores, 1)
	assert.Empty(t, s.stores[*shared])
}
