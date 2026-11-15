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
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3"
	proto3 "google.golang.org/protobuf/proto"
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
	errors     []error
}

var _ SectorHandler = &sectorHandlerHelper{}

func (h *sectorHandlerHelper) SectorError(sectorKey *kvsTypes.SectorKey, err error) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.errors = append(h.errors, err)
}

func (h *sectorHandlerHelper) reportedErrors() []error {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return append([]error(nil), h.errors...)
}

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

// storeHelper follows SimpleStore for sector allocation: allocating twice
// and releasing a sector that was never allocated are errors. It keeps the
// records of one sector, and releasing the sector drops them.
type storeHelper struct {
	mtx       sync.Mutex
	allocated map[kvsTypes.SectorKey]struct{}
	values    map[string][]byte
}

var _ kvsTypes.Store = &storeHelper{}

func (s *storeHelper) AllocateSector(sectorKey *kvsTypes.SectorKey) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.allocated == nil {
		s.allocated = make(map[kvsTypes.SectorKey]struct{})
	}
	if _, exists := s.allocated[*sectorKey]; exists {
		return kvsTypes.ErrorStoreSectorExists
	}
	s.allocated[*sectorKey] = struct{}{}
	return nil
}

func (s *storeHelper) ReleaseSector(sectorKey *kvsTypes.SectorKey) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if _, exists := s.allocated[*sectorKey]; !exists {
		return kvsTypes.ErrorStoreSectorNotFound
	}
	delete(s.allocated, *sectorKey)
	s.values = nil
	return nil
}

func (s *storeHelper) Get(sectorKey *kvsTypes.SectorKey, key string) ([]byte, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	value, exists := s.values[key]
	if !exists {
		return nil, kvsTypes.ErrorStoreKeyNotFound
	}
	return value, nil
}
func (s *storeHelper) Set(sectorKey *kvsTypes.SectorKey, key string, value []byte) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if _, exists := s.allocated[*sectorKey]; !exists {
		return kvsTypes.ErrorStoreSectorNotFound
	}
	if s.values == nil {
		s.values = make(map[string][]byte)
	}
	s.values[key] = value
	return nil
}

func (s *storeHelper) value(key string) []byte {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.values[key]
}
func (s *storeHelper) Patch(sectorKey *kvsTypes.SectorKey, key string, value []byte) error {
	return nil
}
func (s *storeHelper) Delete(sectorKey *kvsTypes.SectorKey, key string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	delete(s.values, key)
	return nil
}

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

// TestSector_checkQuorumLoss covers checkQuorumLoss: a sector destroys itself
// locally once its raft group has had no leader for forceTerminateDuration,
// because a group that lost its quorum can never commit anything, not even a
// normal Terminate proposal. A healthy group with a leader must never be
// destroyed this way.
func TestSector_checkQuorumLoss(t *testing.T) {
	tests := []struct {
		name string
		// withDeadPeer adds a peer that never runs, so the group is a
		// 2-member majority that can never elect a leader.
		withDeadPeer bool
		// forceTerminateDuration must be longer than the time it normally
		// takes to elect a leader, or the "has a leader" case could destroy
		// the sector before the leader is elected.
		forceTerminateDuration time.Duration
		wantTerminated         bool
	}{
		{
			name:                   "destroys the sector when the group never elects a leader",
			withDeadPeer:           true,
			forceTerminateDuration: 1 * time.Second,
			wantTerminated:         true,
		},
		{
			name:                   "keeps the sector when the group has a leader",
			withDeadPeer:           false,
			forceTerminateDuration: 5 * time.Second,
			wantTerminated:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			nodeIDs := testUtil.UniqueNodeIDs(2)
			localNodeID := nodeIDs[0]
			members := map[kvsTypes.SectorNo]*types.NodeID{
				kvsTypes.HostNodeSectorNo: localNodeID,
			}
			if tt.withDeadPeer {
				members[kvsTypes.SectorNo(2)] = nodeIDs[1]
			}

			handler := &sectorHandlerHelper{}
			s := newTestSector(t, ctx, localNodeID, handler, members)
			s.proposalRetryDuration = 200 * time.Millisecond
			s.forceTerminateDuration = tt.forceTerminateDuration
			s.Start(ctx)

			if tt.wantTerminated {
				require.Eventually(t, func() bool {
					return handler.terminatedCount() > 0
				}, 10*time.Second, 100*time.Millisecond)

				s.mtx.RLock()
				defer s.mtx.RUnlock()
				require.True(t, s.terminated)
				require.True(t, s.stopped)
				return
			}

			// wait for the single-voter group to elect itself, then make
			// sure it stays untouched past forceTerminateDuration
			require.Eventually(t, func() bool {
				return s.consensus.Status().Lead != 0
			}, 10*time.Second, 100*time.Millisecond)
			time.Sleep(tt.forceTerminateDuration + time.Second)

			require.Equal(t, 0, handler.terminatedCount())
			require.Nil(t, s.GetTailAddress()) // untouched, still inactive
		})
	}
}

