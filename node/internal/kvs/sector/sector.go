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
	"log/slog"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/operator"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"go.etcd.io/raft/v3"
)

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
	triggerCh             chan struct{}
	stopCtx               context.CancelFunc

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

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.stopCtx = cancel
		defer cancel()
		timer := time.NewTimer(s.proposalRetryDuration)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-timer.C:
				s.applyProposals()

			case <-s.triggerCh:
				s.applyProposals()
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

func (s *Sector) Extend(newTail types.NodeID) {
	if !s.isHosting {
		panic("only host sector can be extended")
	}

	s.mtx.Lock()

	if newTail.IsBetween(&s.head, s.tail) {
		s.mtx.Unlock()
		return
	}
	s.proposalExtending = &newTail
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	for {
		if s.proposalExtending == nil || s.stopped {
			break
		}
		s.cond.Wait()
	}
	return
}

func (s *Sector) Migrate(to *Sector) error {
	splittingAddress := to.GetHeadAddress()
	s.operator.SetSplitting(splittingAddress)

	records, err := s.operator.ExportRecords(splittingAddress, s.GetTailAddress())
	if err != nil {
		return fmt.Errorf("failed to export records: %w", err)
	}

	to.Import(records)
	return nil
}

func (s *Sector) PreCommitSplit(frontwardNodeID *types.NodeID) error {
	if !s.isHosting {
		panic("only host sector can be extended")
	}

	s.mtx.Lock()
	s.proposalPreCommitSplitting = frontwardNodeID
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	for {
		if s.mergeBy != nil {
			return fmt.Errorf("cannot pre-commit split while merge is being prepared by %s", s.mergeBy.String())
		}
		if s.proposalPreCommitSplitting == nil || s.stopped {
			break
		}
		s.cond.Wait()
	}
	return nil
}

func (s *Sector) CommitSplit(newTail *types.NodeID) error {
	s.mtx.Lock()
	s.proposalCommittingSplit = newTail
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	for {
		if s.proposalCommittingSplit == nil || s.stopped {
			break
		}
		s.cond.Wait()
	}
	return nil
}

func (s *Sector) PrepareMerge(proposedBy *types.NodeID) error {
	s.mtx.Lock()
	s.proposalPrepareMerge = proposedBy
	s.mtx.Unlock()

	s.triggerCh <- struct{}{}

	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	for {
		if s.mergeBy != nil {
			if !s.mergeBy.Equal(proposedBy) {
				return fmt.Errorf("merge is prepared by %s, not %s", s.mergeBy.String(), proposedBy.String())
			}
			break
		}
		if s.stopped {
			break
		}
		s.cond.Wait()
	}
	return nil
}

func (s *Sector) Merge(from *Sector) error {
	records, err := from.operator.ExportRecords(from.GetHeadAddress(), from.GetTailAddress())
	if err != nil {
		return fmt.Errorf("failed to export records: %w", err)
	}

	s.Import(records)
	return nil
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

	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	for {
		if s.proposalCommittingMerge == nil || s.stopped {
			break
		}
		s.cond.Wait()
	}
	return nil
}

func (s *Sector) Import(records map[string][]byte) {
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

	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	for {
		if s.proposalImporting == nil || s.stopped {
			break
		}
		s.cond.Wait()
	}
}

func (s *Sector) applyProposals() {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	// re-apply terminating
	if s.proposalTerminating {
		if !s.terminated {
			s.consensus.Propose(&proto.ConsensusProposal{
				Content: &proto.ConsensusProposal_Terminate{
					Terminate: &proto.Terminate{},
				},
			})
		}
		return
	}

	if s.terminated {
		return
	}

	// Apply appending nodes.
	for sectorNo, nodeID := range s.proposalAppendingNodes {
		s.consensus.AppendNode(sectorNo, nodeID)
	}

	// Apply removing nodes.
	for sectorNo := range s.proposalRemovingNodes {
		s.consensus.RemoveNode(sectorNo)
	}

	// Apply activating.
	if s.proposalActivating != nil {
		s.consensus.Propose(&proto.ConsensusProposal{
			Content: &proto.ConsensusProposal_Activate{
				Activate: &proto.Activate{
					Tail: s.proposalActivating.Proto(),
				},
			},
		})
	}

	// Apply extending
	if s.proposalExtending != nil {
		s.consensus.Propose(&proto.ConsensusProposal{
			Content: &proto.ConsensusProposal_Extend{
				Extend: &proto.Extend{
					Tail: s.proposalExtending.Proto(),
				},
			},
		})
	}

	// Apply importing
	if s.proposalImporting != nil {
		s.consensus.Propose(&proto.ConsensusProposal{
			Content: &proto.ConsensusProposal_Import{
				Import: &proto.Import{
					Records: s.proposalImporting,
				},
			},
		})
	}

	// Apply pre-commit splitting.
	if s.proposalPreCommitSplitting != nil {
		s.consensus.Propose(&proto.ConsensusProposal{
			Content: &proto.ConsensusProposal_PreCommitSplit{
				PreCommitSplit: &proto.PreCommitSplit{
					Tail: s.proposalPreCommitSplitting.Proto(),
				},
			},
		})
	}

	// Apply committing split.
	if s.proposalCommittingSplit != nil {
		s.consensus.Propose(&proto.ConsensusProposal{
			Content: &proto.ConsensusProposal_CommitSplit{
				CommitSplit: &proto.CommitSplit{
					Tail: s.proposalCommittingSplit.Proto(),
				},
			},
		})
	}

	// Apply prepare merge.
	if s.proposalPrepareMerge != nil {
		s.consensus.Propose(&proto.ConsensusProposal{
			Content: &proto.ConsensusProposal_PrepareMerge{
				PrepareMerge: &proto.PrepareMerge{
					Handler: s.proposalPrepareMerge.Proto(),
				},
			},
		})
	}

	// Apply committing merge.
	if s.proposalCommittingMerge != nil {
		s.consensus.Propose(&proto.ConsensusProposal{
			Content: &proto.ConsensusProposal_CommitMerge{
				CommitMerge: &proto.CommitMerge{
					Tail: s.proposalCommittingMerge.Proto(),
				},
			},
		})
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
		if err := s.processActivateProposal(activate); err != nil {
			return err
		}
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
	s.operator.ClearRange()
	if err := s.store.ReleaseSector(&s.sectorKey); err != nil {
		return err
	}

	s.proposalActivating = nil
	s.terminated = true
	s.stopped = true

	s.handler.SectorTerminated(&s.sectorKey)
	return nil
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
