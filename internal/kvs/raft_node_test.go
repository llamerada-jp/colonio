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

type raftNodeHelper struct {
	t *testing.T
	// raftNodeManager interface
	raftNodeErrorF         func(sectorKey *config.KvsSectorKey, err error)
	raftNodeApplyProposalF func(sectorKey *config.KvsSectorKey, proposal *proto.RaftProposalManagement)
	raftNodeAppendNodeF    func(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo, nodeID *shared.NodeID)
	raftNodeRemoveNodeF    func(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo)
	// RaftInfrastructure interface
	sendRaftMessageF func(param *raftMessageParam)
}

var _ raftNodeManager = &raftNodeHelper{}
var _ RaftInfrastructure = &raftNodeHelper{}

func (h *raftNodeHelper) raftNodeError(sectorKey *config.KvsSectorKey, err error) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeErrorF)
	h.raftNodeErrorF(sectorKey, err)
}

func (h *raftNodeHelper) raftNodeApplyProposal(sectorKey *config.KvsSectorKey, proposal *proto.RaftProposalManagement) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeApplyProposalF)
	h.raftNodeApplyProposalF(sectorKey, proposal)
}

func (h *raftNodeHelper) raftNodeAppendNode(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo, nodeID *shared.NodeID) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeAppendNodeF)
	h.raftNodeAppendNodeF(sectorKey, sectorNo, nodeID)
}

func (h *raftNodeHelper) raftNodeRemoveNode(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo) {
	h.t.Helper()
	require.NotNil(h.t, h.raftNodeRemoveNodeF)
	h.raftNodeRemoveNodeF(sectorKey, sectorNo)
}