// TestSector_terminate_inactiveSector covers Terminate on an inactive sector
// of a healthy group. Such a sector was never allocated in the store, so
// ReleaseSector fails; the apply must still complete instead of leaving the
// Terminate to be re-proposed forever.
func TestSector_terminate_inactiveSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := testUtil.UniqueNodeIDs(1)[0]
	handler := &sectorHandlerHelper{}
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
	require.Empty(t, handler.reportedErrors())
}

// TestSector_import_activeSector covers Import into an active sector, which
// is how a merge moves records into the hosting sector. The store sector is
// already allocated by the activate, and the import must still store the
// records.
func TestSector_import_activeSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, tailNodeID := nodeIDs[0], nodeIDs[1]
	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.Start(ctx)

	require.Eventually(t, func() bool {
		return s.consensus.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	s.Activate(*tailNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	require.NoError(t, s.Import(map[string][]byte{"key": []byte("value")}))
	require.Equal(t, []byte("value"), s.store.(*storeHelper).value("key"))
	require.Empty(t, handler.reportedErrors())
}

// TestSector_processCommitSplitProposal_alreadyActivated covers a CommitSplit
// applied to a sector that is already active: it must be a no-op that clears
// the pending proposal, so the retry loop stops proposing it.
func TestSector_processCommitSplitProposal_alreadyActivated(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	localNodeID, tailNodeID, splitTailNodeID := nodeIDs[0], nodeIDs[1], nodeIDs[2]
	s := newTestSector(t, t.Context(), localNodeID, &sectorHandlerHelper{}, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	defer s.consensus.Stop()

	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.tail = tailNodeID
	s.proposalCommittingSplit = splitTailNodeID

	err := s.processCommitSplitProposal(&proto.CommitSplit{Tail: splitTailNodeID.Proto()})
	require.NoError(t, err)
	require.Nil(t, s.proposalCommittingSplit)
	require.True(t, s.tail.Equal(tailNodeID))
}

// TestSector_import_commitSplit_onInactiveSector covers the frontward side of
// a split: the frontward sector is inactive while the split imports records
// into it, and CommitSplit then activates it. Both must be applied although
// the sector is not active yet.
func TestSector_import_commitSplit_onInactiveSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, tailNodeID := nodeIDs[0], nodeIDs[1]

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.Start(ctx)

	require.Eventually(t, func() bool {
		return s.consensus.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	require.NoError(t, s.Import(map[string][]byte{"key": []byte("value")}))
	require.Equal(t, []byte("value"), s.store.(*storeHelper).value("key"))
	require.Nil(t, s.GetTailAddress())

	require.NoError(t, s.CommitSplit(tailNodeID))
	require.True(t, s.GetTailAddress().Equal(tailNodeID))
	require.Equal(t, 0, handler.terminatedCount())
	require.Empty(t, handler.reportedErrors())
}

// TestSector_processCommitSplitProposal_activatePending covers a CommitSplit
// applied while this replica has its own Activate pending. The pending flag
// is local to this replica, so the apply must not depend on it: CommitSplit
// sets the tail as on every other replica, and the later Activate is ignored.
func TestSector_processCommitSplitProposal_activatePending(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	localNodeID, activateTailNodeID, splitTailNodeID := nodeIDs[0], nodeIDs[1], nodeIDs[2]
	s := newTestSector(t, t.Context(), localNodeID, &sectorHandlerHelper{}, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	defer s.consensus.Stop()

	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.proposalActivating = activateTailNodeID

	require.NoError(t, s.processCommitSplitProposal(&proto.CommitSplit{Tail: splitTailNodeID.Proto()}))
	require.True(t, s.tail.Equal(splitTailNodeID))

	require.NoError(t, s.processActivateProposal(&proto.Activate{Tail: activateTailNodeID.Proto()}))
	require.True(t, s.tail.Equal(splitTailNodeID))
	require.Nil(t, s.proposalActivating)
}

// TestSector_appendDeadNode_keepsQuorum covers appending a member that never
// runs. It joins as a learner, so the single-voter group keeps its quorum and
// keeps committing, and the sector is not destroyed although the append stays
// pending.
func TestSector_appendDeadNode_keepsQuorum(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, deadNodeID := nodeIDs[0], nodeIDs[1]

	handler := &sectorHandlerHelper{}
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

	s.AppendNode(kvsTypes.SectorNo(2), deadNodeID)
	require.Eventually(t, func() bool {
		return len(s.consensus.Status().Config.Learners) == 1
	}, 10*time.Second, 100*time.Millisecond)

	s.Activate(*deadNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	time.Sleep(4 * time.Second)
	require.Equal(t, 0, handler.terminatedCount())
	require.Len(t, s.consensus.Status().Config.Voters[0], 1)
}

// TestSector_checkQuorumLoss_pendingWithoutProgress covers the backstop: a
// proposal that stays pending without any commit progress destroys the
// replica even when the leaderless check is effectively disabled.
func TestSector_checkQuorumLoss_pendingWithoutProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, deadNodeID := nodeIDs[0], nodeIDs[1]

	handler := &sectorHandlerHelper{}
	// the other voter never runs, so nothing can be committed
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
		kvsTypes.SectorNo(2):      deadNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.forceTerminateDuration = time.Hour
	s.forcePendingDuration = 3 * time.Second
	s.Start(ctx)

	s.Activate(*deadNodeID)

	require.Eventually(t, func() bool {
		return handler.terminatedCount() > 0
	}, 15*time.Second, 100*time.Millisecond)
	require.Nil(t, s.GetTailAddress())
}

// TestSector_import_blockedOnQuorumLoss covers waitProposal: a blocking
// operation such as Import must not wait forever when its raft group cannot
// commit the proposal. It gives up with ErrProposalTimeout once
// proposalWaitTimeout elapses, or with ErrSectorStopped once checkQuorumLoss
// destroys the sector first. Either way, the pending proposal must be
// cleared so it is not retried forever.
func TestSector_import_blockedOnQuorumLoss(t *testing.T) {
	tests := []struct {
		name                   string
		proposalWaitTimeout    time.Duration
		forceTerminateDuration time.Duration
		wantErr                error
	}{
		{
			name:                   "returns ErrProposalTimeout once the wait times out",
			proposalWaitTimeout:    1 * time.Second,
			forceTerminateDuration: time.Hour, // keep the destroy path from firing first
			wantErr:                ErrProposalTimeout,
		},
		{
			name:                   "returns ErrSectorStopped once a forced local destroy wakes it up",
			proposalWaitTimeout:    time.Hour, // keep the timeout path from firing first
			forceTerminateDuration: 1 * time.Second,
			wantErr:                ErrSectorStopped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			nodeIDs := testUtil.UniqueNodeIDs(2)
			localNodeID, deadNodeID := nodeIDs[0], nodeIDs[1]

			handler := &sectorHandlerHelper{}
			s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
				kvsTypes.HostNodeSectorNo: localNodeID,
				kvsTypes.SectorNo(2):      deadNodeID,
			})
			s.proposalRetryDuration = 200 * time.Millisecond
			s.proposalWaitTimeout = tt.proposalWaitTimeout
			s.forceTerminateDuration = tt.forceTerminateDuration
			s.Start(ctx)

			done := make(chan error, 1)
			go func() {
				done <- s.Import(map[string][]byte{"key": []byte("value")})
			}()

			select {
			case err := <-done:
				require.ErrorIs(t, err, tt.wantErr)
			case <-time.After(10 * time.Second):
				t.Fatal("Import did not return")
			}

			require.False(t, s.HasManagementProposal())
			if tt.wantErr == ErrSectorStopped {
				require.Eventually(t, func() bool {
					return handler.terminatedCount() > 0
				}, 5*time.Second, 100*time.Millisecond)
			}
		})
	}
}

// TestSector_mergeConflict_rejectedBeforeProposing covers PrepareMerge and
// PreCommitSplit on a sector whose prepare_merge is held by another node. Both
// must fail without proposing anything: a leftover pending proposal would be
// re-proposed on every retry, and a PreCommitSplit committed after the caller
// gave up would shrink the tail of a split that is being aborted.
func TestSector_mergeConflict_rejectedBeforeProposing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(4)
	localNodeID, holderNodeID, otherNodeID, tailNodeID := nodeIDs[0], nodeIDs[1], nodeIDs[2], nodeIDs[3]

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.Start(ctx)

	s.Activate(*tailNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)
	require.NoError(t, s.PrepareMerge(holderNodeID))

	err := s.PrepareMerge(otherNodeID)
	require.ErrorContains(t, err, "merge is prepared by")
	require.False(t, s.HasManagementProposal())

	err = s.PreCommitSplit(otherNodeID)
	require.ErrorContains(t, err, "merge is being prepared by")
	require.False(t, s.HasManagementProposal())
	require.Never(t, func() bool {
		return !s.GetTailAddress().Equal(tailNodeID)
	}, time.Second, 100*time.Millisecond)

	// the holder itself can prepare the merge again
	require.NoError(t, s.PrepareMerge(holderNodeID))
	require.Equal(t, 0, handler.terminatedCount())
}

// TestSector_prepareMerge_releasedAfterHolderStalls covers a prepare_merge
// claim whose holder never finishes the merge. Once another node's PrepareMerge
// has been rejected by the same holder for mergeReleaseDuration, the claim is
// released and the other node's PrepareMerge succeeds.
func TestSector_prepareMerge_releasedAfterHolderStalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(4)
	localNodeID, holderNodeID, mergerNodeID, tailNodeID := nodeIDs[0], nodeIDs[1], nodeIDs[2], nodeIDs[3]

	handler := &sectorHandlerHelper{}
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
	require.NoError(t, s.PrepareMerge(holderNodeID))

	require.ErrorContains(t, s.PrepareMerge(mergerNodeID), "merge is prepared by")

	require.Eventually(t, func() bool {
		return s.PrepareMerge(mergerNodeID) == nil
	}, 10*time.Second, 200*time.Millisecond)

	s.mtx.RLock()
	defer s.mtx.RUnlock()
	require.True(t, s.mergeBy.Equal(mergerNodeID))
	require.Nil(t, s.proposalReleaseMerge)
	require.Equal(t, 0, handler.terminatedCount())
}

