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

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/kvs/activation"
	"github.com/llamerada-jp/colonio/node/internal/kvs/hosting"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/observation"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"go.etcd.io/raft/v3"
)

type Handler interface {
	KvsGetStability() (bool, []*types.NodeID, []*types.NodeID)
}

type Config struct {
	Logger             *slog.Logger
	EnableRaftLogging  bool
	Handler            Handler
	Outbound           OutboundPort
	ConsensusOutbound  consensus.OutboundPort
	ActivationResolver *activation.Resolver
	HostingManager     *hosting.Manager
	Observation        observation.Caller
	Store              kvsTypes.Store
}

type KVS struct {
	logger             *slog.Logger
	raftLogger         raft.Logger
	ctx                context.Context
	handler            Handler
	outbound           OutboundPort
	consensusOutbound  consensus.OutboundPort
	activationResolver *activation.Resolver
	hostingManager     *hosting.Manager
	observation        observation.Caller
	store              kvsTypes.Store
	localNodeID        *types.NodeID
	mtx                sync.RWMutex // for sectors, hostingManager, sectorUpdated
	sectors            map[kvsTypes.SectorKey]*sector.Sector
	sectorUpdated      bool       // for observation
	mtxFrontward       sync.Mutex // for activating frontward sector
}

func NewKVS(conf *Config) *KVS {
	k := &KVS{
		logger:             conf.Logger,
		handler:            conf.Handler,
		outbound:           conf.Outbound,
		consensusOutbound:  conf.ConsensusOutbound,
		activationResolver: conf.ActivationResolver,
		hostingManager:     conf.HostingManager,
		observation:        conf.Observation,
		store:              conf.Store,
		sectors:            make(map[kvsTypes.SectorKey]*sector.Sector),
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

	k.hostingManager.Start(k, localNodeID)

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

	sectorIsStable := k.hostingManager.ManageMember(nextNodeIDs)

	hostingSectorKey := k.hostingManager.GetHostingSectorKey()
	hostingSector := k.sectors[*hostingSectorKey]
	hostingSectorIsActive := hostingSector.GetTailAddress() != nil
	// frontwardNodeExists, frontwardSector := k.getFrontwardSector(frontwardNextNodeIDs)

	if !hostingSectorIsActive && sectorIsStable {
		entireState, err := k.activationResolver.ResolveEntireState(k.ctx)
		if err != nil {
			k.logger.Warn("Failed to get sector entire state", "error", err)
			return
		}

		// other node might have already activated the sector, check the state again
		if entireState != kvsTypes.EntireStateInactive {
			return
		}

		// Haven't activated sector yet, try to activate if the sector is stable.
		k.activateHostingSector(frontwardNextNodeIDs)
		return
	}

	if hostingSectorIsActive {
		if err := k.activationResolver.SetSectorState(k.ctx, kvsTypes.SectorStateActive); err != nil {
			k.logger.Warn("Failed to set sector state to active", "error", err)
		}
	}
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

func (k *KVS) kvsOperate(command proto.KvsOperation_Command, key string, value []byte) (proto.KvsOperationResponse_Error, []byte) {
	k.mtx.RLock()
	hostingSectorKey := k.hostingManager.GetHostingSectorKey()
	if hostingSectorKey == nil {
		k.mtx.RUnlock()
		k.logger.Debug("preparing hosting sector")
		return proto.KvsOperationResponse_ERROR_PREPARING, nil
	}

	operator := k.sectors[*hostingSectorKey].GetOperator()
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

func (k *KVS) sectorActivate(srcNodeID *types.NodeID, sectorID kvsTypes.SectorID, withImport bool) bool {
	k.mtx.Lock()
	defer k.mtx.Unlock()

	hostingSectorKey := k.hostingManager.GetHostingSectorKey()

	// ignore if it does not match the current node
	if hostingSectorKey == nil || hostingSectorKey.SectorID != sectorID {
		return false
	}

	// skip if there isn't hosing sector
	hostingSector, ok := k.sectors[*hostingSectorKey]
	if !ok {
		return false
	}

	// already activated
	if hostingSector.GetTailAddress() != nil {
		return true
	}

	nodeIsStable, _, frontwardNextNodeIDs := k.handler.KvsGetStability()
	if !nodeIsStable {
		return false
	}
	k.activateHostingSector(frontwardNextNodeIDs)

	return true
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
	k.mtx.Lock()
	defer k.mtx.Unlock()

	k.hostingManager.OnSectorAppendNode(sectorKey, sectorNo, nodeID)
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

	k.hostingManager.OnSectorRemoveNode(sectorKey, sectorNo)

	delete(k.sectors, targetKey)
	k.sectorUpdated = true

	go sector.Stop()
}

// AllocateSector implements hosting.SectorHandler.
func (k *KVS) AllocateSector(sectorKey *kvsTypes.SectorKey, head *types.NodeID, isHosting bool, join bool, members map[kvsTypes.SectorNo]*types.NodeID) {
	k.allocateSector(sectorKey, head, isHosting, join, members)
}

// ApplyAppendNode implements hosting.SectorHandler.
func (k *KVS) ApplyAppendNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	if s, ok := k.sectors[sectorKey]; ok {
		s.AppendNode(sectorNo, nodeID)
	}
}

// ApplyRemoveNode implements hosting.SectorHandler.
func (k *KVS) ApplyRemoveNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo) {
	if s, ok := k.sectors[sectorKey]; ok {
		s.RemoveNode(sectorNo)
	}
}

// SendSectorManageMember implements hosting.OutboundPort.
func (k *KVS) SendSectorManageMember(param *hosting.SectorManageMemberParam) {
	k.outbound.sendSectorManageMember(&SectorManageMemberParam{
		dstNodeID: param.DstNodeID,
		sectorID:  param.SectorID,
		sectorNo:  param.SectorNo,
		command:   param.Command,
		members:   param.Members,
	})
}

func (k *KVS) activateHostingSector(frontwardNextNodeIDs []*types.NodeID) {
	hostingSectorKey := k.hostingManager.GetHostingSectorKey()

	// local node is stable and other nodes are not exist
	if len(frontwardNextNodeIDs) == 0 {
		if len(k.sectors) != 0 {
			k.logger.Warn("No frontward node but sectors exist")
			return
		}
		hostingSector := k.sectors[*hostingSectorKey]
		hostingSector.Activate(*k.localNodeID)
		return
	}

	var candidate *sector.Sector
	frontwardNodeID := frontwardNextNodeIDs[0]
	for sectorKey, sector := range k.sectors {
		sectorHead := sector.GetHeadAddress()
		if sectorHead.Equal(k.localNodeID) {
			continue
		}
		if sectorHead.Equal(frontwardNodeID) {
			if candidate == nil || sectorKey.SectorNo > candidate.GetKey().SectorNo {
				candidate = sector
			}
		}

		if sectorHead.IsBetween(k.localNodeID, frontwardNodeID) {
			return
		}
	}
	if candidate == nil {
		return
	}

	hostingSector := k.sectors[*hostingSectorKey]
	hostingSector.Activate(*candidate.GetHeadAddress())
}

func (k *KVS) activateFrontwardSector(hostingSector, frontwardSector *sector.Sector) {
	// Frontward sector has already been activated.
	if frontwardSector.GetTailAddress() != nil {
		return
	}

	// Frontward sector is not adjacent.
	if !hostingSector.GetTailAddress().Equal(frontwardSector.GetHeadAddress()) {
		return
	}

	// lock to avoid multiple activation for frontward sector at the same time.
	locked := k.mtxFrontward.TryLock()
	if !locked {
		return
	}
	go func() {
		defer k.mtxFrontward.Unlock()
		k.outbound.sendSectorActivate(&SectorActivateParam{
			dstNodeID:  frontwardSector.GetHeadAddress(),
			sectorID:   frontwardSector.GetKey().SectorID,
			withImport: false,
		})
	}()
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
		headAddress := sector.GetHeadAddress()
		tailAddress := sector.GetTailAddress()
		tail := ""
		if tailAddress != nil {
			tail = tailAddress.String()
		}
		sectorInfos[sectorKey] = &observation.SectorInfo{
			Head: headAddress.String(),
			Tail: tail,
		}
	}

	k.observation.ChangeKvsSectors(sectorInfos)
}
