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
	"github.com/llamerada-jp/colonio/node/internal/kvs/activation"
	"github.com/llamerada-jp/colonio/node/internal/kvs/broker"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/observation"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
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
	KvsGetStability() (bool, []*types.NodeID, []*types.NodeID)
}

type Config struct {
	Logger                  *slog.Logger
	EnableRaftLogging       bool
	Handler                 Handler
	Outbound                OutboundPort
	ConsensusOutbound       consensus.OutboundPort
	ActivationResolver      *activation.Resolver
	SectorInformationBroker *broker.Broker
	Observation             observation.Caller
	Store                   kvsTypes.Store
}

type memberStateEntry struct {
	nodeID *types.NodeID
	state  memberState
}

type KVS struct {
	logger                  *slog.Logger
	raftLogger              raft.Logger
	ctx                     context.Context
	handler                 Handler
	outbound                OutboundPort
	consensusOutbound       consensus.OutboundPort
	activationResolver      *activation.Resolver
	sectorInformationBroker *broker.Broker
	observation             observation.Caller
	store                   kvsTypes.Store
	localNodeID             *types.NodeID
	mtx                     sync.RWMutex // for sectors, hostingSector, memberStates
	sectors                 map[kvsTypes.SectorKey]*sector.Sector
	sectorUpdated           bool // for observation
	hostingSector           *kvsTypes.SectorKey
	lastSectorNo            kvsTypes.SectorNo
	memberStates            map[kvsTypes.SectorNo]*memberStateEntry
}

func NewKVS(conf *Config) *KVS {
	k := &KVS{
		logger:                  conf.Logger,
		handler:                 conf.Handler,
		outbound:                conf.Outbound,
		consensusOutbound:       conf.ConsensusOutbound,
		sectorInformationBroker: conf.SectorInformationBroker,
		activationResolver:      conf.ActivationResolver,
		observation:             conf.Observation,
		store:                   conf.Store,
		sectors:                 make(map[kvsTypes.SectorKey]*sector.Sector),
		memberStates:            make(map[kvsTypes.SectorNo]*memberStateEntry),
	}

	if conf.EnableRaftLogging {
		k.raftLogger = newSlogWrapper(conf.Logger)
	} else {
		k.raftLogger = newEmptyLogger()
	}

	return k
}

