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
package consensus

import (
	"sync"
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type consensusHandlerHelper struct {
	t                       *testing.T
	consensusErrorF         func(err error)
	consensusAppendNodeF    func(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID)
	consensusRemoveNodeF    func(sectorNo kvsTypes.SectorNo)
	consensusApplyProposalF func(proposal *proto.ConsensusProposal)
	consensusGetSnapshotF   func() ([]byte, error)
	consensusApplySnapshotF func(snapshot []byte) error
}

var _ Handler = &consensusHandlerHelper{}

func (h *consensusHandlerHelper) ConsensusError(err error) {
	h.t.Helper()
	require.NotNil(h.t, h.consensusErrorF)
	h.consensusErrorF(err)
}

func (h *consensusHandlerHelper) ConsensusApplyProposal(proposal *proto.ConsensusProposal) {
	h.t.Helper()
	require.NotNil(h.t, h.consensusApplyProposalF)
	h.consensusApplyProposalF(proposal)
}

func (h *consensusHandlerHelper) ConsensusAppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	h.t.Helper()
	require.NotNil(h.t, h.consensusAppendNodeF)
	h.consensusAppendNodeF(sectorNo, nodeID)
}

func (h *consensusHandlerHelper) ConsensusRemoveNode(sectorNo kvsTypes.SectorNo) {
	h.t.Helper()
	require.NotNil(h.t, h.consensusRemoveNodeF)
	h.consensusRemoveNodeF(sectorNo)
}

func (h *consensusHandlerHelper) ConsensusGetSnapshot() ([]byte, error) {
	h.t.Helper()
	require.NotNil(h.t, h.consensusGetSnapshotF)
	return h.consensusGetSnapshotF()
}

func (h *consensusHandlerHelper) ConsensusApplySnapshot(snapshot []byte) error {
	h.t.Helper()
	require.NotNil(h.t, h.consensusApplySnapshotF)
	return h.consensusApplySnapshotF(snapshot)
}

type consensusOutboundHelper struct {
	t                     *testing.T
	sendConsensusMessageF func(dstNodeID *types.NodeID, message *proto.ConsensusMessage)
}

var _ OutboundPort = &consensusOutboundHelper{}

func (h *consensusOutboundHelper) sendConsensusMessage(dstNodeID *types.NodeID, message *proto.ConsensusMessage) {
	h.t.Helper()
	require.NotNil(h.t, h.sendConsensusMessageF)
	h.sendConsensusMessageF(dstNodeID, message)
}

