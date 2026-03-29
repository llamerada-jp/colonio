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
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/shared"
	"go.etcd.io/raft/v3"
)

type SectorHandler interface {
	SectorError(sectorKey *config.KvsSectorKey, err error)
	SectorAppendNode(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo, nodeID *shared.NodeID)
	SectorRemoveNode(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo)
}

type SectorConfig struct {
	Logger         *slog.Logger
	RaftLogger     raft.Logger
	Handler        SectorHandler
	Infrastructure ConsensusInfrastructure
	SectorKey      *config.KvsSectorKey
	IsHosting      bool
	Join           bool
	Members        map[config.SectorNo]*shared.NodeID
	Store          config.KvsStore
	Head           *shared.NodeID
}

type SectorInfo struct {
	Head *shared.NodeID
	Tail *shared.NodeID
}

type Sector struct {
	handler   SectorHandler
	sectorKey config.KvsSectorKey
	isHosting bool
	consensus *consensus
	operator  *operator
	mtx       sync.RWMutex
	head      shared.NodeID
	tail      *shared.NodeID
}

func NewSector(config *SectorConfig) *Sector {
	sector := &Sector{
		handler:   config.Handler,
		sectorKey: *config.SectorKey,
		isHosting: config.IsHosting,
		head:      *config.Head,
	}

	sector.consensus = newConsensus(&consensusConfig{
		logger:         config.Logger,
		raftLogger:     config.RaftLogger,
		handler:        sector,
		infrastructure: config.Infrastructure,
		sectorKey:      config.SectorKey,
		join:           config.Join,
		members:        config.Members,
	})

	sector.operator = newOperator(&operatorConfig{
		sectorKey: config.SectorKey,
		handler:   sector,
		head:      config.Head,
	})

	return sector
}

func (s *Sector) Start(ctx context.Context) {
	s.consensus.start(ctx)
}

func (s *Sector) Stop() {
	s.consensus.stop()
}

func (s *Sector) ProcessConsensusMessage(message *proto.ConsensusMessage) error {
	return s.consensus.processMessage(message)
}

func (s *Sector) AppendNode(sectorNo config.SectorNo, nodeID *shared.NodeID) {
	if !s.isHosting {
		panic("only host sector can append node")
	}
	s.consensus.appendNode(sectorNo, nodeID)
}

func (s *Sector) RemoveNode(sectorNo config.SectorNo) {
	if !s.isHosting {
		panic("only host sector can remove node")
	}
	s.consensus.removeNode(sectorNo)
}

func (s *Sector) GetTail() *shared.NodeID {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.tail
}

func (s *Sector) Activate(tail shared.NodeID) {
	if !s.isHosting {
		panic("only host sector can be activated")
	}

	s.consensus.propose(&proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Activate{
			Activate: &proto.Activate{
				Tail: tail.Proto(),
			},
		},
	})
}

func (s *Sector) GetOperator() Operations {
	return s.operator
}

func (s *Sector) GetInfo() SectorInfo {
	return SectorInfo{
		Head: &s.operator.head,
		Tail: s.operator.tail,
	}
}

func (s *Sector) consensusError(err error) {
	s.handler.SectorError(&s.sectorKey, err)
}

func (s *Sector) consensusApplyProposal(proposal *proto.ConsensusProposal) {
	if operation := proposal.GetOperation(); operation != nil {
		s.operator.applyProposal(operation)

	} else if activate := proposal.GetActivate(); activate != nil {
		s.processActivateProposal(activate)

	} else {
		s.consensusError(fmt.Errorf("invalid proposal content"))
	}
}

func (s *Sector) processActivateProposal(activate *proto.Activate) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// set tail when the sector is activated for the first time
	if s.tail != nil {
		return
	}

	tail, err := shared.NewNodeIDFromProto(activate.Tail)
	if err != nil {
		s.consensusError(fmt.Errorf("failed to parse tail NodeID: %w", err))
		return
	}

	s.tail = tail
	s.operator.setRange(*tail)
}

func (s *Sector) consensusAppendNode(sectorNo config.SectorNo, nodeID *shared.NodeID) {
	s.handler.SectorAppendNode(&s.sectorKey, sectorNo, nodeID)
}

func (s *Sector) consensusRemoveNode(sectorNo config.SectorNo) {
	s.handler.SectorRemoveNode(&s.sectorKey, sectorNo)
}

func (s *Sector) consensusGetSnapshot() ([]byte, error) {
	return s.operator.exportSnapshot()
}

func (s *Sector) consensusApplySnapshot(snapshot []byte) error {
	return s.operator.importSnapshot(snapshot)
}

func (s *Sector) operatorProposeOperation(operation *proto.Operation) {
	s.consensus.propose(&proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Operation{
			Operation: operation,
		},
	})
}
