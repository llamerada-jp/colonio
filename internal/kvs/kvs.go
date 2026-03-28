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
package kvs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/kvs/sector"
	"github.com/llamerada-jp/colonio/internal/shared"
	"go.etcd.io/raft/v3"
)

type memberState int

const (
	memberStateNormal              memberState = iota
	memberStateCreating                        // Creating a new Sector cluster with initial members
	memberStateAppending                       // Appending a new member to the existing Sector
	memberStateConfiguredNode                  // Configured node member but not yet joined to the Sector
	memberStateConfiguredConsensus             // Get consensus of Sector but not configured to the node
	memberStateRemoving
)

type Handler interface {
	KvsGetStability() (bool, []*shared.NodeID)
	KvsState(state constants.KvsState) (constants.KvsState, error)
}

type Config struct {
	Logger                  *slog.Logger
	EnableRaftLogging       bool
	Handler                 Handler
	KvsInfrastructure       KvsInfrastructure
	ConsensusInfrastructure sector.ConsensusInfrastructure
	Observation             config.ObservationCaller
	Store                   config.KvsStore
}

type memberStateEntry struct {
	nodeID *shared.NodeID
	state  memberState
}

type KVS struct {
	logger                  *slog.Logger
	raftLogger              raft.Logger
	ctx                     context.Context
	handler                 Handler
	infrastructure          KvsInfrastructure
	consensusInfrastructure sector.ConsensusInfrastructure
	observation             config.ObservationCaller
	store                   config.KvsStore
	localNodeID             *shared.NodeID
	mtx                     sync.RWMutex // for sectors, hostingSector, memberStates
	sectors                 map[config.KvsSectorKey]*sector.Sector
	sectorUpdated           bool // for observation
	hostingSector           *config.KvsSectorKey
	lastSectorNo            config.SectorNo
	memberStates            map[config.SectorNo]*memberStateEntry
}

func NewKVS(conf *Config) *KVS {
	k := &KVS{
		logger:                  conf.Logger,
		handler:                 conf.Handler,
		infrastructure:          conf.KvsInfrastructure,
		consensusInfrastructure: conf.ConsensusInfrastructure,
		observation:             conf.Observation,
		store:                   conf.Store,
		sectors:                 make(map[config.KvsSectorKey]*sector.Sector),
		memberStates:            make(map[config.SectorNo]*memberStateEntry),
	}

	if conf.EnableRaftLogging {
		k.raftLogger = newSlogWrapper(conf.Logger)
	} else {
		k.raftLogger = newEmptyLogger()
	}

	return k
}

func (k *KVS) Start(ctx context.Context, localNodeID *shared.NodeID) {
	k.localNodeID = localNodeID
	k.ctx = ctx

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				k.subRoutine()
				if k.observation != nil {
					k.takeObservation()
				}
			}
		}
	}()
}

func (k *KVS) Get(key string) chan *config.KvsGetResult {
	c := make(chan *config.KvsGetResult, 1)
	k.infrastructure.sendKvsOperation(&operationParam{
		command: proto.KvsOperation_COMMAND_GET,
		key:     key,
		value:   nil,
		receiver: func(res *proto.KvsOperationResponse, err error) {
			defer close(c)

			if err != nil {
				c <- &config.KvsGetResult{
					Data: nil,
					Err:  err,
				}
				return
			}

			switch res.Error {
			case proto.KvsOperationResponse_ERROR_NONE:
				// ok
				c <- &config.KvsGetResult{
					Data: res.Value,
					Err:  nil,
				}

			case proto.KvsOperationResponse_ERROR_NOT_FOUND:
				c <- &config.KvsGetResult{
					Data: nil,
					Err:  fmt.Errorf("key not found: %s", key),
				}

			default:
				c <- &config.KvsGetResult{
					Data: nil,
					Err:  fmt.Errorf("unknown error: %d", res.Error),
				}
			}
		},
	})

	return c
}

func (k *KVS) Set(key string, value []byte) chan error {
	c := make(chan error, 1)
	k.infrastructure.sendKvsOperation(&operationParam{
		command: proto.KvsOperation_COMMAND_SET,
		key:     key,
		value:   value,
		receiver: func(res *proto.KvsOperationResponse, err error) {
			defer close(c)

			if err != nil {
				c <- err
				return
			}

			if res.Error != proto.KvsOperationResponse_ERROR_NONE {
				c <- fmt.Errorf("kvs set error: %d", res.Error)
				return
			}

			c <- nil
		},
	})

	return c
}

