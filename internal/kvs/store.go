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
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type storeHandler interface {
	storePropose(command *proto.RaftProposalStore)
}

type storeConfig struct {
	nodeKey *config.KVSNodeKey
	handler storeHandler
	head    *shared.NodeID
}

type lock struct {
	coordinator uint64
	address     *shared.NodeID
}

type store struct {
	nodeKey  config.KVSNodeKey
	handler  storeHandler
	head     *shared.NodeID
	tail     *shared.NodeID
	readonly *lock
	blocked  *lock
	records  map[shared.NodeID]map[string][]byte
}

func newStore(config *storeConfig) *store {
	return &store{
		nodeKey: *config.nodeKey,
		handler: config.handler,
		head:    config.head,
		records: make(map[shared.NodeID]map[string][]byte),
	}
}

func (s *store) firstActivate() {
	panic("firstActivate not implemented")
}

func (s *store) applyProposal(command *proto.RaftProposalStore) {
	panic("apply not implemented")
}

func (s *store) exportRecords(head, tail *shared.NodeID) []byte {
	panic("export not implemented")
}

func (s *store) importRecords(data []byte) error {
	panic("import not implemented")
}

func (s *store) exportSnapshot() ([]byte, error) {
	panic("exportSnapshot not implemented")
}

func (s *store) importSnapshot(data []byte) error {
	panic("importSnapshot not implemented")
}
