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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/operator"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"go.etcd.io/raft/v3"
	proto3 "google.golang.org/protobuf/proto"
)

// ErrProposalTimeout is returned by blocking sector operations when the raft
// group did not commit the proposal within proposalWaitTimeout. It usually
// means the group has lost its quorum (TLA+ TimeoutAbort).
var ErrProposalTimeout = errors.New("proposal was not committed within timeout")

// ErrSectorStopped is returned by blocking sector operations when the sector
// was stopped or force-terminated before the proposal was committed, so the
// caller can abort the ongoing multi-sector operation instead of treating the
// proposal as applied.
var ErrSectorStopped = errors.New("sector was stopped before the proposal was committed")

type SectorHandler interface {
	SectorError(sectorKey *kvsTypes.SectorKey, err error)
	SectorAppendNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID)
	SectorRemoveNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo)
	SectorTerminated(sectorKey *kvsTypes.SectorKey)
}

type SectorConfig struct {
	Logger     *slog.Logger
	RaftLogger raft.Logger
	Handler    SectorHandler
	Outbound   consensus.OutboundPort
	Store      kvsTypes.Store
	SectorKey  *kvsTypes.SectorKey
	IsHosting  bool
	Join       bool
	Members    map[kvsTypes.SectorNo]*types.NodeID
	Head       *types.NodeID
}

type SectorInfo struct {
	Head *types.NodeID
	Tail *types.NodeID
}

type Sector struct {
	handler   SectorHandler
	store     kvsTypes.Store
	sectorKey kvsTypes.SectorKey
	isHosting bool
	consensus *consensus.Consensus
	operator  *operator.Operator
	head      types.NodeID

	proposalRetryDuration time.Duration
	// proposalWaitTimeout bounds the cond.Wait of blocking operations
	// (Extend/Import/PreCommitSplit/CommitSplit/PrepareMerge/CommitMerge) so a
	// quorum-lost group cannot block the caller (and mtxOperateSectors) forever.
	proposalWaitTimeout time.Duration
	// forceTerminateDuration is how long the raft group may stay leaderless
	// before the local replica is destroyed without raft (TLA+ LocalDestroy).
	forceTerminateDuration time.Duration
	// forcePendingDuration is how long management proposals may stay pending
	// without any commit progress before the replica is destroyed without
	// raft. This backstop catches quorum-lost groups that still see a leader
	// (e.g. detection races around CheckQuorum step-down), where proposals
	// enter the leader's log but can never commit.
	forcePendingDuration time.Duration
	// mergeReleaseDuration is how long a PrepareMerge/PreCommitSplit attempt
	// may stay blocked by another node's committed prepare_merge (mergeBy)
	// before this replica proposes ReleaseMerge to clear it. mergeBy has no
	// other release path: if the preparer dies (or never finishes) between
	// prepare and commit_merge, every later merge/split on this sector is
	// rejected forever and the activation chain stalls (シミュレーション run 11,
	// 2026-07-09; TLA+ KvsSectorMergeLock Phase 1 の liveness 違反)。A healthy
	// merge completes in well under a second, so a conflict persisting this
	// long means the holder is dead or stuck; a false positive merely restores
	// the lock-free interleaving whose safety is model-checked (Phase 3).
	mergeReleaseDuration time.Duration
	// leaderlessSince / pendingSince / pendingCommitIndex are touched only by
	// the Start loop goroutine.
	leaderlessSince    time.Time
	pendingSince       time.Time
	pendingCommitIndex uint64
	triggerCh          chan struct{}
	stopCtx            context.CancelFunc

	mtx                        sync.RWMutex
	cond                       *sync.Cond
	stopped                    bool
	tail                       *types.NodeID
	mergeBy                    *types.NodeID
	terminated                 bool
	proposalAppendingNodes     map[kvsTypes.SectorNo]*types.NodeID
	proposalRemovingNodes      map[kvsTypes.SectorNo]struct{}
	proposalActivating         *types.NodeID // tail
	proposalTerminating        bool
	proposalExtending          *types.NodeID // tail
	proposalImporting          []*proto.Import_Record
	proposalPreCommitSplitting *types.NodeID // frontwardNodeID
	proposalCommittingSplit    *types.NodeID // tail
	proposalPrepareMerge       *types.NodeID // proposed by
	proposalCommittingMerge    *types.NodeID // tail
	proposalReleaseMerge       *types.NodeID // holder to release (CAS)
	// mergeConflictHolder / mergeConflictSince track how long local
	// PrepareMerge/PreCommitSplit attempts have been rejected by the same
	// mergeBy holder; checkMergeRelease proposes ReleaseMerge once the
	// conflict outlives mergeReleaseDuration.
	mergeConflictHolder *types.NodeID
	mergeConflictSince  time.Time
}