func (k *KVS) Patch(key string, value []byte) chan error {
	c := make(chan error, 1)
	k.infrastructure.sendKvsOperation(&operationParam{
		command: proto.KvsOperation_COMMAND_PATCH,
		key:     key,
		value:   value,
		receiver: func(res *proto.KvsOperationResponse, err error) {
			defer close(c)

			if err != nil {
				c <- err
				return
			}

			if res.Error != proto.KvsOperationResponse_ERROR_NONE {
				c <- fmt.Errorf("kvs patch error: %d", res.Error)
				return
			}

			c <- nil
		},
	})

	return c
}

func (k *KVS) Delete(key string) chan error {
	c := make(chan error, 1)
	k.infrastructure.sendKvsOperation(&operationParam{
		command: proto.KvsOperation_COMMAND_DELETE,
		key:     key,
		value:   nil,
		receiver: func(res *proto.KvsOperationResponse, err error) {
			defer close(c)

			if err != nil {
				c <- err
				return
			}

			if res.Error != proto.KvsOperationResponse_ERROR_NONE {
				c <- fmt.Errorf("kvs delete error: %d", res.Error)
				return
			}

			c <- nil
		},
	})

	return c
}

func (k *KVS) subRoutine() {
	isStable, nextNodeIDs := k.handler.KvsGetStability()
	if !isStable {
		return
	}

	k.mtx.Lock()
	defer k.mtx.Unlock()

	if k.hostingSector == nil {
		k.hostSector(nextNodeIDs)
	}

	toAppend, toRemove := k.getNodesToBeChanged(nextNodeIDs)
	for _, nodeID := range toAppend {
		k.lastSectorNo++
		k.memberStates[k.lastSectorNo] = &memberStateEntry{
			nodeID: nodeID,
			state:  memberStateAppending,
		}
	}
	for sec := range toRemove {
		k.memberStates[sec].state = memberStateRemoving
	}

	if err := k.applyMemberSectors(); err != nil {
		k.logger.Warn("Failed to apply Raft config", "error", err)
	}

	k.sendSettingMessage()
}

func (k *KVS) getNodesToBeChanged(nextNodeIDs []*shared.NodeID) ([]*shared.NodeID, map[config.SectorNo]struct{}) {
	nextNodeIDMap := make(map[shared.NodeID]struct{})
	for _, nodeID := range nextNodeIDs {
		nextNodeIDMap[*nodeID] = struct{}{}
	}

	toAppend := make([]*shared.NodeID, 0)
	memberMap := make(map[shared.NodeID]config.SectorNo)
	for sec, entry := range k.memberStates {
		if entry.state == memberStateRemoving {
			continue
		}
		memberMap[*entry.nodeID] = sec
	}
	for nodeID := range nextNodeIDMap {
		if _, ok := memberMap[nodeID]; !ok {
			toAppend = append(toAppend, &nodeID)
		}
	}

	toRemove := make(map[config.SectorNo]struct{})
	for sec, entry := range k.memberStates {
		if entry.state == memberStateRemoving || sec == config.KvsHostNodeSectorNo {
			continue
		}
		if _, ok := nextNodeIDMap[*entry.nodeID]; !ok {
			toRemove[sec] = struct{}{}
		}
	}
	return toAppend, toRemove
}

func (k *KVS) hostSector(nextNodeIDs []*shared.NodeID) {
	sectorID, err := uuid.NewV7()
	if err != nil {
		panic("Failed to create new sector ID")
	}

	k.lastSectorNo = config.KvsHostNodeSectorNo
	members := make(map[config.SectorNo]*shared.NodeID)
	members[config.KvsHostNodeSectorNo] = k.localNodeID
	k.memberStates[config.KvsHostNodeSectorNo] = &memberStateEntry{
		nodeID: k.localNodeID,
		state:  memberStateCreating,
	}
	for _, nodeID := range nextNodeIDs {
		if nodeID.Equal(k.localNodeID) {
			panic("localNodeID found in nextNodeIDs")
		}
		k.lastSectorNo++
		members[k.lastSectorNo] = nodeID
		k.memberStates[k.lastSectorNo] = &memberStateEntry{
			nodeID: nodeID,
			state:  memberStateCreating,
		}
	}

	sectorKey := &config.KvsSectorKey{
		SectorID: config.SectorID(sectorID),
		SectorNo: config.KvsHostNodeSectorNo,
	}
	k.hostingSector = sectorKey
	k.allocateSector(sectorKey, k.localNodeID, true, false, members)
}

