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
	"io"
	"log"
	"sync"
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3"
)

// joinTestCluster wires multiple in-process Consensus instances. Packets to a
// dead member, or to one that has not started yet, are silently dropped.
type joinTestCluster struct {
	t        *testing.T
	sectorID kvsTypes.SectorID
	nodeIDs  map[kvsTypes.SectorNo]*types.NodeID

	mtx         sync.Mutex
	consensuses map[kvsTypes.SectorNo]*Consensus
	dead        map[kvsTypes.SectorNo]bool
	applied     map[kvsTypes.SectorNo][]*proto.ConsensusProposal
	snapshots   map[kvsTypes.SectorNo]int
}

func newJoinTestCluster(t *testing.T) *joinTestCluster {
	return &joinTestCluster{
		t:           t,
		sectorID:    testUtil.UniqueSectorIDs(1)[0],
		nodeIDs:     map[kvsTypes.SectorNo]*types.NodeID{},
		consensuses: map[kvsTypes.SectorNo]*Consensus{},
		dead:        map[kvsTypes.SectorNo]bool{},
		applied:     map[kvsTypes.SectorNo][]*proto.ConsensusProposal{},
		snapshots:   map[kvsTypes.SectorNo]int{},
	}
}

// startConsensus creates and starts the member sectorNo. It is registered for
// message delivery only after Start, since ProcessMessage needs the context
// that Start sets.
func (c *joinTestCluster) startConsensus(sectorNo kvsTypes.SectorNo, join bool, members map[kvsTypes.SectorNo]*types.NodeID) *Consensus {
	handler := &consensusHandlerHelper{
		t: c.t,
		consensusErrorF: func(err error) {
			// proposals may time out while the quorum is degraded; ignore
		},
		consensusAppendNodeF: func(sn kvsTypes.SectorNo, nodeID *types.NodeID) {},
		consensusRemoveNodeF: func(sn kvsTypes.SectorNo) {},
		consensusApplyProposalF: func(proposal *proto.ConsensusProposal) error {
			c.mtx.Lock()
			defer c.mtx.Unlock()
			c.applied[sectorNo] = append(c.applied[sectorNo], proposal)
			return nil
		},
		consensusGetSnapshotF: func() ([]byte, error) {
			return []byte("snapshot"), nil
		},
		consensusApplySnapshotF: func(snapshot []byte) error {
			c.mtx.Lock()
			defer c.mtx.Unlock()
			c.snapshots[sectorNo]++
			return nil
		},
	}

	outbound := &consensusOutboundHelper{
		t: c.t,
		sendConsensusMessageF: func(dstNodeID *types.NodeID, message *proto.ConsensusMessage) {
			dstNo := kvsTypes.SectorNo(message.SectorNo)
			c.mtx.Lock()
			dst := c.consensuses[dstNo]
			isDead := c.dead[dstNo]
			c.mtx.Unlock()
			if dst == nil || isDead {
				return
			}
			_ = dst.ProcessMessage(message)
		},
	}

	cons := NewConsensus(&Config{
		Logger:     testUtil.Logger(c.t),
		RaftLogger: &raft.DefaultLogger{Logger: log.New(io.Discard, "", 0)},
		Handler:    handler,
		Outbound:   outbound,
		SectorKey: &kvsTypes.SectorKey{
			SectorID: c.sectorID,
			SectorNo: sectorNo,
		},
		Join:    join,
		Members: members,
	})
	cons.Start(c.t.Context())
	c.t.Cleanup(cons.Stop)

	c.mtx.Lock()
	c.consensuses[sectorNo] = cons
	c.mtx.Unlock()

	return cons
}

func (c *joinTestCluster) appliedCount(sectorNo kvsTypes.SectorNo) int {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return len(c.applied[sectorNo])
}

func activateProposal(nodeID *types.NodeID) *proto.ConsensusProposal {
	return &proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Activate{
			Activate: &proto.Activate{
				Tail: nodeID.Proto(),
			},
		},
	}
}

// TestConsensus_joinCatchesUpWithDeadMember covers a member joining with an
// empty log (Join=true) a group that has a dead member: the joiner must catch
// up and apply the proposals committed before it joined. In the simulator, a
// replica in this situation looked like it stayed empty for minutes.
func TestConsensus_joinCatchesUpWithDeadMember(t *testing.T) {
	cluster := newJoinTestCluster(t)

	no1 := kvsTypes.SectorNo(1)
	no2 := kvsTypes.SectorNo(2)
	no3 := kvsTypes.SectorNo(3) // dead from the start
	no4 := kvsTypes.SectorNo(4) // joins later

	ids := testUtil.UniqueNodeIDs(4)
	cluster.nodeIDs[no1] = ids[0]
	cluster.nodeIDs[no2] = ids[1]
	cluster.nodeIDs[no3] = ids[2]
	cluster.nodeIDs[no4] = ids[3]
	cluster.dead[no3] = true

	initialMembers := map[kvsTypes.SectorNo]*types.NodeID{
		no1: cluster.nodeIDs[no1],
		no2: cluster.nodeIDs[no2],
		no3: cluster.nodeIDs[no3],
	}

	c1 := cluster.startConsensus(no1, false, initialMembers)
	cluster.startConsensus(no2, false, initialMembers)

	// the surviving majority (2/3) elects a leader
	require.Eventually(t, func() bool {
		return c1.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	// commit a proposal while member 3 is dead
	c1.Propose(activateProposal(cluster.nodeIDs[no1]))
	require.Eventually(t, func() bool {
		return cluster.appliedCount(no1) >= 1 && cluster.appliedCount(no2) >= 1
	}, 10*time.Second, 100*time.Millisecond)

	// append a fresh member with an empty log
	c1.AppendNode(no4, cluster.nodeIDs[no4])

	allMembers := map[kvsTypes.SectorNo]*types.NodeID{
		no1: cluster.nodeIDs[no1],
		no2: cluster.nodeIDs[no2],
		no3: cluster.nodeIDs[no3],
		no4: cluster.nodeIDs[no4],
	}
	cluster.startConsensus(no4, true, allMembers)

	// the joiner must catch up with the previously committed proposal
	require.Eventually(t, func() bool {
		return cluster.appliedCount(no4) >= 1
	}, 15*time.Second, 100*time.Millisecond)
}
