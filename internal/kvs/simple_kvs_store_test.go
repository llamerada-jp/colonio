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
	"testing"

	"github.com/llamerada-jp/colonio/config"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
)

func TestSimpleKvsStore_NewCluster(t *testing.T) {
	sectorIDs := testUtil.UniqueUUIDs(2)
	sequences := testUtil.UniqueNumbers[config.KvsSequence](2)

	tests := []struct {
		name      string
		stores    map[config.KvsSectorKey]map[string][]byte
		sectorKey *config.KvsSectorKey
		expect    map[config.KvsSectorKey]any
		wantErr   string
	}{
		{
			name:   "new cluster",
			stores: map[config.KvsSectorKey]map[string][]byte{},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			expect: map[config.KvsSectorKey]any{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: nil,
			},
		},
		{
			name: "add to existing cluster",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[1],
			},
			expect: map[config.KvsSectorKey]any{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: nil,
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[1],
				}: nil,
			},
		},
		{
			name: "add to another cluster",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[1],
				Sequence: sequences[0],
			},
			expect: map[config.KvsSectorKey]any{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: nil,
				{
					SectorID: sectorIDs[1],
					Sequence: sequences[0],
				}: nil,
			},
		},
		{
			name: "error: already exists",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			expect: map[config.KvsSectorKey]any{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: nil,
			},
			wantErr: "node already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleKvsStore{
				stores: tt.stores,
			}
			err := s.AllocateSector(tt.sectorKey)
			if len(tt.wantErr) > 0 {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Len(t, s.stores, len(tt.expect))
		})
	}
}

func TestSimpleKvsStore_DeleteCluster(t *testing.T) {
	sectorIDs := testUtil.UniqueUUIDs(2)
	sequences := testUtil.UniqueNumbers[config.KvsSequence](2)

	tests := []struct {
		name      string
		stores    map[config.KvsSectorKey]map[string][]byte
		sectorKey *config.KvsSectorKey
		expect    map[config.KvsSectorKey]any
		wantErr   string
	}{
		{
			name: "delete existing cluster",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[1],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			expect: map[config.KvsSectorKey]any{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[1],
				}: nil,
			},
		},
		{
			name: "delete last node in cluster",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			expect: map[config.KvsSectorKey]any{},
		},
		{
			name: "error: not exists",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[1],
				Sequence: sequences[0],
			},
			expect: map[config.KvsSectorKey]any{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: nil,
			},
			wantErr: "node does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleKvsStore{
				stores: tt.stores,
			}
			err := s.ReleaseSector(tt.sectorKey)
			if len(tt.wantErr) > 0 {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Len(t, s.stores, len(tt.expect))
		})
	}
}

func TestSimpleKvsStore_Set(t *testing.T) {
	sectorIDs := testUtil.UniqueUUIDs(2)
	sequences := testUtil.UniqueNumbers[config.KvsSequence](2)

	tests := []struct {
		name      string
		stores    map[config.KvsSectorKey]map[string][]byte
		sectorKey *config.KvsSectorKey
		key       string
		value     []byte
		expect    map[config.KvsSectorKey]map[string][]byte
		wantErr   string
	}{
		{
			name: "set new key",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			key:   "foo",
			value: []byte("bar"),
			expect: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
			},
		},
		{
			name: "update existing key",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			key:   "foo",
			value: []byte("baz"),
			expect: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("baz"),
				},
			},
		},
		{
			name: "set key in another node",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[1],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[1],
			},
			key:   "foo",
			value: []byte("baz"),
			expect: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[1],
				}: {
					"foo": []byte("baz"),
				},
			},
		},
		{
			name: "error: node not exists",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[1],
				Sequence: sequences[0],
			},
			key:   "foo",
			value: []byte("bar"),
			expect: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			wantErr: "node does not exist",
		},
		{
			name: "error: nil value",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			key:   "foo",
			value: nil,
			expect: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
			wantErr: "value cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleKvsStore{
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

func TestSimpleKvsStore_Get(t *testing.T) {
	sectorIDs := testUtil.UniqueUUIDs(2)
	sequences := testUtil.UniqueNumbers[config.KvsSequence](2)

	tests := []struct {
		name      string
		stores    map[config.KvsSectorKey]map[string][]byte
		sectorKey *config.KvsSectorKey
		key       string
		expect    []byte
		wantErr   string
	}{
		{
			name: "get existing key",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			key:    "foo",
			expect: []byte("bar"),
		},
		{
			name: "get non-existing key",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			key:    "baz",
			expect: nil,
		},
		{
			name: "get key from another node",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[1],
				}: {
					"foo": []byte("baz"),
				},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[1],
			},
			key:    "foo",
			expect: []byte("baz"),
		},
		{
			name: "error: node not exists",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[1],
				Sequence: sequences[0],
			},
			key:     "foo",
			expect:  nil,
			wantErr: "node does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleKvsStore{
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

func TestSimpleKvsStore_Delete(t *testing.T) {
	sectorIDs := testUtil.UniqueUUIDs(2)
	sequences := testUtil.UniqueNumbers[config.KvsSequence](2)

	tests := []struct {
		name      string
		stores    map[config.KvsSectorKey]map[string][]byte
		sectorKey *config.KvsSectorKey
		key       string
		expect    map[config.KvsSectorKey]map[string][]byte
		wantErr   string
	}{
		{
			name: "delete existing key",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
					"baz": []byte("qux"),
				},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			key: "foo",
			expect: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"baz": []byte("qux"),
				},
			},
		},
		{
			name: "delete last key",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			key: "foo",
			expect: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {},
			},
		},
		{
			name: "error: key not exists",
			stores: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorIDs[0],
				Sequence: sequences[0],
			},
			key: "baz",
			expect: map[config.KvsSectorKey]map[string][]byte{
				{
					SectorID: sectorIDs[0],
					Sequence: sequences[0],
				}: {
					"foo": []byte("bar"),
				},
			},
			wantErr: "key does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SimpleKvsStore{
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
