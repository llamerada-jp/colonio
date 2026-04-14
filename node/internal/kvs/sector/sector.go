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
}

type SectorConfig struct {
	Logger     *slog.Logger
	RaftLogger raft.Logger
	Handler    SectorHandler
	Outbound   consensus.OutboundPort
	SectorKey  *kvsTypes.SectorKey
	IsHosting  bool
	Join       bool
	Members    map[kvsTypes.SectorNo]*types.NodeID
	Store      kvsTypes.Store
	Head       *types.NodeID
}

type SectorInfo struct {
	Head *types.NodeID
	Tail *types.NodeID
}

type Sector struct {
	handler   SectorHandler
	sectorKey kvsTypes.SectorKey
	isHosting bool
	consensus *consensus.Consensus
	operator  *operator.Operator
	mtx       sync.RWMutex
	head      types.NodeID
	tail      *types.NodeID
}

func NewSector(config *SectorConfig) *Sector {
	sector := &Sector{
		handler:   config.Handler,
		sectorKey: *config.SectorKey,
		isHosting: config.IsHosting,
		head:      *config.Head,
	}

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
}

func (s *Sector) Stop() {
	s.consensus.Stop()
}

func (s *Sector) ProcessConsensusMessage(message *proto.ConsensusMessage) error {
	return s.consensus.ProcessMessage(message)
}

func (s *Sector) AppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	if !s.isHosting {
		panic("only host sector can append node")
	}
	s.consensus.AppendNode(sectorNo, nodeID)
}

func (s *Sector) RemoveNode(sectorNo kvsTypes.SectorNo) {
	if !s.isHosting {
		panic("only host sector can remove node")
	}
	s.consensus.RemoveNode(sectorNo)
}

func (s *Sector) GetTailAddress() *types.NodeID {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.tail
}

func (s *Sector) Activate(tail types.NodeID) {
	if !s.isHosting {
		panic("only host sector can be activated")
	}

	s.consensus.Propose(&proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Activate{
			Activate: &proto.Activate{
				Tail: tail.Proto(),
			},
		},
	})
}

func (s *Sector) GetOperator() operator.Operations {
	return s.operator
}

func (s *Sector) GetInfo() SectorInfo {
	head, tail := s.operator.GetRange()
	return SectorInfo{
		Head: &head,
		Tail: tail,
	}
}

func (s *Sector) ConsensusError(err error) {
	s.handler.SectorError(&s.sectorKey, err)
}

func (s *Sector) ConsensusApplyProposal(proposal *proto.ConsensusProposal) {
	if operation := proposal.GetOperation(); operation != nil {
		s.operator.ApplyProposal(operation)

	} else if activate := proposal.GetActivate(); activate != nil {
		s.processActivateProposal(activate)

	} else {
		s.handler.SectorError(&s.sectorKey, fmt.Errorf("invalid proposal content"))
	}
}

func (s *Sector) processActivateProposal(activate *proto.Activate) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// set tail when the sector is activated for the first time
	if s.tail != nil {
		return
	}

	tail, err := types.NewNodeIDFromProto(activate.Tail)
	if err != nil {
		s.handler.SectorError(&s.sectorKey, fmt.Errorf("failed to parse tail NodeID: %w", err))
		return
	}

	s.tail = tail
	s.operator.SetRange(*tail)
}

func (s *Sector) ConsensusAppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	s.handler.SectorAppendNode(&s.sectorKey, sectorNo, nodeID)
}

func (s *Sector) ConsensusRemoveNode(sectorNo kvsTypes.SectorNo) {
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