func NewSector(config *SectorConfig) *Sector {
	sector := &Sector{
		handler:                config.Handler,
		store:                  config.Store,
		sectorKey:              *config.SectorKey,
		isHosting:              config.IsHosting,
		head:                   *config.Head,
		proposalRetryDuration:  3 * time.Second,
		proposalWaitTimeout:    15 * time.Second,
		forceTerminateDuration: 30 * time.Second,
		forcePendingDuration:   45 * time.Second,
		mergeReleaseDuration:   30 * time.Second,
		triggerCh:              make(chan struct{}, 1),
		proposalAppendingNodes: make(map[kvsTypes.SectorNo]*types.NodeID),
		proposalRemovingNodes:  make(map[kvsTypes.SectorNo]struct{}),
	}
	sector.cond = sync.NewCond(sector.mtx.RLocker())

	sector.consensus = consensus.NewConsensus(&consensus.Config{
		Logger:     config.Logger,
		RaftLogger: config.RaftLogger,
		Handler:    sector,
		Outbound:   config.Outbound,
		SectorKey:  config.SectorKey,
		Join:       config.Join,
		Members:    config.Members,
	})

	sector.operator = operator.NewOperator(&operator.Config{
		SectorKey: config.SectorKey,
		Handler:   sector,
		Store:     config.Store,
		Head:      config.Head,
	})

	return sector
}

func (s *Sector) Start(ctx context.Context) {
	s.consensus.Start(ctx)

	// Derive from the node context so the loop (and quorum-loss detection)
	// stops on node shutdown as well, not only via Stop().
	loopCtx, cancel := context.WithCancel(ctx)
	s.stopCtx = cancel

	go func() {
		defer cancel()
		timer := time.NewTimer(s.proposalRetryDuration)
		defer timer.Stop()

		for {
			select {
			case <-loopCtx.Done():
				return

			case <-timer.C:
				s.checkQuorumLoss()
				s.checkMergeRelease()
				s.applyProposals(true)

			case <-s.triggerCh:
				s.applyProposals(false)
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}

			timer.Reset(s.proposalRetryDuration)
		}
	}()
}

func (s *Sector) Stop() {
	s.stopCtx()
	s.consensus.Stop()

	s.mtx.Lock()
	s.stopped = true
	s.mtx.Unlock()
	s.cond.Broadcast()
}

func (s *Sector) ProcessConsensusMessage(message *proto.ConsensusMessage) error {
	return s.consensus.ProcessMessage(message)
}

func (s *Sector) AppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	if !s.isHosting {
		panic("only host sector can append node")
	}

	s.mtx.Lock()
	existing := s.proposalAppendingNodes[sectorNo]
	if existing != nil {
		if *existing != *nodeID {
			panic(fmt.Sprintf("sectorNo %d is already being appended with nodeID %s", sectorNo, existing.String()))
		}
		s.mtx.Unlock()
		return
	}

	s.proposalAppendingNodes[sectorNo] = nodeID
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}
}

func (s *Sector) RemoveNode(sectorNo kvsTypes.SectorNo) {
	if !s.isHosting {
		panic("only host sector can remove node")
	}

	s.mtx.Lock()
	if _, exists := s.proposalRemovingNodes[sectorNo]; exists {
		s.mtx.Unlock()
		return
	}

	s.proposalRemovingNodes[sectorNo] = struct{}{}
	delete(s.proposalAppendingNodes, sectorNo)
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}
}

// waitProposal blocks until check reports done or the sector stops. check runs
// with s.mtx read-locked. It returns ErrProposalTimeout after proposalWaitTimeout;
// the caller must clear its pending proposal so applyProposals stops re-proposing it.
func (s *Sector) waitProposal(check func() (done bool, err error)) error {
	deadline := time.Now().Add(s.proposalWaitTimeout)
	// Broadcast has no effect on waiters that have not called Wait yet, but such
	// a waiter re-checks the deadline before the next Wait, so no wakeup is lost.
	wake := time.AfterFunc(s.proposalWaitTimeout, func() { s.cond.Broadcast() })
	defer wake.Stop()

	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if s.stopped {
			return ErrSectorStopped
		}
		if !time.Now().Before(deadline) {
			return ErrProposalTimeout
		}
		s.cond.Wait()
	}
}

func (s *Sector) HasManagementProposal() bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.hasManagementProposalLocked()
}

// hasManagementProposalLocked reports whether a (non conf-change) management
// proposal is pending. Call with s.mtx held.
func (s *Sector) hasManagementProposalLocked() bool {
	return s.proposalActivating != nil ||
		s.proposalExtending != nil ||
		s.proposalImporting != nil ||
		s.proposalPreCommitSplitting != nil ||
		s.proposalCommittingSplit != nil ||
		s.proposalPrepareMerge != nil ||
		s.proposalCommittingMerge != nil ||
		s.proposalReleaseMerge != nil
}

func (s *Sector) Activate(tail types.NodeID) {
	if !s.isHosting {
		panic("only host sector can be activated")
	}

	s.mtx.Lock()
	if s.tail != nil || s.terminated ||
		s.proposalActivating != nil || s.proposalTerminating {
		s.mtx.Unlock()
		return
	}

	s.proposalActivating = &tail
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}
}

// NOTE: 終了も raft コミットを要するため (applyProposals 冒頭)、quorum を失った
// グループは自分自身を終了することすらできない。シミュレーション (2026-07-04) で、
// KVS 側が Terminate を毎秒呼び続けても terminated にならないケースを観測。
// → リーダー不在が forceTerminateDuration 続いた場合に raft を経由せずローカルで
// 破棄する脱出経路を checkQuorumLoss として実装 (2026-07-04)。
func (s *Sector) Terminate() {
	s.mtx.Lock()
	if s.terminated || s.proposalTerminating {
		s.mtx.Unlock()
		return
	}

	s.proposalActivating = nil
	s.proposalTerminating = true
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}
}

