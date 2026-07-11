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
	"testing"
	"time"

	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
)

// TestConsensusSnapshot_membersRoundTrip covers the consensus-layer wrapping
// (spec/kvs/snapshot.md): buildSnapshotData wraps the handler's sector-layer
// payload with the member table, and applySnapshot REPLACES the receiver's
// member table with it. A member joining via snapshot never replays the
// compacted conf-change entries, so without this it could not route raft
// messages to any peer.
func TestConsensusSnapshot_membersRoundTrip(t *testing.T) {
	sectorID := testUtil.UniqueSectorIDs(1)[0]
	sectorNos := testUtil.UniqueNumbersU[kvsTypes.SectorNo](3)
	nodeIDs := testUtil.UniqueNodeIDs(3)

	members := map[kvsTypes.SectorNo]*types.NodeID{
		sectorNos[0]: nodeIDs[0],
		sectorNos[1]: nodeIDs[1],
	}

	sectorState := []byte("sector-state")

	src := NewConsensus(&Config{
		Logger:     testUtil.Logger(t),
		Handler: &consensusHandlerHelper{
			t: t,
			consensusGetSnapshotF: func() ([]byte, error) {
				return sectorState, nil
			},
		},
		Outbound:  &consensusOutboundHelper{t: t},
		SectorKey: &kvsTypes.SectorKey{SectorID: sectorID, SectorNo: sectorNos[0]},
		Join:      false,
		Members:   members,
	})
	defer src.Stop()

	data, err := src.buildSnapshotData()
	require.NoError(t, err)

	// the receiver joins with an empty member table plus a stale entry that
	// the snapshot must overwrite (replace, not merge)
	var applied []byte
	dst := NewConsensus(&Config{
		Logger:     testUtil.Logger(t),
		Handler: &consensusHandlerHelper{
			t: t,
			consensusApplySnapshotF: func(snapshot []byte) error {
				applied = snapshot
				return nil
			},
		},
		Outbound:  &consensusOutboundHelper{t: t},
		SectorKey: &kvsTypes.SectorKey{SectorID: sectorID, SectorNo: sectorNos[1]},
		Join:      true,
		Members: map[kvsTypes.SectorNo]*types.NodeID{
			sectorNos[2]: nodeIDs[2], // stale: not in the snapshot
		},
	})
	defer dst.Stop()

	snapshot := raftpb.Snapshot{
		Data: data,
		Metadata: raftpb.SnapshotMetadata{
			Index: 5,
			Term:  1,
			ConfState: raftpb.ConfState{
				Voters: []uint64{uint64(sectorNos[0]), uint64(sectorNos[1])},
			},
		},
	}
	require.NoError(t, dst.applySnapshot(snapshot))

	require.Equal(t, sectorState, applied)

	dst.mtx.RLock()
	require.Len(t, dst.members, 2)
	require.True(t, dst.members[sectorNos[0]].Equal(nodeIDs[0]))
	require.True(t, dst.members[sectorNos[1]].Equal(nodeIDs[1]))
	require.NotContains(t, dst.members, sectorNos[2])
	require.Equal(t, snapshot.Metadata.ConfState, dst.confState)
	dst.mtx.RUnlock()

	require.Equal(t, uint64(5), dst.appliedIndex)
	require.Equal(t, uint64(5), dst.snapshotIndex)
}