// TestSector_preCommitSplit_releasedAfterHolderStalls covers the same stale
// claim blocking a split of the hosting sector. The split proceeds once the
// claim is released.
func TestSector_preCommitSplit_releasedAfterHolderStalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(4)
	localNodeID, holderNodeID, splitNodeID, tailNodeID := nodeIDs[0], nodeIDs[1], nodeIDs[2], nodeIDs[3]

	handler := &sectorHandlerHelper{}
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
	require.NoError(t, s.PrepareMerge(holderNodeID))

	require.ErrorContains(t, s.PreCommitSplit(splitNodeID), "merge is being prepared by")

	require.Eventually(t, func() bool {
		return s.PreCommitSplit(splitNodeID) == nil
	}, 10*time.Second, 200*time.Millisecond)
	require.True(t, s.GetTailAddress().Equal(splitNodeID))
	require.Equal(t, 0, handler.terminatedCount())
}

// TestSector_processReleaseMergeProposal covers the apply of ReleaseMerge. It
// clears the claim only while the claim is held by the given node, so a
// release that races with a newer claim keeps the newer one.
func TestSector_processReleaseMergeProposal(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	localNodeID, oldHolderNodeID, newHolderNodeID := nodeIDs[0], nodeIDs[1], nodeIDs[2]
	s := newTestSector(t, t.Context(), localNodeID, &sectorHandlerHelper{}, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	defer s.consensus.Stop()

	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.mergeBy = newHolderNodeID
	s.proposalReleaseMerge = oldHolderNodeID
	require.NoError(t, s.processReleaseMergeProposal(&proto.ReleaseMerge{Handler: oldHolderNodeID.Proto()}))
	require.True(t, s.mergeBy.Equal(newHolderNodeID))
	require.Nil(t, s.proposalReleaseMerge)

	require.NoError(t, s.processReleaseMergeProposal(&proto.ReleaseMerge{Handler: newHolderNodeID.Proto()}))
	require.Nil(t, s.mergeBy)
}

func importProposal(records map[string][]byte) *proto.ConsensusProposal {
	importing := &proto.Import{}
	for key, value := range records {
		importing.Records = append(importing.Records, &proto.Import_Record{Key: key, Value: value})
	}
	return &proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Import{Import: importing},
	}
}