func (s *Sector) Extend(newTail types.NodeID) error {
	if !s.isHosting {
		panic("only host sector can be extended")
	}

	s.mtx.Lock()

	if newTail.IsBetween(&s.head, s.tail) {
		s.mtx.Unlock()
		return nil
	}
	s.proposalExtending = &newTail
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	if err := s.waitProposal(func() (bool, error) {
		return s.proposalExtending == nil, nil
	}); err != nil {
		s.mtx.Lock()
		s.proposalExtending = nil
		s.mtx.Unlock()
		return fmt.Errorf("failed to extend sector: %w", err)
	}
	return nil
}

func (s *Sector) Migrate(to *Sector) error {
	splittingAddress := to.GetHeadAddress()
	s.operator.SetSplitting(splittingAddress)

	records, err := s.operator.ExportRecords(splittingAddress, s.GetTailAddress())
	if err != nil {
		return fmt.Errorf("failed to export records: %w", err)
	}

	return to.Import(records)
}

func (s *Sector) PreCommitSplit(frontwardNodeID *types.NodeID) error {
	if !s.isHosting {
		panic("only host sector can be extended")
	}

	s.mtx.Lock()
	// Reject before proposing when a prepare_merge claim is committed (TLA+
	// ProposeSplit の enabling `mergeLock[n] = -1`)。Proposing anyway would
	// let the tail shrink commit while the caller aborts, and the conflict
	// error used to leak the pending flag, re-proposing the shrink every
	// retry tick. The conflict starts the release observation, the escape
	// path for a holder that died mid-merge (run 11).
	if s.mergeBy != nil {
		holder := s.mergeBy
		s.noteMergeConflictLocked(holder)
		s.mtx.Unlock()
		return fmt.Errorf("failed to pre-commit split: merge is being prepared by %s", holder.String())
	}
	s.proposalPreCommitSplitting = frontwardNodeID
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	var conflictHolder *types.NodeID
	if err := s.waitProposal(func() (bool, error) {
		if s.mergeBy != nil {
			conflictHolder = s.mergeBy
			return false, fmt.Errorf("cannot pre-commit split while merge is being prepared by %s", s.mergeBy.String())
		}
		return s.proposalPreCommitSplitting == nil, nil
	}); err != nil {
		s.mtx.Lock()
		s.proposalPreCommitSplitting = nil
		if conflictHolder != nil {
			s.noteMergeConflictLocked(conflictHolder)
		}
		s.mtx.Unlock()
		return fmt.Errorf("failed to pre-commit split: %w", err)
	}
	return nil
}

func (s *Sector) CommitSplit(newTail *types.NodeID) error {
	s.mtx.Lock()
	s.proposalCommittingSplit = newTail
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	if err := s.waitProposal(func() (bool, error) {
		return s.proposalCommittingSplit == nil, nil
	}); err != nil {
		s.mtx.Lock()
		s.proposalCommittingSplit = nil
		s.mtx.Unlock()
		return fmt.Errorf("failed to commit split: %w", err)
	}
	return nil
}

func (s *Sector) PrepareMerge(proposedBy *types.NodeID) error {
	s.mtx.Lock()
	// Reject before proposing when another node's prepare_merge is already
	// committed (TLA+ ProposeMerge の enabling `mergeLock[fs] \in {-1, n}`)。
	// The conflict starts the release observation: mergeBy has no other
	// release path, so a holder that died between prepare and commit_merge
	// would otherwise block this sector's merges forever (run 11).
	if s.mergeBy != nil && !s.mergeBy.Equal(proposedBy) {
		holder := s.mergeBy
		s.noteMergeConflictLocked(holder)
		s.mtx.Unlock()
		return fmt.Errorf("failed to prepare merge: merge is prepared by %s, not %s", holder.String(), proposedBy.String())
	}
	s.proposalPrepareMerge = proposedBy
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	var conflictHolder *types.NodeID
	if err := s.waitProposal(func() (bool, error) {
		if s.mergeBy != nil {
			if !s.mergeBy.Equal(proposedBy) {
				conflictHolder = s.mergeBy
				return false, fmt.Errorf("merge is prepared by %s, not %s", s.mergeBy.String(), proposedBy.String())
			}
			return true, nil
		}
		return false, nil
	}); err != nil {
		// Clear the pending flag on every error, including the conflict above
		// (previously only Timeout/Stopped did): a leaked flag makes
		// applyProposals re-propose the losing PrepareMerge every retry tick
		// (run 11 で "pending prepareMerge" の retry スパムとして観測).
		s.mtx.Lock()
		s.proposalPrepareMerge = nil
		if conflictHolder != nil {
			s.noteMergeConflictLocked(conflictHolder)
		}
		s.mtx.Unlock()
		return fmt.Errorf("failed to prepare merge: %w", err)
	}
	return nil
}

func (s *Sector) Merge(from *Sector) error {
	records, err := from.operator.ExportRecords(from.GetHeadAddress(), from.GetTailAddress())
	if err != nil {
		return fmt.Errorf("failed to export records: %w", err)
	}

	return s.Import(records)
}