func (k *KVS) allocateSector(
	sectorKey *config.KvsSectorKey,
	head *shared.NodeID,
	isHosting bool,
	append bool,
	members map[config.SectorNo]*shared.NodeID,
) {
	s := sector.NewSector(&sector.SectorConfig{
		Logger:         k.logger,
		RaftLogger:     k.raftLogger,
		Handler:        k,
		Infrastructure: k.consensusInfrastructure,
		SectorKey:      sectorKey,
		IsHosting:      isHosting,
		Join:           append,
		Members:        members,
		Store:          k.store,
		Head:           head,
	})

	k.sectors[*sectorKey] = s
	k.sectorUpdated = true

	s.Start(k.ctx)
}

func (k *KVS) applyMemberSectors() error {
	sector := k.sectors[*k.hostingSector]

	for sectorNo, member := range k.memberStates {
		switch member.state {
		case memberStateAppending:
			sector.AppendNode(sectorNo, member.nodeID)

		case memberStateRemoving:
			sector.RemoveNode(sectorNo)
		}
	}

	return nil
}

func (k *KVS) sendSettingMessage() {
	for sectorNo, ms := range k.memberStates {
		if sectorNo == config.KvsHostNodeSectorNo {
			continue
		}

		switch ms.state {
		case memberStateNormal, memberStateRemoving, memberStateConfiguredNode:
			continue

		case memberStateCreating, memberStateAppending, memberStateConfiguredConsensus:
			members := make(map[config.SectorNo]*shared.NodeID)
			members[config.KvsHostNodeSectorNo] = k.localNodeID
			for sn, ms := range k.memberStates {
				members[sn] = ms.nodeID
			}

			var command proto.SectorConfig_Command
			if ms.state == memberStateCreating {
				command = proto.SectorConfig_COMMAND_CREATE
			} else { // memberStateAppending
				command = proto.SectorConfig_COMMAND_APPEND
			}

			k.infrastructure.sendSectorConfig(&sectorConfigParam{
				dstNodeID: ms.nodeID,
				sectorID:  k.hostingSector.SectorID,
				sectorNo:  sectorNo,
				command:   command,
				members:   members,
			})
		}
	}
}

func (k *KVS) kvsOperate(command proto.KvsOperation_Command, key string, value []byte) (proto.KvsOperationResponse_Error, []byte) {
	k.mtx.RLock()
	if k.hostingSector == nil {
		k.mtx.RUnlock()
		k.logger.Debug("preparing hosting sector")
		return proto.KvsOperationResponse_ERROR_PREPARING, nil
	}

	operator := k.sectors[*k.hostingSector].GetOperator()
	k.mtx.RUnlock()

	switch command {
	case proto.KvsOperation_COMMAND_GET:
		data, err := operator.Get(key)

		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			if err == config.ErrorKvsStoreKeyNotFound {
				e = proto.KvsOperationResponse_ERROR_NOT_FOUND
			} else {
				e = proto.KvsOperationResponse_ERROR_UNKNOWN
			}
		}
		return e, data

	case proto.KvsOperation_COMMAND_SET:
		err := operator.Set(key, value)
		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			e = proto.KvsOperationResponse_ERROR_UNKNOWN
		}
		return e, nil

	case proto.KvsOperation_COMMAND_PATCH:
		err := operator.Patch(key, value)
		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			e = proto.KvsOperationResponse_ERROR_UNKNOWN
		}
		return e, nil

	case proto.KvsOperation_COMMAND_DELETE:
		err := operator.Delete(key)
		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			e = proto.KvsOperationResponse_ERROR_UNKNOWN
		}
		return e, nil

	default:
		return proto.KvsOperationResponse_ERROR_UNKNOWN, nil
	}
}

type SectorConfigureParam struct {
	command   proto.SectorConfig_Command
	sectorKey config.KvsSectorKey
	head      *shared.NodeID
	members   map[config.SectorNo]*shared.NodeID
}

func (k *KVS) SectorConfigure(param *SectorConfigureParam) error {
	k.mtx.Lock()
	defer k.mtx.Unlock()

	switch param.command {
	case proto.SectorConfig_COMMAND_CREATE, proto.SectorConfig_COMMAND_APPEND:
		if _, ok := k.sectors[param.sectorKey]; !ok {
			append := true
			if param.command == proto.SectorConfig_COMMAND_CREATE {
				append = false
			}
			k.allocateSector(&param.sectorKey, param.head, false, append, param.members)
		}

	default:
		return fmt.Errorf("unknown SectorConfig command: %d", param.command)
	}

	return nil
}

