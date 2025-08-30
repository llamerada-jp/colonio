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

type SimpleKVSStore struct {
	stores map[config.KVSNodeKey]map[string][]byte
}

var _ config.KVSStore = (*SimpleKVSStore)(nil)

func NewSimpleKVSStore() *SimpleKVSStore {
	return &SimpleKVSStore{
		stores: make(map[config.KVSNodeKey]map[string][]byte),
	}
}

func (s *SimpleKVSStore) NewCluster(nodeKey *config.KVSNodeKey) error {
	if _, exists := s.stores[*nodeKey]; exists {
		return fmt.Errorf("node already exists: %s", nodeKey.ClusterID.String())
	}
	s.stores[*nodeKey] = make(map[string][]byte)
	return nil
}

func (s *SimpleKVSStore) DeleteCluster(nodeKey *config.KVSNodeKey) error {
	if _, exists := s.stores[*nodeKey]; !exists {
		return fmt.Errorf("node does not exist: %s", nodeKey.ClusterID.String())
	}
	delete(s.stores, *nodeKey)
	return nil
}

func (s *SimpleKVSStore) Set(nodeKey *config.KVSNodeKey, key string, value []byte) error {
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}
	if _, exists := s.stores[*nodeKey]; !exists {
		return fmt.Errorf("node does not exist: %s", nodeKey.ClusterID.String())
	}
	s.stores[*nodeKey][key] = value
	return nil
}

func (s *SimpleKVSStore) Get(nodeKey *config.KVSNodeKey, key string) ([]byte, error) {
	if _, exists := s.stores[*nodeKey]; !exists {
		return nil, fmt.Errorf("node does not exist: %s", nodeKey.ClusterID.String())
	}
	value, exists := s.stores[*nodeKey][key]
	if !exists {
		return nil, nil // or return an error if preferred
	}
	return value, nil
}

func (s *SimpleKVSStore) Patch(nodeKey *config.KVSNodeKey, key string, value []byte) error {
	panic("Patch not implemented in SimpleKVSStore")
}

func (s *SimpleKVSStore) Delete(nodeKey *config.KVSNodeKey, key string) error {
	if _, exists := s.stores[*nodeKey]; !exists {
		return fmt.Errorf("node does not exist: %s", nodeKey.ClusterID.String())
	}
	if _, exists := s.stores[*nodeKey][key]; !exists {
		return fmt.Errorf("key does not exist: %s", key)
	}
	delete(s.stores[*nodeKey], key)
	return nil
}