func (s *Sector) CommitMerge(newTail *types.NodeID) error {
	if !s.isHosting {
		panic("only host sector can commit merge")
	}

	s.mtx.Lock()
	if newTail.IsBetween(&s.head, s.tail) {
		panic("newTail should be greater than or equal to current tail")
	}
	s.proposalCommittingMerge = newTail
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	if err := s.waitProposal(func() (bool, error) {
		return s.proposalCommittingMerge == nil, nil
	}); err != nil {
		s.mtx.Lock()
		s.proposalCommittingMerge = nil
		s.mtx.Unlock()
		return fmt.Errorf("failed to commit merge: %w", err)
	}
	return nil
}

func (s *Sector) Import(records map[string][]byte) error {
	importRecords := make([]*proto.Import_Record, 0, len(records))
	for key, value := range records {
		importRecords = append(importRecords, &proto.Import_Record{
			Key:   key,
			Value: value,
		})
	}

	s.mtx.Lock()
	s.proposalImporting = importRecords
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	if err := s.waitProposal(func() (bool, error) {
		return s.proposalImporting == nil, nil
	}); err != nil {
		s.mtx.Lock()
		s.proposalImporting = nil
		s.mtx.Unlock()
		return fmt.Errorf("failed to import records: %w", err)
	}
	return nil
}

// pendingProposalNames returns labels of pending proposals for debugging. Call with s.mtx held.
func (s *Sector) pendingProposalNames() []string {
	names := []string{}
	if s.proposalTerminating {
		names = append(names, "terminate")
	}
	if len(s.proposalAppendingNodes) > 0 {
		names = append(names, fmt.Sprintf("appendNodes(%d)", len(s.proposalAppendingNodes)))
	}
	if len(s.proposalRemovingNodes) > 0 {
		names = append(names, fmt.Sprintf("removeNodes(%d)", len(s.proposalRemovingNodes)))
	}
	if s.proposalActivating != nil {
		names = append(names, "activate")
	}
	if s.proposalExtending != nil {
		names = append(names, "extend")
	}
	if s.proposalImporting != nil {
		names = append(names, "import")
	}
	if s.proposalPreCommitSplitting != nil {
		names = append(names, "preCommitSplit")
	}
	if s.proposalCommittingSplit != nil {
		names = append(names, "commitSplit")
	}
	if s.proposalPrepareMerge != nil {
		names = append(names, "prepareMerge")
	}
	if s.proposalCommittingMerge != nil {
		names = append(names, "commitMerge")
	}
	if s.proposalReleaseMerge != nil {
		names = append(names, "releaseMerge")
	}
	return names
}

// applyProposals proposes the pending proposals to the raft group. The
// proposals are collected under s.mtx and proposed after releasing it:
// consensus.Propose blocks (bounded by its propose timeout) while the group
// has no leader, and holding s.mtx here would stall every other sector
// operation for that period.
func (s *Sector) applyProposals(retry bool) {
	proposals := []*proto.ConsensusProposal{}

	s.mtx.RLock()

	// A proposal still pending after proposalRetryDuration means the raft group has not
	// committed it yet; dump the raft status to see whether the group has a leader.
	if retry {
		if pending := s.pendingProposalNames(); len(pending) > 0 {
			status := s.consensus.Status()
			// the "==" marker is required: the simulator's log collection only
			// keeps lines containing "==" or "@@" (2026-07-04 の 2 回の実行で
			// "##" 行が全て欠落していたことが判明)
			fmt.Println(time.Now(), s.head.String(), "== retry proposals", s.sectorKey.String(),
				"pending", strings.Join(pending, ","),
				"state", status.RaftState.String(), "lead", status.Lead, "term", status.Term,
				"commit", status.Commit)
		}
	}

	switch {
	case s.proposalTerminating:
		// re-apply terminating, and skip the other proposals
		if !s.terminated {
			proposals = append(proposals, &proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_Terminate{
					Terminate: &proto.Terminate{},
				},
			})
		}

	case s.terminated:
		// nothing to apply

	default:
		// Apply appending nodes (AppendNode/RemoveNode are already asynchronous).
		for sectorNo, nodeID := range s.proposalAppendingNodes {
			s.consensus.AppendNode(sectorNo, nodeID)
		}

		// Apply removing nodes.
		for sectorNo := range s.proposalRemovingNodes {
			s.consensus.RemoveNode(sectorNo)
		}

		// Apply activating.
		if s.proposalActivating != nil {
			proposals = append(proposals, &proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_Activate{
					Activate: &proto.Activate{
						Tail: s.proposalActivating.Proto(),
					},
				},
			})
		}

		// Apply extending
		if s.proposalExtending != nil {
			proposals = append(proposals, &proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_Extend{
					Extend: &proto.Extend{
						Tail: s.proposalExtending.Proto(),
					},
				},
			})
		}

		// Apply importing
		if s.proposalImporting != nil {
			proposals = append(proposals, &proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_Import{
					Import: &proto.Import{
						Records: s.proposalImporting,
					},
				},
			})
		}

		// Apply pre-commit splitting.
		if s.proposalPreCommitSplitting != nil {
			proposals = append(proposals, &proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_PreCommitSplit{
					PreCommitSplit: &proto.PreCommitSplit{
						Tail: s.proposalPreCommitSplitting.Proto(),
					},
				},
			})
		}

		// Apply committing split.
		if s.proposalCommittingSplit != nil {
			proposals = append(proposals, &proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_CommitSplit{
					CommitSplit: &proto.CommitSplit{
						Tail: s.proposalCommittingSplit.Proto(),
					},
				},
			})
		}

		// Apply prepare merge.
		if s.proposalPrepareMerge != nil {
			proposals = append(proposals, &proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_PrepareMerge{
					PrepareMerge: &proto.PrepareMerge{
						Handler: s.proposalPrepareMerge.Proto(),
					},
				},
			})
		}

		// Apply committing merge.
		if s.proposalCommittingMerge != nil {
			proposals = append(proposals, &proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_CommitMerge{
					CommitMerge: &proto.CommitMerge{
						Tail: s.proposalCommittingMerge.Proto(),
					},
				},
			})
		}

		// Apply release merge.
		if s.proposalReleaseMerge != nil {
			proposals = append(proposals, &proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_ReleaseMerge{
					ReleaseMerge: &proto.ReleaseMerge{
						Handler: s.proposalReleaseMerge.Proto(),
					},
				},
			})
		}
	}

	s.mtx.RUnlock()

	for _, proposal := range proposals {
		s.consensus.Propose(proposal)
	}
}

