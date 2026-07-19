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
package sector

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3"
)

type emptyRaftLogger struct{}

var _ raft.Logger = (*emptyRaftLogger)(nil)

func newEmptyRaftLogger() *emptyRaftLogger { return &emptyRaftLogger{} }

func (e *emptyRaftLogger) Debug(v ...interface{})                   {}
func (e *emptyRaftLogger) Debugf(format string, v ...interface{})   {}
func (e *emptyRaftLogger) Error(v ...interface{})                   {}
func (e *emptyRaftLogger) Errorf(format string, v ...interface{})   {}
func (e *emptyRaftLogger) Info(v ...interface{})                    {}
func (e *emptyRaftLogger) Infof(format string, v ...interface{})    {}
func (e *emptyRaftLogger) Warning(v ...interface{})                 {}
func (e *emptyRaftLogger) Warningf(format string, v ...interface{}) {}
func (e *emptyRaftLogger) Fatal(v ...interface{})                   {}
func (e *emptyRaftLogger) Fatalf(format string, v ...interface{})   {}
func (e *emptyRaftLogger) Panic(v ...interface{})                   {}
func (e *emptyRaftLogger) Panicf(format string, v ...interface{})   {}

type transfererHandlerHelper struct{}

var _ transferer.Handler = &transfererHandlerHelper{}

func (h *transfererHandlerHelper) TransfererSendPacket(p *networkTypes.Packet) {
	// packets to dead peers are dropped
}

func (h *transfererHandlerHelper) TransfererRelayPacket(nid *types.NodeID, p *networkTypes.Packet) {
	panic("not used in this test")
}

type sectorHandlerHelper struct {
	mtx        sync.Mutex
	terminated []kvsTypes.SectorKey
}

var _ SectorHandler = &sectorHandlerHelper{}

func (h *sectorHandlerHelper) SectorError(sectorKey *kvsTypes.SectorKey, err error) {}

func (h *sectorHandlerHelper) SectorAppendNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
}

func (h *sectorHandlerHelper) SectorRemoveNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo) {
}

func (h *sectorHandlerHelper) SectorTerminated(sectorKey *kvsTypes.SectorKey) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.terminated = append(h.terminated, *sectorKey)
}

func (h *sectorHandlerHelper) terminatedCount() int {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return len(h.terminated)
}

// storeHelper mimics SimpleStore's allocation semantics: releasing a sector
// that was never allocated is an error (regression cover for the inactive
// sector terminate stall).
type storeHelper struct {
	mtx       sync.Mutex
	allocated map[kvsTypes.SectorKey]struct{}
}

var _ kvsTypes.Store = &storeHelper{}

func (s *storeHelper) AllocateSector(sectorKey *kvsTypes.SectorKey) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.allocated == nil {
		s.allocated = make(map[kvsTypes.SectorKey]struct{})
	}
	if _, exists := s.allocated[*sectorKey]; exists {
		return fmt.Errorf("sector already exists: %s", sectorKey.SectorID.String())
	}
	s.allocated[*sectorKey] = struct{}{}
	return nil
}

func (s *storeHelper) ReleaseSector(sectorKey *kvsTypes.SectorKey) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if _, exists := s.allocated[*sectorKey]; !exists {
		return fmt.Errorf("sector does not exist: %s", sectorKey.SectorID.String())
	}
	delete(s.allocated, *sectorKey)
	return nil
}

func (s *storeHelper) Get(sectorKey *kvsTypes.SectorKey, key string) ([]byte, error) {
	return nil, kvsTypes.ErrorStoreKeyNotFound
}
func (s *storeHelper) Set(sectorKey *kvsTypes.SectorKey, key string, value []byte) error { return nil }
func (s *storeHelper) Delete(sectorKey *kvsTypes.SectorKey, key string) error            { return nil }

// newTestSector builds a hosting sector whose raft members are the local node
// and the given peers. Peers other than the local node never run, so a sector
// with any dead peer forming a majority can never elect a leader nor commit.
func newTestSector(t *testing.T, ctx context.Context, localNodeID *types.NodeID, handler *sectorHandlerHelper, members map[kvsTypes.SectorNo]*types.NodeID) *Sector {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tr := transferer.NewTransferer(&transferer.Config{
		Logger:  logger,
		Handler: &transfererHandlerHelper{},
	})
	tr.Start(ctx, localNodeID)

	return NewSector(&SectorConfig{
		Logger:     logger,
		RaftLogger: newEmptyRaftLogger(),
		Handler:    handler,
		Outbound:   consensus.NewOutbound(tr),
		Store:      &storeHelper{},
		SectorKey: &kvsTypes.SectorKey{
			SectorID: kvsTypes.SectorID(uuid.New()),
			SectorNo: kvsTypes.HostNodeSectorNo,
		},
		IsHosting: true,
		Join:      false,
		Members:   members,
		Head:      localNodeID,
	})
}