func (k *KVS) SectorConfigureResponse(srcNodeID *shared.NodeID, sectorID config.SectorID, sectorNo config.SectorNo) {
	k.mtx.Lock()
	defer k.mtx.Unlock()

	// ignore response if it does not match the current node
	if k.hostingSector == nil || k.hostingSector.SectorID != sectorID {
		return
	}

	if entry, ok := k.memberStates[sectorNo]; ok && entry.nodeID.Equal(srcNodeID) {
		switch entry.state {
		case memberStateCreating, memberStateAppending:
			entry.state = memberStateConfiguredNode
		case memberStateConfiguredConsensus:
			entry.state = memberStateNormal
		case memberStateNormal, memberStateConfiguredNode:
			// do nothing
		default:
			k.logger.Warn("Unexpected state for Raft config response", "sectorNo", sectorNo, "state", entry.state, "nodeID", srcNodeID.String())
		}
	}
}

func (k *KVS) processConsensusMessage(key config.KvsSectorKey, message *proto.ConsensusMessage) {
	k.mtx.RLock()
	sector, ok := k.sectors[key]
	k.mtx.RUnlock()
	if !ok {
		return
	}

	if err := sector.ProcessConsensusMessage(message); err != nil {
		k.logger.Error("Failed to process Raft message", "error", err)
	}
}

func (k *KVS) SectorError(sectorKey *config.KvsSectorKey, err error) {
	panic(fmt.Sprintf("raftNodeError not implemented: %s", err))
}

func (k *KVS) SectorManage(sectorKey *config.KvsSectorKey, proposal *proto.Management) {
	_, ok := k.sectors[*sectorKey]
	if !ok {
		return
	}

	panic("raftNodeApplyProposal not implemented")
}

func (k *KVS) SectorAppendNode(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo, nodeID *shared.NodeID) {
	if k.hostingSector == nil || *sectorKey != *k.hostingSector {
		return
	}

	k.mtx.Lock()
	defer k.mtx.Unlock()

	member, ok := k.memberStates[sectorNo]
	// unknown
	if !ok {
		k.logger.Warn("Unknown sectorNo for appending", "sectorNo", sectorNo, "nodeID", nodeID)
		k.memberStates[sectorNo] = &memberStateEntry{
			nodeID: nodeID,
			state:  memberStateRemoving,
		}
		return
	}

	if sectorNo == config.KvsHostNodeSectorNo {
		if member.state == memberStateCreating {
			member.state = memberStateNormal
		} else {
			k.logger.Warn("Unexpected state for host node", "sectorNo", sectorNo, "state", member.state, "nodeID", nodeID)
		}
		return
	}

	switch member.state {
	case memberStateCreating, memberStateAppending:
		member.state = memberStateConfiguredConsensus
	case memberStateConfiguredNode:
		member.state = memberStateNormal
	default:
		k.logger.Warn("Unexpected state for appending node", "sectorNo", sectorNo, "state", member.state, "nodeID", nodeID)
	}
}

func (k *KVS) SectorRemoveNode(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo) {
	k.mtx.Lock()
	defer k.mtx.Unlock()
	targetKey := config.KvsSectorKey{
		SectorID: sectorKey.SectorID,
		SectorNo: sectorNo,
	}
	sector, ok := k.sectors[targetKey]
	if !ok {
		return
	}

	if k.hostingSector != nil && *sectorKey == *k.hostingSector {
		delete(k.memberStates, sectorNo)
	}

	delete(k.sectors, targetKey)

	go sector.Stop()
}

func (k *KVS) takeObservation() {
	k.mtx.RLock()
	defer k.mtx.RUnlock()
	if !k.sectorUpdated {
		return
	}

	sectorInfos := make(map[config.KvsSectorKey]*config.ObservationSectorInfo)
	for sectorKey, sector := range k.sectors {
		sectorInfo := sector.GetInfo()
		tail := ""
		if sectorInfo.Tail != nil {
			tail = sectorInfo.Tail.String()
		}
		sectorInfos[sectorKey] = &config.ObservationSectorInfo{
			Head: sectorInfo.Head.String(),
			Tail: tail,
		}
	}

	k.observation.ChangeKvsSectors(sectorInfos)
}