func (s *Sector) GetKey() kvsTypes.SectorKey {
	return s.sectorKey
}

func (s *Sector) GetHeadAddress() *types.NodeID {
	return &s.head
}

func (s *Sector) GetTailAddress() *types.NodeID {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.tail
}

func (s *Sector) GetOperator() operator.Operations {
	return s.operator
}

func (s *Sector) ConsensusError(err error) {
	s.handler.SectorError(&s.sectorKey, err)
}

func (s *Sector) ConsensusApplyProposal(proposal *proto.ConsensusProposal) error {
	if operation := proposal.GetOperation(); operation != nil {
		return s.operator.ApplyProposal(operation)
	}

	s.mtx.Lock()
	defer func() {
		s.mtx.Unlock()
		s.cond.Broadcast()
	}()

	if proposal.GetTerminate() != nil {
		return s.processTerminateProposal()
	}
	if s.stopped || s.terminated {
		return nil
	}

	if activate := proposal.GetActivate(); activate != nil {
		return s.processActivateProposal(activate)
	}
	// Import and CommitSplit target a not-yet-activated sector by design: a
	// split imports records into the inactive frontward sector and then
	// activates it with CommitSplit. They must be applied before the
	// activation gate below — otherwise the committed proposal is silently
	// dropped, the proposer's pending flag is never cleared, and Import()
	// times out even though the group is healthy (commit keeps advancing).
	// (シミュレーション 2026-07-06: このゲートに Import が握り潰されて split が
	// 一度も完了せず、terminate → 再作成 → split 再試行の無限ループにより
	// sector が inactive のまま恒久化するのを観測)
	if importProposal := proposal.GetImport(); importProposal != nil {
		return s.processImportProposal(importProposal)
	}
	if commitSplit := proposal.GetCommitSplit(); commitSplit != nil {
		return s.processCommitSplitProposal(commitSplit)
	}
	// ReleaseMerge is applied regardless of the activation gate: the apply is
	// an idempotent CAS, and gating it would drop a committed proposal without
	// clearing the proposer's pending flag (the run 8 contract violation).
	if releaseMerge := proposal.GetReleaseMerge(); releaseMerge != nil {
		return s.processReleaseMergeProposal(releaseMerge)
	}
	if s.tail == nil { // Not activated yet.
		return nil
	}

	if extend := proposal.GetExtend(); extend != nil {
		return s.processExtendProposal(extend)

	} else if preCommitSplit := proposal.GetPreCommitSplit(); preCommitSplit != nil {
		return s.processPreCommitSplitProposal(preCommitSplit)

	} else if prepareMerge := proposal.GetPrepareMerge(); prepareMerge != nil {
		return s.processPrepareMergeProposal(prepareMerge)

	} else if commitMerge := proposal.GetCommitMerge(); commitMerge != nil {
		return s.processCommitMergeProposal(commitMerge)

	} else {
		return fmt.Errorf("unknown proposal content")
	}
}

func (s *Sector) processActivateProposal(activate *proto.Activate) error {
	tail, err := types.NewNodeIDFromProto(activate.Tail)
	if err != nil {
		return fmt.Errorf("failed to parse tail NodeID: %w", err)
	}

	s.proposalActivating = nil

	// set tail when the sector is activated for the first time
	if s.tail != nil {
		return nil
	}
	s.tail = tail

	// Tolerate an already-allocated store sector (e.g. allocated by a prior
	// import): apply handlers must be idempotent, otherwise the failed apply
	// aborts the remaining committed entries of the batch (publishEntries).
	if err := s.store.AllocateSector(&s.sectorKey); err != nil {
		fmt.Println(time.Now(), s.head.String(), "@@ activate: allocate sector skipped:", err)
	}
	if err := s.operator.SetRange(*tail); err != nil {
		return fmt.Errorf("failed to set range for activate: %w", err)
	}

	return nil
}

func (s *Sector) processTerminateProposal() error {
	return s.terminateLocked()
}

