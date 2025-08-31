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
	"sync"
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/shared"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type raftNodeManagerHelper struct {
	t                      *testing.T
	raftNodeErrorF         func(nodeKey *config.KvsNodeKey, err error)
	raftNodeSendMessageF   func(dstNodeID *shared.NodeID, data *proto.RaftMessage)
	raftNodeApplyProposalF func(nodeKey *config.KvsNodeKey, proposal *proto.RaftProposalManagement)
	raftNodeAppendNodeF    func(nodeKey *config.KvsNodeKey, sequence uint64, nodeID *shared.NodeID)
	raftNodeRemoveNodeF    func(nodeKey *config.KvsNodeKey, sequence uint64)
}

func (h *raftNodeManagerHelper) raftNodeError(nodeKey *config.KvsNodeKey, err error) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeErrorF)
	h.raftNodeErrorF(nodeKey, err)
}

func (h *raftNodeManagerHelper) raftNodeSendMessage(dstNodeID *shared.NodeID, data *proto.RaftMessage) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeSendMessageF)
	h.raftNodeSendMessageF(dstNodeID, data)
}

func (h *raftNodeManagerHelper) raftNodeApplyProposal(nodeKey *config.KvsNodeKey, proposal *proto.RaftProposalManagement) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeApplyProposalF)
	h.raftNodeApplyProposalF(nodeKey, proposal)
}

func (h *raftNodeManagerHelper) raftNodeAppendNode(nodeKey *config.KvsNodeKey, sequence uint64, nodeID *shared.NodeID) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeAppendNodeF)
	h.raftNodeAppendNodeF(nodeKey, sequence, nodeID)
}

func (h *raftNodeManagerHelper) raftNodeRemoveNode(nodeKey *config.KvsNodeKey, sequence uint64) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeRemoveNodeF)
	h.raftNodeRemoveNodeF(nodeKey, sequence)
}

type raftNodeStoreHelper struct {
	t                      *testing.T
	raftNodeApplyProposalF func(command *proto.RaftProposalStore)
	raftNodeGetSnapshotF   func() ([]byte, error)
	raftNodeApplySnapshotF func(snapshot []byte) error
}

func (h *raftNodeStoreHelper) raftNodeApplyProposal(command *proto.RaftProposalStore) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeApplyProposalF)
	h.raftNodeApplyProposalF(command)
}

func (h *raftNodeStoreHelper) raftNodeGetSnapshot() ([]byte, error) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeGetSnapshotF)
	return h.raftNodeGetSnapshotF()
}

func (h *raftNodeStoreHelper) raftNodeApplySnapshot(snapshot []byte) error {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeApplySnapshotF)
	return h.raftNodeApplySnapshotF(snapshot)
}

type proposalReq struct {
	management *proto.RaftProposalManagement
	store      *proto.RaftProposalStore
}

func (p *proposalReq) String() string {
	if p.management != nil {
		return "<management>" + p.management.String()
	} else if p.store != nil {
		return "<store>" + p.store.String()
	}
	return "<nil>"
}