// TestConsensus_snapshotTriggerAndCompaction covers Stage 3
// (spec/kvs/snapshot.md): once the applied entries outrun snapCount, the
// consensus loop must create a snapshot and compact the log, bounding the
// raft storage.
func TestConsensus_snapshotTriggerAndCompaction(t *testing.T) {
	cluster := newJoinTestCluster(t)

	no1 := kvsTypes.SectorNo(1)
	cluster.nodeIDs[no1] = testUtil.UniqueNodeIDs(1)[0]

	c1 := cluster.addConsensus(no1, false, map[kvsTypes.SectorNo]*types.NodeID{
		no1: cluster.nodeIDs[no1],
	})
	c1.snapCount = 10
	c1.snapshotCatchUpEntriesN = 5
	c1.Start(t.Context())

	require.Eventually(t, func() bool {
		return c1.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	for range 15 {
		c1.Propose(activateProposal(cluster.nodeIDs[no1]))
	}
	require.Eventually(t, func() bool {
		return cluster.appliedCount(no1) >= 15
	}, 10*time.Second, 100*time.Millisecond)

	// the snapshot must exist and the log head must be compacted away
	require.Eventually(t, func() bool {
		snapshot, err := c1.raftStorage.Snapshot()
		if err != nil || raft.IsEmptySnap(snapshot) {
			return false
		}
		firstIndex, err := c1.raftStorage.FirstIndex()
		return err == nil && firstIndex > 1
	}, 10*time.Second, 100*time.Millisecond)
}

// TestConsensus_joinAfterCompaction_catchesUpViaSnapshot covers Stage 4
// (spec/kvs/snapshot.md): a member appended after the log head was compacted
// away can no longer replay from entry 1 — the leader must serve it an
// InstallSnapshot (passing through snapshotGuardStorage), and the joiner must
// restore the member table from the snapshot, catch up, be promoted, and
// apply later proposals.
func TestConsensus_joinAfterCompaction_catchesUpViaSnapshot(t *testing.T) {
	cluster := newJoinTestCluster(t)

	no1 := kvsTypes.SectorNo(1)
	no2 := kvsTypes.SectorNo(2) // joins after compaction

	ids := testUtil.UniqueNodeIDs(2)
	cluster.nodeIDs[no1] = ids[0]
	cluster.nodeIDs[no2] = ids[1]

	c1 := cluster.addConsensus(no1, false, map[kvsTypes.SectorNo]*types.NodeID{
		no1: cluster.nodeIDs[no1],
	})
	c1.snapCount = 10
	c1.snapshotCatchUpEntriesN = 5
	c1.Start(t.Context())

	require.Eventually(t, func() bool {
		return c1.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	// build enough history to trigger a snapshot and compact the log head
	for range 15 {
		c1.Propose(activateProposal(cluster.nodeIDs[no1]))
	}
	require.Eventually(t, func() bool {
		firstIndex, err := c1.raftStorage.FirstIndex()
		return err == nil && firstIndex > 1
	}, 10*time.Second, 100*time.Millisecond)

	// the joiner starts with an empty log below the leader's first index
	c1.AppendNode(no2, cluster.nodeIDs[no2])
	c2 := cluster.addConsensus(no2, true, map[kvsTypes.SectorNo]*types.NodeID{
		no1: cluster.nodeIDs[no1],
		no2: cluster.nodeIDs[no2],
	})
	c2.Start(t.Context())

	// the joiner must receive the snapshot (not the replayed history) and be
	// promoted to voter once caught up
	require.Eventually(t, func() bool {
		return cluster.snapshotCount(no2) >= 1 && cluster.appendedCount(no1, no2) >= 1
	}, 15*time.Second, 100*time.Millisecond)

	// the member table must have been restored from the snapshot (+ the
	// conf-change entries after it): both members are routable
	c2.mtx.RLock()
	member1, ok1 := c2.members[no1]
	member2, ok2 := c2.members[no2]
	c2.mtx.RUnlock()
	require.True(t, ok1)
	require.True(t, member1.Equal(cluster.nodeIDs[no1]))
	require.True(t, ok2)
	require.True(t, member2.Equal(cluster.nodeIDs[no2]))

	// proposals after the snapshot reach the joiner as normal entries
	c1.Propose(activateProposal(cluster.nodeIDs[no2]))
	require.Eventually(t, func() bool {
		return cluster.appliedCount(no2) >= 1
	}, 10*time.Second, 100*time.Millisecond)
}