// terminateLocked releases the sector resources and notifies the handler.
// Call with s.mtx write-locked; the caller broadcasts s.cond after unlocking.
//
// Terminate must always complete once it runs: if it bails out before setting
// terminated, the pending proposalTerminating is re-proposed every retry tick
// and each round trips commit → apply-failure forever. An inactive sector was
// never allocated in the store (AllocateSector runs on activate/import), so
// ReleaseSector failing is the normal case there — treat the sector as already
// released and keep going.
// (シミュレーション 2026-07-04: inactive な hosting sector への Terminate が
// ReleaseSector のエラーで完了できず、健全なグループなのに「Terminate 毎秒空振り」
// という quorum 喪失と同じ症状を示し、活性化チェーンを恒久停止させた。
// commit 自体は毎回進むため checkQuorumLoss の pending バックストップも発火しない)
func (s *Sector) terminateLocked() error {
	s.operator.ClearRange()
	if err := s.store.ReleaseSector(&s.sectorKey); err != nil {
		fmt.Println(time.Now(), s.head.String(), "@@ terminate: release sector skipped:", err)
	}

	s.proposalActivating = nil
	s.terminated = true
	s.stopped = true

	go s.handler.SectorTerminated(&s.sectorKey)
	return nil
}

// checkQuorumLoss destroys the local replica without raft when the group can
// no longer commit (TLA+ LocalDestroy). A group that lost its quorum can never
// commit anything — including Terminate — so this is the only escape path;
// without it a stale replica blocks the activation chain forever. Two signals
// are watched:
//
//  1. The group stays leaderless for forceTerminateDuration. Requires
//     CheckQuorum on the raft side so that a leader without a quorum steps
//     down; otherwise Status().Lead stays non-zero on the leader and on the
//     followers it keeps heartbeating (シミュレーション 2026-07-04 で観測).
//  2. Management proposals stay pending with no commit progress for
//     forcePendingDuration. This catches any remaining can't-commit case
//     regardless of what the leader state claims.
//
// If the group is actually alive (e.g. the local node is only partitioned),
// destroying the replica is equivalent to this member leaving, and the
// resulting overlap is repaired by the existing Terminate/Merge paths.
// Runs on the Start loop goroutine only (the *Since fields are not locked).
func (s *Sector) checkQuorumLoss() {
	s.mtx.RLock()
	inactive := s.stopped || s.terminated
	// Conf-change pendings (appendNodes/removeNodes) are excluded from the
	// destroy backstop: with learner-first membership an append legitimately
	// stays pending while the learner catches up (or, for a dead learner,
	// until the membership manager removes it via the routing view), and a
	// healthy but idle group must not be destroyed for it.
	hasPending := s.proposalTerminating || s.hasManagementProposalLocked()
	s.mtx.RUnlock()
	if inactive {
		return
	}

	status := s.consensus.Status()

	if status.Lead != raft.None {
		s.leaderlessSince = time.Time{}
	} else if s.leaderlessSince.IsZero() {
		s.leaderlessSince = time.Now()
	}

	if !hasPending {
		s.pendingSince = time.Time{}
	} else if s.pendingSince.IsZero() || status.Commit != s.pendingCommitIndex {
		// Proposals became pending, or the group is still committing entries:
		// (re)start the no-progress observation window.
		s.pendingSince = time.Now()
		s.pendingCommitIndex = status.Commit
	}

	var reason string
	switch {
	case !s.leaderlessSince.IsZero() && time.Since(s.leaderlessSince) >= s.forceTerminateDuration:
		reason = fmt.Sprintf("leaderless for %v", time.Since(s.leaderlessSince))
	case !s.pendingSince.IsZero() && time.Since(s.pendingSince) >= s.forcePendingDuration:
		reason = fmt.Sprintf("proposals pending without commit progress for %v (lead %d)",
			time.Since(s.pendingSince), status.Lead)
	default:
		return
	}

	fmt.Println(time.Now(), s.head.String(), "@@ force terminate", s.sectorKey.String(), reason)

	s.TerminateLocally()
}

// noteMergeConflictLocked records that a local PrepareMerge/PreCommitSplit
// attempt was rejected because mergeBy is held by another node. The
// observation window restarts when the holder changes. Call with s.mtx held
// for writing.
func (s *Sector) noteMergeConflictLocked(holder *types.NodeID) {
	if s.mergeConflictHolder != nil && s.mergeConflictHolder.Equal(holder) {
		return
	}
	s.mergeConflictHolder = holder.Copy()
	s.mergeConflictSince = time.Now()
}

// checkMergeRelease proposes ReleaseMerge when local merge/split attempts have
// been blocked by the same committed prepare_merge holder (mergeBy) for
// mergeReleaseDuration. mergeBy has no other release path, so a holder that
// died (or stalled) between prepare and commit_merge blocks every later
// merge/split on this sector forever (シミュレーション run 11; TLA+
// KvsSectorMergeLock Phase 1). The release goes through raft and its apply is
// a CAS on the recorded holder, so replicas stay deterministic and a release
// racing a newer PrepareMerge never clears the newer claim. A false positive
// (the holder is alive but slow) only restores the lock-free interleaving
// whose safety is model-checked (Phase 3): the resulting overlap is repaired
// by the existing Terminate paths. Runs on the Start loop goroutine.
func (s *Sector) checkMergeRelease() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.stopped || s.terminated || s.mergeConflictHolder == nil {
		return
	}
	// Resolved, or a different holder took over: restart the observation.
	if s.mergeBy == nil || !s.mergeBy.Equal(s.mergeConflictHolder) {
		s.mergeConflictHolder = nil
		return
	}
	if time.Since(s.mergeConflictSince) < s.mergeReleaseDuration {
		return
	}
	if s.proposalReleaseMerge == nil {
		fmt.Println(time.Now(), s.head.String(), "@@ propose release merge", s.sectorKey.String(),
			"held by", s.mergeConflictHolder.String(),
			"blocked for", time.Since(s.mergeConflictSince).String())
		s.proposalReleaseMerge = s.mergeConflictHolder.Copy()
	}
}