func activateProposal(tail *types.NodeID) *proto.ConsensusProposal {
	return &proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Activate{
			Activate: &proto.Activate{Tail: tail.Proto()},
		},
	}
}

func prepareMergeProposal(handler *types.NodeID) *proto.ConsensusProposal {
	return &proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_PrepareMerge{
			PrepareMerge: &proto.PrepareMerge{Handler: handler.Proto()},
		},
	}
}

// TestSector_snapshot covers a snapshot taken on one replica and applied on
// another. The receiver must end up with the replicated state of the source,
// and lose what it had that the source does not: a replica that fell behind
// may hold records that the group has deleted since. The sectors are not
// started: the test calls the handlers as the consensus loop would.
func TestSector_snapshot(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDsInRingOrder(4)
	localNodeID, staleTail, tail, mergeBy := nodeIDs[0], nodeIDs[1], nodeIDs[2], nodeIDs[3]

	tests := []struct {
		name string
		// proposals are applied to the source before the snapshot is taken.
		proposals     []*proto.ConsensusProposal
		expectTail    *types.NodeID
		expectMergeBy *types.NodeID
		expectRecords map[string][]byte
	}{
		{
			name: "active sector with records and a prepare_merge claim",
			proposals: []*proto.ConsensusProposal{
				activateProposal(tail),
				importProposal(map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")}),
				prepareMergeProposal(mergeBy),
			},
			expectTail:    tail,
			expectMergeBy: mergeBy,
			expectRecords: map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
		},
		{
			// A split imports the records before it activates the sector.
			name: "inactive sector with records",
			proposals: []*proto.ConsensusProposal{
				importProposal(map[string][]byte{"key1": []byte("value1")}),
			},
			expectRecords: map[string][]byte{"key1": []byte("value1")},
		},
		{
			name:          "inactive sector without records",
			expectRecords: map[string][]byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			members := map[kvsTypes.SectorNo]*types.NodeID{
				kvsTypes.HostNodeSectorNo: localNodeID,
			}

			src := newTestSector(t, ctx, localNodeID, &sectorHandlerHelper{}, members)
			for _, proposal := range tt.proposals {
				require.NoError(t, src.ConsensusApplyProposal(proposal))
			}
			data, err := src.ConsensusGetSnapshot()
			require.NoError(t, err)

			// the receiver has a different state and a record of its own
			dstHandler := &sectorHandlerHelper{}
			dst := newTestSector(t, ctx, localNodeID, dstHandler, members)
			require.NoError(t, dst.ConsensusApplyProposal(activateProposal(staleTail)))
			require.NoError(t, dst.ConsensusApplyProposal(importProposal(map[string][]byte{"stale": []byte("value")})))
			require.NoError(t, dst.ConsensusApplyProposal(prepareMergeProposal(localNodeID)))

			// applying the same snapshot again must not change the result
			for range 2 {
				require.NoError(t, dst.ConsensusApplySnapshot(data))

				if tt.expectTail == nil {
					require.Nil(t, dst.GetTailAddress())
				} else {
					require.True(t, tt.expectTail.Equal(dst.GetTailAddress()))
				}
				dst.mtx.RLock()
				if tt.expectMergeBy == nil {
					require.Nil(t, dst.mergeBy)
				} else {
					require.True(t, tt.expectMergeBy.Equal(dst.mergeBy))
				}
				dst.mtx.RUnlock()

				records, err := dst.operator.ExportAllRecords()
				require.NoError(t, err)
				require.Equal(t, tt.expectRecords, records)
				require.Nil(t, dst.store.(*storeHelper).value("stale"))
			}

			// The receiver works on from the snapshot.
			require.NoError(t, dst.ConsensusApplyProposal(importProposal(map[string][]byte{"later": []byte("value")})))
			require.Equal(t, []byte("value"), dst.store.(*storeHelper).value("later"))
			require.Empty(t, dstHandler.reportedErrors())
		})
	}
}