func TestRaftNode(t *testing.T) {
	clusterID := testUtil.UniqueUUIDs(1)[0]
	sequences := testUtil.UniqueNumbers[uint64](4)
	nodeIDs := testUtil.UniqueNodeIDs(len(sequences))
	sequenceMap := map[uint64]int{}
	for i, sequence := range sequences {
		sequenceMap[sequence] = i
	}
	raftNodes := make([]*raftNode, len(sequences))
	mtx := sync.Mutex{}
	type appendReq struct {
		sequence uint64
		nodeID   *shared.NodeID
	}
	type removeReq struct {
		sequence uint64
	}
	receivedAppends := make([][]*appendReq, len(sequences))
	receivedRemoves := make([][]*removeReq, len(sequences))
	node0Stopped := false
	receivedProposals := make([][]*proposalReq, len(sequences))

	manager := &raftNodeManagerHelper{
		t: t,
		raftNodeErrorF: func(nodeKey *config.KvsNodeKey, err error) {
			require.FailNow(t, "unexpected raftNodeError")
		},
		raftNodeSendMessageF: func(dstNodeID *shared.NodeID, data *proto.RaftMessage) {
			assert.Equal(t, data.ClusterId, MustMarshalUUID(clusterID))
			i := sequenceMap[data.Sequence]
			assert.Equal(t, *dstNodeID, *nodeIDs[i])
			mtx.Lock()
			if node0Stopped && i == 0 {
				mtx.Unlock()
				return
			}
			mtx.Unlock()
			err := raftNodes[i].processMessage(data)
			assert.NoError(t, err)
		},
		raftNodeAppendNodeF: func(nodeKey *config.KvsNodeKey, sequence uint64, nodeID *shared.NodeID) {
			mtx.Lock()
			defer mtx.Unlock()
			i := sequenceMap[nodeKey.Sequence]
			receivedAppends[i] = append(receivedAppends[i], &appendReq{
				sequence: sequence,
				nodeID:   nodeID,
			})
		},
		raftNodeRemoveNodeF: func(nodeKey *config.KvsNodeKey, sequence uint64) {
			mtx.Lock()
			defer mtx.Unlock()
			i := sequenceMap[nodeKey.Sequence]
			receivedRemoves[i] = append(receivedRemoves[i], &removeReq{
				sequence: sequence,
			})
		},
		raftNodeApplyProposalF: func(nodeKey *config.KvsNodeKey, proposal *proto.RaftProposalManagement) {
			mtx.Lock()
			defer mtx.Unlock()
			i := sequenceMap[nodeKey.Sequence]
			receivedProposals[i] = append(receivedProposals[i], &proposalReq{
				management: proposal,
			})
		},
	}

	t.Run("create raft cluster with 3 nodes", func(tt *testing.T) {
		// create raft node 0, 1, 2
		for i := 0; i < 3; i++ {
			i := i
			store := &raftNodeStoreHelper{
				t: t,
				raftNodeApplyProposalF: func(command *proto.RaftProposalStore) {
					mtx.Lock()
					defer mtx.Unlock()
					receivedProposals[i] = append(receivedProposals[i], &proposalReq{
						store: command,
					})
				},
			}

			raftNodes[i] = newRaftNode(&raftNodeConfig{
				logger:  testUtil.Logger(t),
				manager: manager,
				store:   store,
				nodeKey: &config.KvsNodeKey{
					ClusterID: clusterID,
					Sequence:  sequences[i],
				},
				join: false,
				member: map[uint64]*shared.NodeID{
					sequences[0]: nodeIDs[0],
					sequences[1]: nodeIDs[1],
					sequences[2]: nodeIDs[2],
				},
			})
			raftNodes[i].start(t.Context())
		}

		assert.Eventually(tt, func() bool {
			mtx.Lock()
			defer mtx.Unlock()
			for i := 0; i < 3; i++ {
				assert.Len(tt, receivedRemoves[i], 0)
				if len(receivedAppends[i]) != 3 {
					return false
				}
			}
			return true
		}, 3*time.Second, 100*time.Millisecond)

		// check append requests
		for _, appends := range receivedAppends {
			appendSeq := map[uint64]bool{}
			for _, req := range appends {
				appendSeq[req.sequence] = true
				assert.Equal(tt, req.nodeID, nodeIDs[sequenceMap[req.sequence]])
			}
			for _, seq := range sequences {
				assert.Contains(tt, sequences, seq)
			}
		}
	})

	t.Run("add a new node", func(tt *testing.T) {
		mtx.Lock()
		for i := 0; i < 3; i++ {
			receivedAppends[i] = nil
		}
		mtx.Unlock()

		err := raftNodes[0].appendNode(sequences[3], nodeIDs[3])
		require.NoError(tt, err)

		raftNodes[3] = newRaftNode(&raftNodeConfig{
			logger:  testUtil.Logger(t),
			manager: manager,
			store: &raftNodeStoreHelper{
				t: t,
				raftNodeGetSnapshotF: func() ([]byte, error) {
					return []byte("snapshot"), nil
				},
				raftNodeApplySnapshotF: func(snapshot []byte) error {
					assert.Equal(t, []byte("snapshot"), snapshot)
					return nil
				},
				raftNodeApplyProposalF: func(command *proto.RaftProposalStore) {
					mtx.Lock()
					defer mtx.Unlock()
					receivedProposals[3] = append(receivedProposals[3], &proposalReq{
						store: command,
					})
				},
			},
			nodeKey: &config.KvsNodeKey{
				ClusterID: clusterID,
				Sequence:  sequences[3],
			},
			join: true,
			member: map[uint64]*shared.NodeID{
				sequences[0]: nodeIDs[0],
				sequences[1]: nodeIDs[1],
				sequences[2]: nodeIDs[2],
				sequences[3]: nodeIDs[3],
			},
		})
		raftNodes[3].start(t.Context())

		assert.Eventually(tt, func() bool {
			mtx.Lock()
			defer mtx.Unlock()
			for i := 0; i < 3; i++ {
				if len(receivedAppends[i]) != 1 {
					return false
				}
				assert.Equal(tt, sequences[3], receivedAppends[i][0].sequence)
			}
			return len(receivedAppends[3]) == 4
		}, 3*time.Second, 100*time.Millisecond)
	})

	t.Run("remove a node", func(tt *testing.T) {
		mtx.Lock()
		for i := 0; i < 4; i++ {
			receivedAppends[i] = nil
		}
		node0Stopped = true
		mtx.Unlock()

		raftNodes[0].stop()
		removed := false
		assert.Eventually(tt, func() bool {
			if !removed {
				// remove node until the sender node receives the remove request: is it necessary?
				raftNodes[1].removeNode(sequences[0])
			}

			mtx.Lock()
			defer mtx.Unlock()
			for i := 1; i < 4; i++ {
				if i == 1 && len(receivedRemoves[i]) == 1 {
					removed = true
				}
				if len(receivedRemoves[i]) != 1 {
					return false
				}
				assert.Equal(tt, sequences[0], receivedRemoves[i][0].sequence)
			}
			return true
		}, 3*time.Second, 100*time.Millisecond)
	})

	t.Run("send proposal", func(t *testing.T) {
		proposals := []struct {
			node int
			prop *proto.RaftProposal
		}{
			{
				node: 1,
				prop: &proto.RaftProposal{
					Content: &proto.RaftProposal_Store{
						Store: &proto.RaftProposalStore{
							Command:    proto.RaftProposalStore_COMMAND_DELETE,
							ProposalId: 1,
							Key:        "key",
							Value:      []byte("value"),
						},
					},
				},
			},
			{
				node: 2,
				prop: &proto.RaftProposal{
					Content: &proto.RaftProposal_Store{
						Store: &proto.RaftProposalStore{
							Command:    proto.RaftProposalStore_COMMAND_DELETE,
							ProposalId: 2,
							Key:        "key",
							Value:      []byte("value"),
						},
					},
				},
			},
			{
				node: 1,
				prop: &proto.RaftProposal{
					Content: &proto.RaftProposal_Management{
						Management: &proto.RaftProposalManagement{},
					},
				},
			},
		}

		for _, p := range proposals {
			raftNodes[p.node].propose(p.prop)
		}

		assert.Eventually(t, func() bool {
			mtx.Lock()
			defer mtx.Unlock()
			for i := 1; i < 4; i++ {
				if len(receivedProposals[i]) != 3 {
					return false
				}
			}
			return true
		}, 3*time.Second, 100*time.Millisecond)

		assert.Len(t, receivedProposals[0], 0, "node 0 should not receive proposals")

		proposalStrs := []string{}
		for _, p := range proposals {
			proposalStrs = append(proposalStrs,
				(&proposalReq{
					management: p.prop.GetManagement(),
					store:      p.prop.GetStore(),
				}).String())
		}
		for i := 1; i < 4; i++ {
			for j := 0; j < 3; j++ {
				rpStr := receivedProposals[i][j].String()
				assert.Equal(t, receivedProposals[1][j].String(), rpStr, "all nodes should receive same proposals")
				assert.Contains(t, proposalStrs, rpStr, "received proposal should be in sent proposals")
			}
		}
	})
}