// TestSector_forceTerminate_onQuorumLoss covers the escape path for the stall
// found in the simulator (2026-07-04): a raft group that lost its quorum can
// never commit anything — including Terminate — so the replica must destroy
// itself locally once the group stays leaderless for forceTerminateDuration.
func TestSector_forceTerminate_onQuorumLoss(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
		kvsTypes.SectorNo(2):      deadNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.forceTerminateDuration = 1 * time.Second
	s.Start(ctx)

	require.Eventually(t, func() bool {
		return handler.terminatedCount() > 0
	}, 10*time.Second, 100*time.Millisecond)

	s.mtx.RLock()
	defer s.mtx.RUnlock()
	require.True(t, s.terminated)
	require.True(t, s.stopped)
}

// TestSector_forceTerminate_notFiredWithLeader checks the negative side of the
// quorum-loss detection: a healthy group (single voter, so the local node is
// always the leader once elected) must never be force-terminated.
func TestSector_forceTerminate_notFiredWithLeader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	// Longer than the initial election period so the transient leaderless
	// window right after start cannot trigger a false destroy.
	s.forceTerminateDuration = 3 * time.Second
	s.Start(ctx)

	// wait for the single-voter group to elect itself
	require.Eventually(t, func() bool {
		return s.consensus.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	time.Sleep(4 * time.Second)
	require.Equal(t, 0, handler.terminatedCount())
	require.Nil(t, s.GetTailAddress()) // untouched, still inactive
}

// TestSector_terminate_inactiveSector reproduces the stall observed in the
// simulator (2026-07-04, run 2): Terminate on an INACTIVE sector commits fine
// (the group is healthy), but the apply handler used to bail out because
// ReleaseSector fails for a store sector that was never allocated (allocation
// happens on activate/import only). The terminate then never completed, and
// the retry loop re-proposed it every 3 seconds forever — mimicking the
// quorum-loss symptom on a healthy group and defeating the pending backstop
// (each retry advances the commit index).
func TestSector_terminate_inactiveSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)

	handler := &sectorHandlerHelper{}
	// healthy single-voter group; the sector stays inactive (no Activate)
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.Start(ctx)

	require.Eventually(t, func() bool {
		return s.consensus.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	s.Terminate()

	require.Eventually(t, func() bool {
		return handler.terminatedCount() > 0
	}, 10*time.Second, 100*time.Millisecond)

	s.mtx.RLock()
	defer s.mtx.RUnlock()
	require.True(t, s.terminated)
}

// TestSector_appendDeadNode_learnerKeepsQuorum verifies learner-first
// membership at the sector level (シミュレーション run4, 2026-07-04): appending
// a dead (or not-yet-synced) node must not enter it into the quorum. Before
// learner-first, this exact sequence made the single-voter group grow to an
// unreachable 2/2 quorum: the leader could no longer commit anything (the
// leader-without-quorum stall) and the sector had to be force-terminated.
// Now the dead node stays a learner, the quorum remains 1, and the group
// keeps committing.
func TestSector_appendDeadNode_learnerKeepsQuorum(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &sectorHandlerHelper{}
	// start as a single-voter group so the local node becomes leader
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.forceTerminateDuration = 2 * time.Second
	s.forcePendingDuration = 3 * time.Second
	s.Start(ctx)

	require.Eventually(t, func() bool {
		return s.consensus.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	// the dead member is added as a learner and never promoted
	s.AppendNode(kvsTypes.SectorNo(2), deadNodeID)

	// the group must still be able to commit with the single-voter quorum
	s.Activate(*deadNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	// the healthy group must not be destroyed even though the learner append
	// stays pending (conf-change pendings are excluded from the backstop)
	time.Sleep(4 * time.Second)
	require.Equal(t, 0, handler.terminatedCount())
}

// TestSector_import_timeoutOnQuorumLoss covers the splitSector hang found in
// the simulator (2026-07-04): Import blocks on cond.Wait until the raft group
// applies the proposal, so a quorum-lost group used to block the caller (and
// KVS.mtxOperateSectors) forever. It must give up with ErrProposalTimeout and
// clear the pending proposal so operateSectors is not skipped forever.
func TestSector_import_timeoutOnQuorumLoss(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
		kvsTypes.SectorNo(2):      deadNodeID,
	})
	s.proposalWaitTimeout = 1 * time.Second
	s.forceTerminateDuration = time.Hour // keep the destroy path out of this test
	s.Start(ctx)

	done := make(chan error, 1)
	go func() {
		done <- s.Import(map[string][]byte{"key": []byte("value")}, 0)
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrProposalTimeout)
	case <-time.After(10 * time.Second):
		t.Fatal("Import did not return: cond.Wait without timeout")
	}

	require.False(t, s.HasManagementProposal())
}

// TestSector_import_commitSplit_onInactiveSector reproduces the split receiver
// sequence (シミュレーション 2026-07-06): the frontward sector is inactive by
// definition while a split imports records into it and then activates it with
// CommitSplit. ConsensusApplyProposal used to drop both proposals at the
// "not activated yet" gate even though they committed, so Import timed out on
// a healthy group and no split ever completed (784 attempts, 0 successes) —
// the sector was terminated, re-created inactive, and re-split forever.
func TestSector_import_commitSplit_onInactiveSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	tailNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &sectorHandlerHelper{}
	// healthy single-voter group; the sector stays inactive (no Activate)
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.Start(ctx)

	require.Eventually(t, func() bool {
		return s.consensus.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	require.NoError(t, s.Import(map[string][]byte{"key": []byte("value")}, 0))
	require.Nil(t, s.GetTailAddress()) // import alone must not activate

	require.NoError(t, s.CommitSplit(tailNodeID))
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil && s.GetTailAddress().Equal(tailNodeID)
	}, 10*time.Second, 100*time.Millisecond)
	require.Equal(t, 0, handler.terminatedCount())
}