func (h *raftNodeHelper) sendRaftMessage(param *raftMessageParam) {
	h.t.Helper()
	require.NotNil(h.t, h.sendRaftMessageF)
	h.sendRaftMessageF(param)
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
	sectorID := testUtil.UniqueSectorIDs(1)[0]
	sectorNos := testUtil.UniqueNumbersU[config.SectorNo](4)
	nodeIDs := testUtil.UniqueNodeIDs(len(sectorNos))
	sectorNoMap := map[config.SectorNo]int{}
	for i, sectorNo := range sectorNos {
		sectorNoMap[sectorNo] = i
	}
	raftNodes := make([]*raftNode, len(sectorNos))
	mtx := sync.Mutex{}
	type appendReq struct {
		sectorNo config.SectorNo
		nodeID   *shared.NodeID
	}
	type removeReq struct {
		sectorNo config.SectorNo
	}
	receivedAppends := make([][]*appendReq, len(sectorNos))
	receivedRemoves := make([][]*removeReq, len(sectorNos))
	node0Stopped := false
	receivedProposals := make([][]*proposalReq, len(sectorNos))

	helper := &raftNodeHelper{
		t: t,
		raftNodeErrorF: func(sectorKey *config.KvsSectorKey, err error) {
			require.FailNow(t, "unexpected raftNodeError")
		},
		raftNodeAppendNodeF: func(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo, nodeID *shared.NodeID) {
			mtx.Lock()
			defer mtx.Unlock()
			i := sectorNoMap[sectorKey.SectorNo]
			receivedAppends[i] = append(receivedAppends[i], &appendReq{
				sectorNo: sectorNo,
				nodeID:   nodeID,
			})
		},
		raftNodeRemoveNodeF: func(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo) {
			mtx.Lock()
			defer mtx.Unlock()
			i := sectorNoMap[sectorKey.SectorNo]
			receivedRemoves[i] = append(receivedRemoves[i], &removeReq{
				sectorNo: sectorNo,
			})
		},
		raftNodeApplyProposalF: func(sectorKey *config.KvsSectorKey, proposal *proto.RaftProposalManagement) {
			mtx.Lock()
			defer mtx.Unlock()
			i := sectorNoMap[sectorKey.SectorNo]
			receivedProposals[i] = append(receivedProposals[i], &proposalReq{
				management: proposal,
			})
		},
		sendRaftMessageF: func(param *raftMessageParam) {
			assert.Equal(t, param.message.SectorId, MustMarshalSectorID(sectorID))
			i := sectorNoMap[config.SectorNo(param.message.SectorNo)]
			assert.Equal(t, *param.dstNodeID, *nodeIDs[i])
			mtx.Lock()
			if node0Stopped && i == 0 {
				mtx.Unlock()
				return
			}
			mtx.Unlock()
			err := raftNodes[i].processMessage(param.message)
			assert.NoError(t, err)
		},
	}

	initialMemberCount := 3
	t.Run("create the initial raft cluster with 3 nodes", func(tt *testing.T) {
		// create raft node 0, 1, 2
		initialMember := map[config.SectorNo]*shared.NodeID{}
		for i := 0; i < initialMemberCount; i++ {
			initialMember[sectorNos[i]] = nodeIDs[i]
		}

		for i := 0; i < initialMemberCount; i++ {
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
				logger:         testUtil.Logger(t),
				manager:        helper,
				infrastructure: helper,
				store:          store,
				sectorKey: &config.KvsSectorKey{
					SectorID: sectorID,
					SectorNo: sectorNos[i],
				},
				join:   false,
				member: initialMember,
			})
			raftNodes[i].start(t.Context())
		}

		assert.Eventually(tt, func() bool {
			mtx.Lock()
			defer mtx.Unlock()
			for i := 0; i < initialMemberCount; i++ {
				assert.Len(tt, receivedRemoves[i], 0)
				if len(receivedAppends[i]) != initialMemberCount-1 {
					return false
				}
			}
			return true
		}, 3*time.Second, 100*time.Millisecond)

		// check append requests
		for i := 0; i < initialMemberCount; i++ {
			appends := receivedAppends[i]
			appendSec := make(map[config.SectorNo]struct{})
			for _, req := range appends {
				appendSec[req.sectorNo] = struct{}{}
				assert.Equal(tt, req.nodeID, nodeIDs[sectorNoMap[req.sectorNo]])
			}

			for j := 0; j < initialMemberCount; j++ {
				seqNo := sectorNos[j]
				if j == i {
					assert.NotContains(tt, appendSec, seqNo)
				} else {
					assert.Contains(tt, appendSec, seqNo)
				}
			}
		}
	})

	additionalNodeIdx := initialMemberCount
	t.Run("add a new node", func(tt *testing.T) {
		mtx.Lock()
		for i := 0; i < initialMemberCount; i++ {
			receivedAppends[i] = nil
		}
		mtx.Unlock()

		err := raftNodes[0].appendNode(
			sectorNos[additionalNodeIdx],
			nodeIDs[additionalNodeIdx],
		)
		require.NoError(tt, err)

		raftNodes[additionalNodeIdx] = newRaftNode(&raftNodeConfig{
			logger:         testUtil.Logger(t),
			manager:        helper,
			infrastructure: helper,
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
					receivedProposals[additionalNodeIdx] =
						append(receivedProposals[additionalNodeIdx], &proposalReq{
							store: command,
						})
				},
			},
			sectorKey: &config.KvsSectorKey{
				SectorID: sectorID,
				SectorNo: sectorNos[additionalNodeIdx],
			},
			join: true,
			member: map[config.SectorNo]*shared.NodeID{
				sectorNos[0]: nodeIDs[0],
				sectorNos[1]: nodeIDs[1],
				sectorNos[2]: nodeIDs[2],
				sectorNos[3]: nodeIDs[3],
			},
		})
		raftNodes[additionalNodeIdx].start(t.Context())

		assert.Eventually(tt, func() bool {
			mtx.Lock()
			defer mtx.Unlock()
			for i := 0; i < initialMemberCount; i++ {
				if len(receivedAppends[i]) != 1 {
					return false
				}
				assert.Equal(tt, sectorNos[additionalNodeIdx], receivedAppends[i][0].sectorNo)
			}
			return len(receivedAppends[additionalNodeIdx]) == initialMemberCount
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
				raftNodes[1].removeNode(sectorNos[0])
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
				assert.Equal(tt, sectorNos[0], receivedRemoves[i][0].sectorNo)
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