func TestConsensus(t *testing.T) {
	sectorCount := 4
	sectorID := testUtil.UniqueSectorIDs(1)[0]
	sectorNos := testUtil.UniqueNumbersU[kvsTypes.SectorNo](sectorCount)
	nodeIDs := testUtil.UniqueNodeIDs(sectorCount)
	sectorNoMap := map[kvsTypes.SectorNo]int{}
	for i, sectorNo := range sectorNos {
		sectorNoMap[sectorNo] = i
	}
	consensuses := make([]*Consensus, sectorCount)
	mtx := sync.Mutex{}
	type appendReq struct {
		sectorNo kvsTypes.SectorNo
		nodeID   *types.NodeID
	}
	type removeReq struct {
		sectorNo kvsTypes.SectorNo
	}
	receivedAppends := make([][]*appendReq, sectorCount)
	receivedRemoves := make([][]*removeReq, sectorCount)
	node0Stopped := false
	receivedProposals := make([][]*proto.ConsensusProposal, sectorCount)
	handlers := make([]*consensusHandlerHelper, sectorCount)
	outbound := make([]*consensusOutboundHelper, sectorCount)
	for i := 0; i < sectorCount; i++ {
		handlers[i] = &consensusHandlerHelper{
			t: t,

			consensusErrorF: func(err error) {
				require.FailNow(t, "unexpected consensusError")
			},

			consensusAppendNodeF: func(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
				mtx.Lock()
				defer mtx.Unlock()
				receivedAppends[i] = append(receivedAppends[i], &appendReq{
					sectorNo: sectorNo,
					nodeID:   nodeID,
				})
			},

			consensusRemoveNodeF: func(sectorNo kvsTypes.SectorNo) {
				mtx.Lock()
				defer mtx.Unlock()
				receivedRemoves[i] = append(receivedRemoves[i], &removeReq{
					sectorNo: sectorNo,
				})
			},

			consensusApplyProposalF: func(proposal *proto.ConsensusProposal) {
				mtx.Lock()
				defer mtx.Unlock()
				receivedProposals[i] = append(receivedProposals[i], proposal)
			},
		}

		outbound[i] = &consensusOutboundHelper{
			t: t,
			sendConsensusMessageF: func(dstNodeID *types.NodeID, message *proto.ConsensusMessage) {
				assert.Equal(t, message.SectorId, kvsTypes.MustMarshalSectorID(sectorID))
				i := sectorNoMap[kvsTypes.SectorNo(message.SectorNo)]
				assert.Equal(t, *dstNodeID, *nodeIDs[i])
				mtx.Lock()
				if node0Stopped && i == 0 {
					mtx.Unlock()
					return
				}
				mtx.Unlock()
				err := consensuses[i].ProcessMessage(message)
				assert.NoError(t, err)
			},
		}
	}

	initialMemberCount := sectorCount - 1
	t.Run("create the initial raft cluster with 3 nodes", func(tt *testing.T) {
		// create raft node 0, 1, 2
		initialMember := map[kvsTypes.SectorNo]*types.NodeID{}
		for i := 0; i < initialMemberCount; i++ {
			initialMember[sectorNos[i]] = nodeIDs[i]
		}

		for i := 0; i < initialMemberCount; i++ {
			consensuses[i] = NewConsensus(&Config{
				Logger:   testUtil.Logger(t),
				Handler:  handlers[i],
				Outbound: outbound[i],
				SectorKey: &kvsTypes.SectorKey{
					SectorID: sectorID,
					SectorNo: sectorNos[i],
				},
				Join:    false,
				Members: initialMember,
			})
			consensuses[i].Start(t.Context())
		}

		assert.EventuallyWithT(tt, func(c *assert.CollectT) {
			mtx.Lock()
			defer mtx.Unlock()
			for i := 0; i < initialMemberCount; i++ {
				assert.Len(tt, receivedRemoves[i], 0)
				assert.Len(c, receivedAppends[i], initialMemberCount)
			}
		}, 3*time.Second, 100*time.Millisecond)

		// check append requests
		for i := 0; i < initialMemberCount; i++ {
			appends := receivedAppends[i]
			appendSec := make(map[kvsTypes.SectorNo]struct{})
			for _, req := range appends {
				appendSec[req.sectorNo] = struct{}{}
				assert.Equal(tt, req.nodeID, nodeIDs[sectorNoMap[req.sectorNo]])
			}

			for j := 0; j < initialMemberCount; j++ {
				assert.Contains(tt, appendSec, sectorNos[j])
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

		consensuses[0].AppendNode(
			sectorNos[additionalNodeIdx],
			nodeIDs[additionalNodeIdx],
		)

		handlers[additionalNodeIdx].consensusGetSnapshotF = func() ([]byte, error) {
			return []byte("snapshot"), nil
		}

		handlers[additionalNodeIdx].consensusApplySnapshotF = func(snapshot []byte) error {
			assert.Equal(t, []byte("snapshot"), snapshot)
			return nil
		}

		consensuses[additionalNodeIdx] = NewConsensus(&Config{
			Logger:   testUtil.Logger(t),
			Handler:  handlers[additionalNodeIdx],
			Outbound: outbound[additionalNodeIdx],
			SectorKey: &kvsTypes.SectorKey{
				SectorID: sectorID,
				SectorNo: sectorNos[additionalNodeIdx],
			},
			Join: true,
			Members: map[kvsTypes.SectorNo]*types.NodeID{
				sectorNos[0]: nodeIDs[0],
				sectorNos[1]: nodeIDs[1],
				sectorNos[2]: nodeIDs[2],
				sectorNos[3]: nodeIDs[3],
			},
		})
		consensuses[additionalNodeIdx].Start(t.Context())

		assert.EventuallyWithT(tt, func(c *assert.CollectT) {
			mtx.Lock()
			defer mtx.Unlock()
			for i := 0; i < initialMemberCount; i++ {
				require.Len(c, receivedAppends[i], 1)
				assert.Equal(c, sectorNos[additionalNodeIdx], receivedAppends[i][0].sectorNo)
			}
			assert.Len(c, receivedAppends[additionalNodeIdx], len(nodeIDs))
		}, 3*time.Second, 100*time.Millisecond)
	})

	t.Run("remove a node", func(tt *testing.T) {
		mtx.Lock()
		for i := 0; i < sectorCount; i++ {
			receivedAppends[i] = nil
		}
		node0Stopped = true
		mtx.Unlock()

		consensuses[0].Stop()
		removed := false
		assert.EventuallyWithT(tt, func(c *assert.CollectT) {
			mtx.Lock()
			defer mtx.Unlock()

			if !removed {
				// remove node until the sender node receives the remove request
				consensuses[1].RemoveNode(sectorNos[0])

			} else {
				for i := 1; i < 4; i++ {
					if i == 1 && len(receivedRemoves[i]) > 0 {
						removed = true
					}
					require.Greater(c, len(receivedRemoves[i]), 0)
					// all remove requests should be for the same sector
					for _, r := range receivedRemoves[i] {
						assert.Equal(c, sectorNos[0], r.sectorNo)
					}
				}
			}
		}, 3*time.Second, 100*time.Millisecond)
	})

	t.Run("wait for leader election", func(tt *testing.T) {
		require.Eventually(tt, func() bool {
			consensuses[1].Propose(&proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_Operation{
					Operation: &proto.Operation{
						Command:     proto.Operation_COMMAND_DELETE,
						OperationId: 1,
						Key:         "key",
						Value:       []byte("value"),
					},
				},
			})

			mtx.Lock()
			defer mtx.Unlock()

			for i := 1; i < sectorCount; i++ {
				if len(receivedProposals[i]) > 0 {
					return true
				}
			}
			return false
		}, 3*time.Second, 100*time.Millisecond)
	})

	t.Run("send proposal", func(tt *testing.T) {
		// wait for all proposals to be received by all nodes
		time.Sleep(1 * time.Second)
		// clear received proposals
		mtx.Lock()
		receivedProposals = make([][]*proto.ConsensusProposal, sectorCount)
		mtx.Unlock()

		proposals := []struct {
			node int
			prop *proto.ConsensusProposal
		}{
			{
				node: 1,
				prop: &proto.ConsensusProposal{
					Content: &proto.ConsensusProposal_Operation{
						Operation: &proto.Operation{
							Command:     proto.Operation_COMMAND_DELETE,
							OperationId: 1,
							Key:         "key",
							Value:       []byte("value"),
						},
					},
				},
			},
			{
				node: 2,
				prop: &proto.ConsensusProposal{
					Content: &proto.ConsensusProposal_Operation{
						Operation: &proto.Operation{
							Command:     proto.Operation_COMMAND_DELETE,
							OperationId: 2,
							Key:         "key",
							Value:       []byte("value"),
						},
					},
				},
			},
			{
				node: 1,
				prop: &proto.ConsensusProposal{
					Content: &proto.ConsensusProposal_Activate{
						Activate: &proto.Activate{},
					},
				},
			},
		}

		for _, p := range proposals {
			consensuses[p.node].Propose(p.prop)
			// wait for the proposal to be sent to other nodes
			time.Sleep(100 * time.Millisecond)
		}

		assert.EventuallyWithT(tt, func(c *assert.CollectT) {
			mtx.Lock()
			defer mtx.Unlock()
			for i := 1; i < sectorCount; i++ {
				assert.Len(c, receivedProposals[i], len(proposals))
			}
		}, 3*time.Second, 100*time.Millisecond)

		assert.Len(t, receivedProposals[0], 0, "node 0 should not receive proposals")

		proposalStrings := []string{}
		for _, p := range proposals {
			proposalStrings = append(proposalStrings, p.prop.String())
		}
		for i := 1; i < sectorCount; i++ {
			for j, receivedProposal := range receivedProposals[i] {
				rpStr := receivedProposal.String()
				assert.Equal(t, receivedProposals[1][j].String(), rpStr, "all nodes should receive same proposals")
				assert.Contains(t, proposalStrings, rpStr, "received proposal should be in sent proposals")
			}
		}
	})
}