// TestSector_prepareMerge_releasedAfterHolderStalls reproduces the stale
// prepare_merge deadlock (シミュレーション run 11, 2026-07-09): mergeBy had no
// release path, so once a preparer committed prepare_merge and died before
// commit_merge, every later PrepareMerge on the sector was rejected with
// "merge is prepared by X" forever, and the activation chain stalled (TLA+
// KvsSectorMergeLock Phase 1 の liveness 違反と同じ系列). A conflict that
// outlives mergeReleaseDuration must now trigger a ReleaseMerge commit, after
// which the surviving merger's PrepareMerge succeeds.
func TestSector_prepareMerge_releasedAfterHolderStalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadHolderNodeID := types.NewNormalNodeID(0x8000000000000000, 0)
	mergerNodeID := types.NewNormalNodeID(0xc000000000000000, 0)
	tailNodeID := types.NewNormalNodeID(0xf000000000000000, 0)

	handler := &sectorHandlerHelper{}
	// healthy single-voter group
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.mergeReleaseDuration = 500 * time.Millisecond
	s.Start(ctx)

	// merges target active sectors only
	s.Activate(*tailNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	// the holder commits prepare_merge, then "dies" (never finishes the merge)
	_, errPrepare := s.PrepareMerge(deadHolderNodeID)
	require.NoError(t, errPrepare)

	// another merger is rejected while the claim is fresh
	_, err := s.PrepareMerge(mergerNodeID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "merge is prepared by")
	// the losing attempt must not leak a pending proposal (retry spam)
	require.False(t, s.HasManagementProposal())

	// after mergeReleaseDuration the conflict triggers ReleaseMerge and the
	// merger's PrepareMerge goes through
	require.Eventually(t, func() bool {
		_, errRetry := s.PrepareMerge(mergerNodeID)
		return errRetry == nil
	}, 10*time.Second, 200*time.Millisecond)

	s.mtx.RLock()
	defer s.mtx.RUnlock()
	require.True(t, s.mergeBy.Equal(mergerNodeID)) // CAS kept the new claim
	require.Equal(t, 0, handler.terminatedCount())
}

// TestSector_preCommitSplit_releasedAfterHolderStalls covers the second victim
// of a stale prepare_merge claim: PreCommitSplit on the hosting sector is
// rejected while mergeBy is held, so a holder that died mid-merge used to
// block every future split of this sector (the exact liveness violation shown
// by TLA+ KvsSectorMergeLock Phase 1: a joiner inside the sector range can
// never be activated). The conflict must also be detected before proposing:
// the old in-wait check let the tail shrink commit while the caller aborted.
func TestSector_preCommitSplit_releasedAfterHolderStalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadHolderNodeID := types.NewNormalNodeID(0x8000000000000000, 0)
	splitNodeID := types.NewNormalNodeID(0xc000000000000000, 0)
	tailNodeID := types.NewNormalNodeID(0xf000000000000000, 0)

	handler := &sectorHandlerHelper{}
	// healthy single-voter group
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.mergeReleaseDuration = 500 * time.Millisecond
	s.Start(ctx)

	s.Activate(*tailNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	// a backward node commits prepare_merge on this sector, then "dies"
	_, errPrepare := s.PrepareMerge(deadHolderNodeID)
	require.NoError(t, errPrepare)

	// the split is rejected before proposing: the tail must not shrink
	err := s.PreCommitSplit(splitNodeID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "merge is being prepared by")
	require.True(t, s.GetTailAddress().Equal(tailNodeID))

	// after mergeReleaseDuration the stale claim is released and the split
	// proceeds
	require.Eventually(t, func() bool {
		return s.PreCommitSplit(splitNodeID) == nil
	}, 10*time.Second, 200*time.Millisecond)
	require.True(t, s.GetTailAddress().Equal(splitNodeID))
	require.Equal(t, 0, handler.terminatedCount())
}