// TestSector_snapshot_stoppedSector covers snapshots and stopped replicas. A
// replica can be destroyed locally, without raft, so its state is not what
// the group agreed on: it must not create a snapshot. A stopped replica must
// not be revived by a snapshot either.
func TestSector_snapshot_stoppedSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, tail := nodeIDs[0], nodeIDs[1]
	members := map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	}

	live := newTestSector(t, ctx, localNodeID, &sectorHandlerHelper{}, members)
	require.NoError(t, live.ConsensusApplyProposal(activateProposal(tail)))
	require.NoError(t, live.ConsensusApplyProposal(importProposal(map[string][]byte{"key": []byte("value")})))
	data, err := live.ConsensusGetSnapshot()
	require.NoError(t, err)

	t.Run("a locally destroyed replica does not create a snapshot", func(t *testing.T) {
		s := newTestSector(t, ctx, localNodeID, &sectorHandlerHelper{}, members)
		require.NoError(t, s.ConsensusApplyProposal(activateProposal(tail)))
		s.TerminateLocally()

		_, err := s.ConsensusGetSnapshot()
		require.ErrorIs(t, err, consensus.ErrSnapshotUnavailable)
	})

	t.Run("a snapshot does not revive a destroyed replica", func(t *testing.T) {
		handler := &sectorHandlerHelper{}
		s := newTestSector(t, ctx, localNodeID, handler, members)
		s.TerminateLocally()

		require.NoError(t, s.ConsensusApplySnapshot(data))
		s.mtx.RLock()
		require.True(t, s.terminated)
		require.Nil(t, s.tail)
		s.mtx.RUnlock()
		require.Nil(t, s.store.(*storeHelper).value("key"))
		require.Empty(t, handler.reportedErrors())
	})

	t.Run("a snapshot of a terminated sector terminates the replica", func(t *testing.T) {
		handler := &sectorHandlerHelper{}
		s := newTestSector(t, ctx, localNodeID, handler, members)
		require.NoError(t, s.ConsensusApplyProposal(activateProposal(tail)))

		terminated, err := proto3.Marshal(&proto.SectorSnapshot{Terminated: true})
		require.NoError(t, err)
		require.NoError(t, s.ConsensusApplySnapshot(terminated))
		s.mtx.RLock()
		require.True(t, s.terminated)
		s.mtx.RUnlock()
		require.Eventually(t, func() bool {
			return handler.terminatedCount() == 1
		}, 5*time.Second, 10*time.Millisecond)
	})
}

