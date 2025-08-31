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

	"github.com/llamerada-jp/colonio/config"
)

type SimpleKvsStore struct {
	stores map[config.KvsSectorKey]map[string][]byte
}

var _ config.KvsStore = (*SimpleKvsStore)(nil)

func NewSimpleKvsStore() *SimpleKvsStore {
	return &SimpleKvsStore{
		stores: make(map[config.KvsSectorKey]map[string][]byte),
	}
}

func (s *SimpleKvsStore) AllocateSector(sectorKey *config.KvsSectorKey) error {
	if _, exists := s.stores[*sectorKey]; exists {
		return fmt.Errorf("node already exists: %s", sectorKey.SectorID.String())
	}
	s.stores[*sectorKey] = make(map[string][]byte)
	return nil
}

func (s *SimpleKvsStore) ReleaseSector(sectorKey *config.KvsSectorKey) error {
	if _, exists := s.stores[*sectorKey]; !exists {
		return fmt.Errorf("node does not exist: %s", sectorKey.SectorID.String())
	}
	delete(s.stores, *sectorKey)
	return nil
}

func (s *SimpleKvsStore) Set(sectorKey *config.KvsSectorKey, key string, value []byte) error {
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}
	if _, exists := s.stores[*sectorKey]; !exists {
		return fmt.Errorf("node does not exist: %s", sectorKey.SectorID.String())
	}
	s.stores[*sectorKey][key] = value
	return nil
}

func (s *SimpleKvsStore) Get(sectorKey *config.KvsSectorKey, key string) ([]byte, error) {
	if _, exists := s.stores[*sectorKey]; !exists {
		return nil, fmt.Errorf("node does not exist: %s", sectorKey.SectorID.String())
	}
	value, exists := s.stores[*sectorKey][key]
	if !exists {
		return nil, nil // or return an error if preferred
	}
	return value, nil
}

func (s *SimpleKvsStore) Patch(sectorKey *config.KvsSectorKey, key string, value []byte) error {
	panic("Patch not implemented in SimpleKvsStore")
}

func (s *SimpleKvsStore) Delete(sectorKey *config.KvsSectorKey, key string) error {
	if _, exists := s.stores[*sectorKey]; !exists {
		return fmt.Errorf("node does not exist: %s", sectorKey.SectorID.String())
	}
	if _, exists := s.stores[*sectorKey][key]; !exists {
		return fmt.Errorf("key does not exist: %s", key)
	}
	delete(s.stores[*sectorKey], key)
	return nil
}