// TestSector_import_unblockedByForceTerminate combines both new mechanisms:
// when a blocked operation outlives the quorum-loss detection, the forced
// local destroy must wake it up with ErrSectorStopped (not fake success).
func TestSector_import_unblockedByForceTerminate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
		kvsTypes.SectorNo(2):      deadNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.proposalWaitTimeout = time.Hour // let the destroy path win
	s.forceTerminateDuration = 1 * time.Second
	s.Start(ctx)

	done := make(chan error, 1)
	go func() {
		done <- s.Import(map[string][]byte{"key": []byte("value")}, 0)
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrSectorStopped)
	case <-time.After(10 * time.Second):
		t.Fatal("Import was not unblocked by the forced local destroy")
	}
	require.Eventually(t, func() bool {
		return handler.terminatedCount() > 0
	}, 5*time.Second, 100*time.Millisecond)
}

// TestSector_terminateForMerge_staleTenureIsNoOp covers the tenure (ABA) hole
// of the merge-scoped terminate found by TLA+ KvsSectorFalseLeftover (修正 2):
// a terminate proposed for an abandoned merge tenure must not destroy state
// written under a later tenure of the SAME holder. The apply CASes on
// (mergeBy, mergeGeneration); the generation moves forward on every new
// prepare_merge claim, so the stale entry is consumed as a no-op — including
// its local pending flag, which must not be re-proposed forever.
func TestSector_terminateForMerge_staleTenureIsNoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	rivalNodeID := types.NewNormalNodeID(0x8000000000000000, 0)
	mergerNodeID := types.NewNormalNodeID(0xc000000000000000, 0)
	tailNodeID := types.NewNormalNodeID(0xf000000000000000, 0)

	handler := &sectorHandlerHelper{}
	// healthy single-voter group
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.mergeReleaseDuration = 500 * time.Millisecond
	s.Start(ctx)

	s.Activate(*tailNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	// tenure 1: the merger claims the lock, then abandons the merge
	gen1, err := s.PrepareMerge(mergerNodeID)
	require.NoError(t, err)

	// a rival's rejected attempt starts the release observation; the stale
	// claim is eventually released (checkMergeRelease)
	_, err = s.PrepareMerge(rivalNodeID)
	require.Error(t, err)
	require.Eventually(t, func() bool {
		s.mtx.RLock()
		defer s.mtx.RUnlock()
		return s.mergeBy == nil
	}, 10*time.Second, 200*time.Millisecond)

	// tenure 2 by the SAME holder — the ABA shape
	gen2, err := s.PrepareMerge(mergerNodeID)
	require.NoError(t, err)
	require.NotEqual(t, gen1, gen2)

	// the terminate left over from tenure 1 must be consumed as a no-op
	s.TerminateForMerge(mergerNodeID, gen1)
	require.Eventually(t, func() bool {
		s.mtx.RLock()
		defer s.mtx.RUnlock()
		return !s.proposalTerminating
	}, 10*time.Second, 100*time.Millisecond)
	s.mtx.RLock()
	terminated := s.terminated
	s.mtx.RUnlock()
	require.False(t, terminated)
	require.Equal(t, 0, handler.terminatedCount())

	// the current tenure's terminate destroys the sector
	s.TerminateForMerge(mergerNodeID, gen2)
	require.NoError(t, s.WaitTerminated())
	require.Eventually(t, func() bool {
		return handler.terminatedCount() == 1
	}, 5*time.Second, 100*time.Millisecond)
}

// TestSector_import_rejectedWhileMergeLockHeld covers 修正 4 of TLA+
// KvsSectorFalseLeftover (NoAbsorbWhileLocked): a sector whose mergeBy is
// claimed is about to be exported by its absorber, so records imported after
// that export would be destroyed with the sector without ever reaching the
// absorber. The client-write path is already fenced (mergeFenced); the import
// apply must be fenced too, and the proposer must observe the rejection as an
// error — not as a silently dropped "success".
func TestSector_import_rejectedWhileMergeLockHeld(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	holderNodeID := types.NewNormalNodeID(0x8000000000000000, 0)
	tailNodeID := types.NewNormalNodeID(0xf000000000000000, 0)

	handler := &sectorHandlerHelper{}
	// healthy single-voter group
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.Start(ctx)

	s.Activate(*tailNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	_, err := s.PrepareMerge(holderNodeID)
	require.NoError(t, err)

	err = s.Import(map[string][]byte{"key-1": []byte("value-1")}, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "import fence")
	require.Equal(t, 0, handler.terminatedCount())
}