// TestSector_operations covers the data plane through a real raft group of
// one member: a write returns after its operation is committed and applied,
// and what it wrote is read back, stored, and part of a snapshot.
func TestSector_operations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := testUtil.UniqueNodeIDs(1)[0]
	members := map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	}
	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, members)
	s.Start(ctx)
	operator := s.GetOperator()

	require.ErrorIs(t, operator.Set("key", []byte("value1")), kvsTypes.ErrorSectorNotReady)

	// the tail equal to the head: the sector covers the whole ring
	s.Activate(*localNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	require.NoError(t, operator.Set("key", []byte("value1")))
	value, err := operator.Get("key")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)
	require.Equal(t, []byte("value1"), s.store.(*storeHelper).value("key"))

	data, err := s.ConsensusGetSnapshot()
	require.NoError(t, err)
	dst := newTestSector(t, ctx, localNodeID, &sectorHandlerHelper{}, members)
	require.NoError(t, dst.ConsensusApplySnapshot(data))
	value, err = dst.GetOperator().Get("key")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)

	require.NoError(t, operator.Delete("key"))
	_, err = operator.Get("key")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)
	require.Nil(t, s.store.(*storeHelper).value("key"))
	require.Empty(t, handler.reportedErrors())
}

// TestSector_operations_rejectedWhileMergePrepared covers a sector with a
// prepare_merge claim: the node that merges it is about to export its
// records, so it must reject writes until the claim is released. A replica
// restored from a snapshot must do the same.
func TestSector_operations_rejectedWhileMergePrepared(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, holder := nodeIDs[0], nodeIDs[1]
	members := map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	}
	s := newTestSector(t, ctx, localNodeID, &sectorHandlerHelper{}, members)
	s.Start(ctx)
	operator := s.GetOperator()

	s.Activate(*localNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)
	require.NoError(t, operator.Set("key1", []byte("value1")))

	require.NoError(t, s.PrepareMerge(holder))
	require.ErrorIs(t, operator.Set("key2", []byte("value2")), kvsTypes.ErrorSectorNotReady)
	value, err := operator.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)

	data, err := s.ConsensusGetSnapshot()
	require.NoError(t, err)
	restored := newTestSector(t, ctx, localNodeID, &sectorHandlerHelper{}, members)
	require.NoError(t, restored.ConsensusApplySnapshot(data))
	require.ErrorIs(t, restored.GetOperator().Set("key2", []byte("value2")), kvsTypes.ErrorSectorNotReady)

	require.NoError(t, s.ConsensusApplyProposal(&proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_ReleaseMerge{
			ReleaseMerge: &proto.ReleaseMerge{Handler: holder.Proto()},
		},
	}))
	require.NoError(t, operator.Set("key2", []byte("value2")))
}