// TerminateLocally destroys the local replica without going through raft.
// It is the escape hatch for replicas that can no longer receive anything
// from their group: quorum-lost groups (checkQuorumLoss) and members that
// were removed from the group — a removed member cannot learn its own
// removal from the raft log (the leader stops messaging it once the removal
// applies), so KVS calls this when the host's out-of-band removal
// notification (SectorManageMember COMMAND_REMOVE) arrives. Idempotent; safe
// to call on an already stopped or terminated sector.
func (s *Sector) TerminateLocally() {
	s.mtx.Lock()
	var err error
	if !s.stopped && !s.terminated {
		err = s.terminateLocked()
	}
	s.mtx.Unlock()
	s.cond.Broadcast()

	if err != nil {
		s.handler.SectorError(&s.sectorKey, fmt.Errorf("failed to force-terminate sector: %w", err))
	}
}

func (s *Sector) processExtendProposal(extend *proto.Extend) error {
	newTail, err := types.NewNodeIDFromProto(extend.Tail)
	if err != nil {
		return fmt.Errorf("failed to parse tail NodeID: %w", err)
	}

	if s.proposalExtending != nil &&
		(s.proposalExtending.Equal(newTail) ||
			s.proposalExtending.IsBetween(&s.head, newTail)) {
		s.proposalExtending = nil
	}

	// The sector is already extended by other proposal, so ignore the proposal.
	if newTail.IsBetween(&s.head, s.tail) {
		return nil
	}
	s.tail = newTail

	if err := s.operator.SetRange(*newTail); err != nil {
		return fmt.Errorf("failed to set range for extend: %w", err)
	}

	return nil
}

func (s *Sector) processImportProposal(importProposal *proto.Import) error {
	records := make(map[string][]byte)
	for _, record := range importProposal.Records {
		records[record.Key] = record.Value
	}

	s.proposalImporting = nil

	// Tolerate an already-allocated store sector (repeated import proposals):
	// see processActivateProposal for why apply handlers must be idempotent.
	if err := s.store.AllocateSector(&s.sectorKey); err != nil {
		fmt.Println(time.Now(), s.head.String(), "@@ import: allocate sector skipped:", err)
	}
	if err := s.operator.ImportRecords(records); err != nil {
		return fmt.Errorf("failed to import records: %w", err)
	}

	return nil
}

func (s *Sector) processPreCommitSplitProposal(preCommitSplit *proto.PreCommitSplit) error {
	tail, err := types.NewNodeIDFromProto(preCommitSplit.Tail)
	if err != nil {
		return fmt.Errorf("failed to parse tail NodeID: %w", err)
	}

	s.proposalPreCommitSplitting = nil
	s.tail = tail

	if err := s.operator.SetRange(*tail); err != nil {
		return fmt.Errorf("failed to set range for pre-commit split: %w", err)
	}

	return nil
}

func (s *Sector) processCommitSplitProposal(commitSplit *proto.CommitSplit) error {
	newTail, err := types.NewNodeIDFromProto(commitSplit.Tail)
	if err != nil {
		return fmt.Errorf("failed to parse tail NodeID: %w", err)
	}

	// Idempotent no-op when already activated (e.g. a duplicated commit-split
	// entry): the pending proposal must be cleared here, otherwise the retry
	// loop re-proposes it every 3 seconds and each round trips
	// commit → apply-failure, poisoning the apply path forever.
	if s.tail != nil || s.proposalActivating != nil {
		s.proposalCommittingSplit = nil
		return nil
	}

	s.proposalCommittingSplit = nil
	s.tail = newTail

	if err := s.operator.SetRange(*newTail); err != nil {
		return fmt.Errorf("failed to set range for commit split: %w", err)
	}

	return nil
}

func (s *Sector) processPrepareMergeProposal(prepareMerge *proto.PrepareMerge) error {
	proposedBy, err := types.NewNodeIDFromProto(prepareMerge.Handler)
	if err != nil {
		return fmt.Errorf("failed to parse proposedBy NodeID: %w", err)
	}

	s.proposalPrepareMerge = nil
	// Accept the first prepare merge proposal, and reject the others.
	if s.mergeBy == nil {
		s.mergeBy = proposedBy
	}

	return nil
}

func (s *Sector) processReleaseMergeProposal(releaseMerge *proto.ReleaseMerge) error {
	holder, err := types.NewNodeIDFromProto(releaseMerge.Handler)
	if err != nil {
		return fmt.Errorf("failed to parse holder NodeID: %w", err)
	}

	s.proposalReleaseMerge = nil
	// CAS: clear only the recorded holder's claim, so a release racing a
	// newer PrepareMerge never clears the newer claim.
	if s.mergeBy != nil && s.mergeBy.Equal(holder) {
		fmt.Println(time.Now(), s.head.String(), "@@ release merge", s.sectorKey.String(),
			"held by", holder.String())
		s.mergeBy = nil
	}
	if s.mergeConflictHolder != nil && s.mergeConflictHolder.Equal(holder) {
		s.mergeConflictHolder = nil
	}

	return nil
}

