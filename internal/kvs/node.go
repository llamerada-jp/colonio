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

import proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"

type node struct {
	raft  *raftNode
	store *store
}

func (n *node) raftNodeApplyProposal(proposal *proto.RaftProposalStore) {
	n.store.applyProposal(proposal)
}

func (n *node) raftNodeGetSnapshot() ([]byte, error) {
	return n.store.exportSnapshot()
}

func (n *node) raftNodeApplySnapshot(snapshot []byte) error {
	return n.store.importSnapshot(snapshot)
}

func (n *node) storePropose(command *proto.RaftProposalStore) {
	n.raft.propose(&proto.RaftProposal{
		Content: &proto.RaftProposal_Store{
			Store: command,
		},
	})
}