func (k *KVS) Start(ctx context.Context, localNodeID *types.NodeID) {
	k.localNodeID = localNodeID
	k.ctx = ctx

	k.sectorInformationBroker.Start(ctx, localNodeID)

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

func (k *KVS) Get(key string) chan *kvsTypes.GetResult {
	c := make(chan *kvsTypes.GetResult, 1)
	k.outbound.sendKvsOperation(&operationParam{
		command: proto.KvsOperation_COMMAND_GET,
		key:     key,
		value:   nil,
		receiver: func(res *proto.KvsOperationResponse, err error) {
			defer close(c)

			if err != nil {
				c <- &kvsTypes.GetResult{
					Data: nil,
					Err:  err,
				}
				return
			}

			switch res.Error {
			case proto.KvsOperationResponse_ERROR_NONE:
				// ok
				c <- &kvsTypes.GetResult{
					Data: res.Value,
					Err:  nil,
				}

			case proto.KvsOperationResponse_ERROR_NOT_FOUND:
				c <- &kvsTypes.GetResult{
					Data: nil,
					Err:  fmt.Errorf("key not found: %s", key),
				}

			default:
				c <- &kvsTypes.GetResult{
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
	k.outbound.sendKvsOperation(&operationParam{
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
	k.outbound.sendKvsOperation(&operationParam{
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
	k.outbound.sendKvsOperation(&operationParam{
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
	nodeIsStable, backwardNextNodeIDs, frontwardNextNodeIDs := k.handler.KvsGetStability()
	if !nodeIsStable {
		return
	}
	nextNodeIDs := append(backwardNextNodeIDs, frontwardNextNodeIDs...)
	k.sectorInformationBroker.UpdateNextNodeIDs(backwardNextNodeIDs, frontwardNextNodeIDs)

	k.mtx.Lock()
	defer k.mtx.Unlock()

	sectorIsStable := k.manageMember(nextNodeIDs)
	tailAddress := k.getTailAddress()
	k.sectorInformationBroker.UpdateTailAddress(tailAddress)
	if tailAddress == nil {
		// Haven't activated sector yet, try to activate if the sector is stable.
		if sectorIsStable {
			k.activateSector()
		}
		return
	} else {
		if err := k.activationResolver.SetSectorState(k.ctx, kvsTypes.SectorStateActive); err != nil {
			k.logger.Warn("Failed to set sector state to active", "error", err)
		}
	}
}

func (k *KVS) manageMember(nextNodeIDs []*types.NodeID) bool {
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

	// check if all members are in normal state
	for state := range k.memberStates {
		if k.memberStates[state].state != memberStateNormal {
			return false
		}
	}

	return true
}

func (k *KVS) getNodesToBeChanged(nextNodeIDs []*types.NodeID) ([]*types.NodeID, map[kvsTypes.SectorNo]struct{}) {
	nextNodeIDMap := make(map[types.NodeID]struct{})
	for _, nodeID := range nextNodeIDs {
		nextNodeIDMap[*nodeID] = struct{}{}
	}

	toAppend := make([]*types.NodeID, 0)
	memberMap := make(map[types.NodeID]kvsTypes.SectorNo)
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

	toRemove := make(map[kvsTypes.SectorNo]struct{})
	for sec, entry := range k.memberStates {
		if entry.state == memberStateRemoving || sec == kvsTypes.HostNodeSectorNo {
			continue
		}
		if _, ok := nextNodeIDMap[*entry.nodeID]; !ok {
			toRemove[sec] = struct{}{}
		}
	}
	return toAppend, toRemove
}

func (k *KVS) hostSector(nextNodeIDs []*types.NodeID) {
	sectorID, err := uuid.NewV7()
	if err != nil {
		panic("Failed to create new sector ID")
	}

	k.lastSectorNo = kvsTypes.HostNodeSectorNo
	members := make(map[kvsTypes.SectorNo]*types.NodeID)
	members[kvsTypes.HostNodeSectorNo] = k.localNodeID
	k.memberStates[kvsTypes.HostNodeSectorNo] = &memberStateEntry{
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

	sectorKey := &kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(sectorID),
		SectorNo: kvsTypes.HostNodeSectorNo,
	}
	k.hostingSector = sectorKey
	k.allocateSector(sectorKey, k.localNodeID, true, false, members)
}

func (k *KVS) allocateSector(
	sectorKey *kvsTypes.SectorKey,
	head *types.NodeID,
	isHosting bool,
	append bool,
	members map[kvsTypes.SectorNo]*types.NodeID,
) {
	s := sector.NewSector(&sector.SectorConfig{
		Logger:     k.logger,
		RaftLogger: k.raftLogger,
		Handler:    k,
		Outbound:   k.consensusOutbound,
		SectorKey:  sectorKey,
		IsHosting:  isHosting,
		Join:       append,
		Members:    members,
		Store:      k.store,
		Head:       head,
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
		if sectorNo == kvsTypes.HostNodeSectorNo {
			continue
		}

		switch ms.state {
		case memberStateNormal, memberStateRemoving, memberStateConfiguredNode:
			continue

		case memberStateCreating, memberStateAppending, memberStateConfiguredConsensus:
			members := make(map[kvsTypes.SectorNo]*types.NodeID)
			members[kvsTypes.HostNodeSectorNo] = k.localNodeID
			for sn, ms := range k.memberStates {
				members[sn] = ms.nodeID
			}

			var command proto.SectorManageMember_Command
			if ms.state == memberStateCreating {
				command = proto.SectorManageMember_COMMAND_CREATE
			} else { // memberStateAppending
				command = proto.SectorManageMember_COMMAND_APPEND
			}

			go k.outbound.sendSectorManageMember(&SectorManageMemberParam{
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
			if err == kvsTypes.ErrorStoreKeyNotFound {
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

type sectorManageMemberParam struct {
	command   proto.SectorManageMember_Command
	sectorKey kvsTypes.SectorKey
	head      *types.NodeID
	members   map[kvsTypes.SectorNo]*types.NodeID
}

func (k *KVS) sectorManageMember(param *sectorManageMemberParam) error {
	k.mtx.Lock()
	defer k.mtx.Unlock()

	switch param.command {
	case proto.SectorManageMember_COMMAND_CREATE, proto.SectorManageMember_COMMAND_APPEND:
		if _, ok := k.sectors[param.sectorKey]; !ok {
			append := true
			if param.command == proto.SectorManageMember_COMMAND_CREATE {
				append = false
			}
			k.allocateSector(&param.sectorKey, param.head, false, append, param.members)
		}

	default:
		return fmt.Errorf("unknown SectorManageMember command: %d", param.command)
	}

	return nil
}

func (k *KVS) sectorManageMemberResponse(srcNodeID *types.NodeID, sectorID kvsTypes.SectorID, sectorNo kvsTypes.SectorNo) {
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

func (k *KVS) processConsensusMessage(key kvsTypes.SectorKey, message *proto.ConsensusMessage) {
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

func (k *KVS) SectorError(sectorKey *kvsTypes.SectorKey, err error) {
	panic(fmt.Sprintf("raftNodeError not implemented: %s", err))
}

func (k *KVS) SectorAppendNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
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

	if sectorNo == kvsTypes.HostNodeSectorNo {
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

func (k *KVS) SectorRemoveNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo) {
	k.mtx.Lock()
	defer k.mtx.Unlock()
	targetKey := kvsTypes.SectorKey{
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
	k.sectorUpdated = true

	go sector.Stop()
}

func (k *KVS) getTailAddress() *types.NodeID {
	sector := k.sectors[*k.hostingSector]
	return sector.GetTailAddress()
}

func (k *KVS) activateSector() {
	entireState, err := k.activationResolver.ResolveEntireState(k.ctx)
	if err != nil {
		k.logger.Warn("Failed to get KVS state", "error", err)
		return
	}

	// other node might have already activated the sector, check the state again
	if entireState != kvsTypes.EntireStateInactive {
		return
	}

	frontwardAddress, _ := k.sectorInformationBroker.GetFrontwardState()
	// haven't got frontward address yet, wait for next subRoutine
	if frontwardAddress == nil {
		return
	}

	sector := k.sectors[*k.hostingSector]
	sector.Activate(*frontwardAddress)
}

func (k *KVS) takeObservation() {
	k.mtx.RLock()
	defer k.mtx.RUnlock()
	if !k.sectorUpdated {
		return
	}
	k.sectorUpdated = false

	sectorInfos := make(map[kvsTypes.SectorKey]*observation.SectorInfo)
	for sectorKey, sector := range k.sectors {
		sectorInfo := sector.GetInfo()
		tail := ""
		if sectorInfo.Tail != nil {
			tail = sectorInfo.Tail.String()
		}
		sectorInfos[sectorKey] = &observation.SectorInfo{
			Head: sectorInfo.Head.String(),
			Tail: tail,
		}
	}

	k.observation.ChangeKvsSectors(sectorInfos)
}