func (s *Sector) processCommitMergeProposal(commitMerge *proto.CommitMerge) error {
	newTail, err := types.NewNodeIDFromProto(commitMerge.Tail)
	if err != nil {
		return fmt.Errorf("failed to parse newTail NodeID: %w", err)
	}

	s.proposalCommittingMerge = nil

	if newTail.IsBetween(&s.head, s.tail) {
		return nil
	}
	s.tail = newTail

	if err := s.operator.SetRange(*newTail); err != nil {
		return fmt.Errorf("failed to set range for commit merge: %w", err)
	}

	return nil
}

func (s *Sector) ConsensusAppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	s.mtx.Lock()
	delete(s.proposalAppendingNodes, sectorNo)
	s.mtx.Unlock()

	s.handler.SectorAppendNode(&s.sectorKey, sectorNo, nodeID)
}

func (s *Sector) ConsensusRemoveNode(sectorNo kvsTypes.SectorNo) {
	s.mtx.Lock()
	delete(s.proposalRemovingNodes, sectorNo)
	s.mtx.Unlock()

	s.handler.SectorRemoveNode(&s.sectorKey, sectorNo)
}

// ConsensusGetSnapshot assembles the sector-layer snapshot: exactly the
// replicated state that ConsensusApplyProposal mutates (records, tail,
// mergeBy, terminated) and none of the local intent (proposal* flags,
// splittingAddress) — each replica rebuilds those from its own role.
// See spec/kvs/snapshot.md.
//
// Runs on the consensus loop goroutine, serialized with the apply handlers;
// s.mtx is held across the record export so the payload is a consistent cut
// (applies mutate records while holding s.mtx for writing).
func (s *Sector) ConsensusGetSnapshot() ([]byte, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	records, err := s.operator.ExportAllRecords()
	if err != nil {
		return nil, err
	}

	snapshot := &proto.SectorSnapshot{
		Records:    make([]*proto.Import_Record, 0, len(records)),
		Terminated: s.terminated,
	}
	for key, value := range records {
		snapshot.Records = append(snapshot.Records, &proto.Import_Record{
			Key:   key,
			Value: value,
		})
	}
	if s.tail != nil {
		snapshot.Tail = s.tail.Proto()
	}
	if s.mergeBy != nil {
		snapshot.MergeBy = s.mergeBy.Proto()
	}

	return proto3.Marshal(snapshot)
}

// ConsensusApplySnapshot replaces the local sector state with the snapshot.
// Replacement, not merge: a replica that fell behind may hold keys the group
// has since deleted, so the store sector is reset before importing. Like the
// apply handlers this must be idempotent and, like them, it must not
// resurrect a stopped or terminated replica.
func (s *Sector) ConsensusApplySnapshot(data []byte) error {
	snapshot := &proto.SectorSnapshot{}
	if err := proto3.Unmarshal(data, snapshot); err != nil {
		return fmt.Errorf("failed to unmarshal sector snapshot: %w", err)
	}

	s.mtx.Lock()
	defer func() {
		s.mtx.Unlock()
		s.cond.Broadcast()
	}()

	if snapshot.Terminated {
		return s.terminateLocked()
	}
	if s.stopped || s.terminated {
		return nil
	}

	// Reset the store sector so ReplaceRecords starts from empty. Both calls
	// tolerate errors like the activate/import handlers do: ReleaseSector
	// fails when the sector was never allocated (an inactive replica), which
	// is the normal case for a fresh joiner.
	if err := s.store.ReleaseSector(&s.sectorKey); err != nil {
		fmt.Println(time.Now(), s.head.String(), "@@ apply snapshot: release sector skipped:", err)
	}
	if err := s.store.AllocateSector(&s.sectorKey); err != nil {
		fmt.Println(time.Now(), s.head.String(), "@@ apply snapshot: allocate sector skipped:", err)
	}

	// Clear the range before importing so the SetRange below starts from the
	// nil-range branch and never walks the delete loop against records that
	// are being replaced anyway.
	s.tail = nil
	s.operator.ClearRange()

	records := make(map[string][]byte, len(snapshot.Records))
	for _, record := range snapshot.Records {
		records[record.Key] = record.Value
	}
	if err := s.operator.ReplaceRecords(records); err != nil {
		return fmt.Errorf("failed to replace records from snapshot: %w", err)
	}

	if snapshot.Tail != nil {
		tail, err := types.NewNodeIDFromProto(snapshot.Tail)
		if err != nil {
			return fmt.Errorf("failed to parse tail NodeID in snapshot: %w", err)
		}
		s.tail = tail
		if err := s.operator.SetRange(*tail); err != nil {
			return fmt.Errorf("failed to set range from snapshot: %w", err)
		}
	}

	if snapshot.MergeBy != nil {
		mergeBy, err := types.NewNodeIDFromProto(snapshot.MergeBy)
		if err != nil {
			return fmt.Errorf("failed to parse mergeBy NodeID in snapshot: %w", err)
		}
		s.mergeBy = mergeBy
	} else {
		s.mergeBy = nil
	}

	return nil
}

func (s *Sector) OperatorProposeOperation(operation *proto.Operation) {
	s.consensus.Propose(&proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Operation{
			Operation: operation,
		},
	})
}
