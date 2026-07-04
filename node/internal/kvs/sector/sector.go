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
	// leaderlessSince is touched only by the Start loop goroutine.
	leaderlessSince time.Time
	triggerCh       chan struct{}
	stopCtx         context.CancelFunc

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

	return s.proposalActivating != nil ||
		s.proposalExtending != nil ||
		s.proposalImporting != nil ||
		s.proposalPreCommitSplitting != nil ||
		s.proposalCommittingSplit != nil ||
		s.proposalPrepareMerge != nil ||
		s.proposalCommittingMerge != nil
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
	s.proposalPreCommitSplitting = frontwardNodeID
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	if err := s.waitProposal(func() (bool, error) {
		if s.mergeBy != nil {
			return false, fmt.Errorf("cannot pre-commit split while merge is being prepared by %s", s.mergeBy.String())
		}
		return s.proposalPreCommitSplitting == nil, nil
	}); err != nil {
		if errors.Is(err, ErrProposalTimeout) || errors.Is(err, ErrSectorStopped) {
			s.mtx.Lock()
			s.proposalPreCommitSplitting = nil
			s.mtx.Unlock()
		}
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
	s.proposalPrepareMerge = proposedBy
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	if err := s.waitProposal(func() (bool, error) {
		if s.mergeBy != nil {
			if !s.mergeBy.Equal(proposedBy) {
				return false, fmt.Errorf("merge is prepared by %s, not %s", s.mergeBy.String(), proposedBy.String())
			}
			return true, nil
		}
		return false, nil
	}); err != nil {
		if errors.Is(err, ErrProposalTimeout) || errors.Is(err, ErrSectorStopped) {
			s.mtx.Lock()
			s.proposalPrepareMerge = nil
			s.mtx.Unlock()
		}
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
			fmt.Println(s.head.String(), "## retry proposals", s.sectorKey.String(),
				"pending", strings.Join(pending, ","),
				"state", status.RaftState.String(), "lead", status.Lead, "term", status.Term)
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
	if s.tail == nil { // Not activated yet.
		return nil
	}

	if extend := proposal.GetExtend(); extend != nil {
		return s.processExtendProposal(extend)

	} else if importProposal := proposal.GetImport(); importProposal != nil {
		return s.processImportProposal(importProposal)

	} else if preCommitSplit := proposal.GetPreCommitSplit(); preCommitSplit != nil {
		return s.processPreCommitSplitProposal(preCommitSplit)

	} else if commitSplit := proposal.GetCommitSplit(); commitSplit != nil {
		return s.processCommitSplitProposal(commitSplit)

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

	if err := s.store.AllocateSector(&s.sectorKey); err != nil {
		return fmt.Errorf("failed to allocate sector: %w", err)
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
func (s *Sector) terminateLocked() error {
	s.operator.ClearRange()
	if err := s.store.ReleaseSector(&s.sectorKey); err != nil {
		return err
	}

	s.proposalActivating = nil
	s.terminated = true
	s.stopped = true

	go s.handler.SectorTerminated(&s.sectorKey)
	return nil
}

// checkQuorumLoss destroys the local replica without raft when the group has
// been leaderless for forceTerminateDuration (TLA+ LocalDestroy). A group that
// lost its quorum can never commit anything — including Terminate — so this is
// the only escape path; without it a stale replica blocks the activation chain
// forever. If the group is actually alive (e.g. the local node is only
// partitioned), destroying the replica is equivalent to this member leaving,
// and the resulting overlap is repaired by the existing Terminate/Merge paths.
// Runs on the Start loop goroutine only (leaderlessSince is not locked).
func (s *Sector) checkQuorumLoss() {
	s.mtx.RLock()
	inactive := s.stopped || s.terminated
	s.mtx.RUnlock()
	if inactive {
		return
	}

	if s.consensus.Status().Lead != raft.None {
		s.leaderlessSince = time.Time{}
		return
	}
	if s.leaderlessSince.IsZero() {
		s.leaderlessSince = time.Now()
		return
	}
	if time.Since(s.leaderlessSince) < s.forceTerminateDuration {
		return
	}

	fmt.Println(time.Now(), s.head.String(), "## force terminate", s.sectorKey.String(),
		"leaderless for", time.Since(s.leaderlessSince))

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

	if err := s.store.AllocateSector(&s.sectorKey); err != nil {
		return fmt.Errorf("failed to allocate sector: %w", err)
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

	if s.tail != nil || s.proposalActivating != nil {
		return fmt.Errorf("sector is already activated")
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

func (s *Sector) ConsensusGetSnapshot() ([]byte, error) {
	return s.operator.ExportSnapshot()
}

func (s *Sector) ConsensusApplySnapshot(snapshot []byte) error {
	return s.operator.ImportSnapshot(snapshot)
}

func (s *Sector) OperatorProposeOperation(operation *proto.Operation) {
	s.consensus.Propose(&proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Operation{
			Operation: operation,
		},
	})
}
