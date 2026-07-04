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
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
)

// joinTestCluster wires multiple in-process Consensus instances with droppable
// links, mimicking the simulator topology (dead members silently drop packets).
type joinTestCluster struct {
	t        *testing.T
	sectorID kvsTypes.SectorID
	nodeIDs  map[kvsTypes.SectorNo]*types.NodeID

	mtx         sync.Mutex
	consensuses map[kvsTypes.SectorNo]*Consensus
	dead        map[kvsTypes.SectorNo]bool
	applied     map[kvsTypes.SectorNo][]*proto.ConsensusProposal
	snapshots   map[kvsTypes.SectorNo]int
	// appended[X][Y]: member X observed the append (voter promotion) of member Y
	appended map[kvsTypes.SectorNo]map[kvsTypes.SectorNo]int
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
		appended:    map[kvsTypes.SectorNo]map[kvsTypes.SectorNo]int{},
	}
}

func (c *joinTestCluster) addConsensus(sectorNo kvsTypes.SectorNo, join bool, members map[kvsTypes.SectorNo]*types.NodeID) *Consensus {
	handler := &consensusHandlerHelper{
		t: c.t,
		consensusErrorF: func(err error) {
			// proposals may time out while the quorum is degraded; ignore
		},
		consensusAppendNodeF: func(sn kvsTypes.SectorNo, nodeID *types.NodeID) {
			c.mtx.Lock()
			defer c.mtx.Unlock()
			if c.appended[sectorNo] == nil {
				c.appended[sectorNo] = map[kvsTypes.SectorNo]int{}
			}
			c.appended[sectorNo][sn]++
		},
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
				return // dead or not-yet-created member: drop silently
			}
			_ = dst.ProcessMessage(message)
		},
	}

	cons := NewConsensus(&Config{
		Logger:   testUtil.Logger(c.t),
		Handler:  handler,
		Outbound: outbound,
		SectorKey: &kvsTypes.SectorKey{
			SectorID: c.sectorID,
			SectorNo: sectorNo,
		},
		Join:    join,
		Members: members,
	})

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

func (c *joinTestCluster) appendedCount(observer, member kvsTypes.SectorNo) int {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.appended[observer][member]
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

// TestConsensus_joinCatchesUpWithDeadMember reproduces the simulator run-3
// class-C scenario (2026-07-04): a group is bootstrapped with a member that is
// dead from the start (packets silently dropped), the surviving majority
// elects a leader and commits proposals, and then a fresh member joins with an
// empty log (Join=true, RestartNode). The joiner must catch up and apply the
// previously committed proposals; in the simulator the backward node's replica
// stayed empty (tail never set) for 144+ seconds in this situation.
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

	c1 := cluster.addConsensus(no1, false, initialMembers)
	c2 := cluster.addConsensus(no2, false, initialMembers)
	c1.Start(t.Context())
	c2.Start(t.Context())

	// the surviving majority (2/3) elects a leader
	require.Eventually(t, func() bool {
		return c1.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	// commit a proposal while member 3 is dead (equivalent to Activate)
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
	c4 := cluster.addConsensus(no4, true, allMembers)
	c4.Start(t.Context())

	// the joiner must catch up with the previously committed proposal
	require.Eventually(t, func() bool {
		return cluster.appliedCount(no4) >= 1
	}, 15*time.Second, 100*time.Millisecond)
}

// TestConsensus_learnerFirst_deadAppendKeepsQuorum verifies the core property
// of learner-first membership (シミュレーション run4, 2026-07-04 の破棄ストーム
// 対策): appending a dead node adds it as a learner only, so the quorum stays
// with the synced members and the group keeps committing. Before learner-first
// the same sequence grew a single-voter group to an unreachable 2/2 quorum and
// the group could never commit again.
func TestConsensus_learnerFirst_deadAppendKeepsQuorum(t *testing.T) {
	cluster := newJoinTestCluster(t)

	no1 := kvsTypes.SectorNo(1)
	no2 := kvsTypes.SectorNo(2) // dead, appended later

	ids := testUtil.UniqueNodeIDs(2)
	cluster.nodeIDs[no1] = ids[0]
	cluster.nodeIDs[no2] = ids[1]
	cluster.dead[no2] = true

	c1 := cluster.addConsensus(no1, false, map[kvsTypes.SectorNo]*types.NodeID{
		no1: cluster.nodeIDs[no1],
	})
	c1.Start(t.Context())

	require.Eventually(t, func() bool {
		return c1.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	c1.AppendNode(no2, cluster.nodeIDs[no2])

	// wait until the learner-add is applied (it appears in the conf state)
	require.Eventually(t, func() bool {
		c1.mtx.RLock()
		defer c1.mtx.RUnlock()
		return len(c1.confState.Learners) == 1
	}, 10*time.Second, 100*time.Millisecond)

	// the group must still commit with the single synced voter
	c1.Propose(activateProposal(cluster.nodeIDs[no1]))
	require.Eventually(t, func() bool {
		return cluster.appliedCount(no1) >= 1
	}, 10*time.Second, 100*time.Millisecond)

	// the dead learner must never be promoted to voter
	time.Sleep(2 * time.Second)
	require.Equal(t, 0, cluster.appendedCount(no1, no2))
	require.Len(t, c1.Status().Config.Voters[0], 1)
}

// TestConsensus_learnerFirst_liveAppendPromoted verifies that a live appended
// member catches up as a learner and is then promoted to voter by the leader,
// which is when the membership manager is notified via ConsensusAppendNode.
func TestConsensus_learnerFirst_liveAppendPromoted(t *testing.T) {
	cluster := newJoinTestCluster(t)

	no1 := kvsTypes.SectorNo(1)
	no2 := kvsTypes.SectorNo(2) // live, appended later

	ids := testUtil.UniqueNodeIDs(2)
	cluster.nodeIDs[no1] = ids[0]
	cluster.nodeIDs[no2] = ids[1]

	c1 := cluster.addConsensus(no1, false, map[kvsTypes.SectorNo]*types.NodeID{
		no1: cluster.nodeIDs[no1],
	})
	c1.Start(t.Context())

	require.Eventually(t, func() bool {
		return c1.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	// commit history the new member has to catch up with
	c1.Propose(activateProposal(cluster.nodeIDs[no1]))
	require.Eventually(t, func() bool {
		return cluster.appliedCount(no1) >= 1
	}, 10*time.Second, 100*time.Millisecond)

	c1.AppendNode(no2, cluster.nodeIDs[no2])
	c2 := cluster.addConsensus(no2, true, map[kvsTypes.SectorNo]*types.NodeID{
		no1: cluster.nodeIDs[no1],
		no2: cluster.nodeIDs[no2],
	})
	c2.Start(t.Context())

	// the member catches up and is promoted to voter; the promotion is what
	// notifies the handler
	require.Eventually(t, func() bool {
		return cluster.appendedCount(no1, no2) >= 1 && cluster.appliedCount(no2) >= 1
	}, 15*time.Second, 100*time.Millisecond)
	require.Len(t, c1.Status().Config.Voters[0], 2)
}

// TestConsensus_checkQuorum_leaderStepsDown verifies the CheckQuorum premise
// of the quorum-loss detection: when a leader loses contact with the quorum
// (here: the other voter of a 2-voter group dies), it must step down so that
// Status().Lead becomes 0 and the sector's leaderless detection can fire.
func TestConsensus_checkQuorum_leaderStepsDown(t *testing.T) {
	cluster := newJoinTestCluster(t)

	no1 := kvsTypes.SectorNo(1)
	no2 := kvsTypes.SectorNo(2)

	ids := testUtil.UniqueNodeIDs(2)
	cluster.nodeIDs[no1] = ids[0]
	cluster.nodeIDs[no2] = ids[1]

	members := map[kvsTypes.SectorNo]*types.NodeID{
		no1: cluster.nodeIDs[no1],
		no2: cluster.nodeIDs[no2],
	}
	c1 := cluster.addConsensus(no1, false, members)
	c2 := cluster.addConsensus(no2, false, members)
	c1.Start(t.Context())
	c2.Start(t.Context())

	require.Eventually(t, func() bool {
		return c1.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	// kill member 2: whichever side was leader, member 1 must observe
	// Lead == 0 (leader steps down via CheckQuorum, or the follower starts
	// campaigning after losing the heartbeats)
	cluster.mtx.Lock()
	cluster.dead[no2] = true
	cluster.mtx.Unlock()

	require.Eventually(t, func() bool {
		return c1.Status().Lead == 0
	}, 10*time.Second, 100*time.Millisecond)
}

// TestSnapshotGuardStorage covers the guard for the raft panic observed in the
// simulator (run6, 2026-07-04): snapshot creation is not implemented, but a
// leader asked to send a snapshot panics on an empty one. The guard converts
// the empty snapshot into ErrSnapshotTemporarilyUnavailable, which raft
// handles by skipping the send.
func TestSnapshotGuardStorage(t *testing.T) {
	ms := raft.NewMemoryStorage()
	s := &snapshotGuardStorage{ms}

	_, err := s.Snapshot()
	require.ErrorIs(t, err, raft.ErrSnapshotTemporarilyUnavailable)

	// a real snapshot passes through unchanged
	require.NoError(t, ms.Append([]raftpb.Entry{{Index: 1, Term: 1}}))
	_, err = ms.CreateSnapshot(1, &raftpb.ConfState{Voters: []uint64{1}}, []byte("data"))
	require.NoError(t, err)
	snapshot, err := s.Snapshot()
	require.NoError(t, err)
	require.Equal(t, uint64(1), snapshot.Metadata.Index)
}
