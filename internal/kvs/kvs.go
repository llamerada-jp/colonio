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
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type memberState int

const (
	memberStateNormal memberState = iota
	memberStateCreating
	memberStateAppendingRaft
	memberStateAppendingNode
	memberStateRemoving
)

type Handler interface {
	KvsGetStability() (bool, []*shared.NodeID)
	KvsState(state constants.KvsState) (constants.KvsState, error)
}

type Config struct {
	Logger      *slog.Logger
	Handler     Handler
	Observation config.ObservationCaller
	Store       config.KvsStore
	Transferer  *transferer.Transferer
}

type memberStateEntry struct {
	nodeID *shared.NodeID
	state  memberState
}

type KVS struct {
	logger        *slog.Logger
	ctx           context.Context
	handler       Handler
	observation   config.ObservationCaller
	store         config.KvsStore
	transferer    *transferer.Transferer
	localNodeID   *shared.NodeID
	mtx           sync.RWMutex
	sectors       map[config.KvsSectorKey]*sector
	sectorUpdated bool // for observation
	hostingSector *config.KvsSectorKey
	lastSequence  config.KvsSequence
	memberStates  map[config.KvsSequence]*memberStateEntry
}

func NewKVS(conf *Config) *KVS {
	k := &KVS{
		logger:       conf.Logger,
		handler:      conf.Handler,
		observation:  conf.Observation,
		store:        conf.Store,
		transferer:   conf.Transferer,
		sectors:      make(map[config.KvsSectorKey]*sector),
		memberStates: make(map[config.KvsSequence]*memberStateEntry),
	}

	transferer.SetRequestHandler[proto.PacketContent_KvsOperation](k.transferer, k.recvOperation)
	transferer.SetRequestHandler[proto.PacketContent_RaftConfig](k.transferer, k.recvConfig)
	transferer.SetRequestHandler[proto.PacketContent_RaftConfigResponse](k.transferer, k.recvConfigResponse)
	transferer.SetRequestHandler[proto.PacketContent_RaftMessage](k.transferer, k.recvMessage)

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
	k.sendOperation(proto.KvsOperation_COMMAND_GET, key, nil, func(res *proto.KvsOperationResponse, err error) {
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
	})

	return c
}

func (k *KVS) Set(key string, value []byte) chan error {
	c := make(chan error, 1)
	k.sendOperation(proto.KvsOperation_COMMAND_SET, key, value, func(res *proto.KvsOperationResponse, err error) {
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
	})

	return c
}

func (k *KVS) Patch(key string, value []byte) chan error {
	c := make(chan error, 1)
	k.sendOperation(proto.KvsOperation_COMMAND_PATCH, key, value, func(res *proto.KvsOperationResponse, err error) {
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
	})

	return c
}

func (k *KVS) Delete(key string) chan error {
	c := make(chan error, 1)
	k.sendOperation(proto.KvsOperation_COMMAND_DELETE, key, nil, func(res *proto.KvsOperationResponse, err error) {
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
		k.createCluster(nextNodeIDs)
		if k.hostingSector == nil {
			return
		}
	}

	toAppend, toRemove := k.getNodeDiff(nextNodeIDs)
	for _, nodeID := range toAppend {
		k.lastSequence++
		k.memberStates[k.lastSequence] = &memberStateEntry{
			nodeID: nodeID,
			state:  memberStateAppendingRaft,
		}
	}
	for seq := range toRemove {
		k.memberStates[seq].state = memberStateRemoving
	}

	if err := k.applyConfigRaft(); err != nil {
		k.logger.Warn("Failed to apply Raft config", "error", err)
	}

	k.sendSettingMessage()
}

func (k *KVS) getNodeDiff(nextNodeIDs []*shared.NodeID) ([]*shared.NodeID, map[config.KvsSequence]*memberStateEntry) {
	nextNodeIDMap := make(map[shared.NodeID]struct{})
	for _, nodeID := range nextNodeIDs {
		nextNodeIDMap[*nodeID] = struct{}{}
	}

	memberMap := make(map[shared.NodeID]config.KvsSequence)
	for seq, entry := range k.memberStates {
		if entry.state == memberStateRemoving {
			continue
		}
		memberMap[*entry.nodeID] = seq
	}

	toAppend := make([]*shared.NodeID, 0)
	toRemove := make(map[config.KvsSequence]*memberStateEntry)
	for nodeID := range nextNodeIDMap {
		if _, ok := memberMap[nodeID]; !ok {
			toAppend = append(toAppend, &nodeID)
		}
	}
	for seq, entry := range k.memberStates {
		if entry.state == memberStateRemoving {
			continue
		}
		if _, ok := nextNodeIDMap[*entry.nodeID]; !ok {
			toRemove[seq] = entry
		}
	}
	return toAppend, toRemove
}

func (k *KVS) createCluster(nextNodeIDs []*shared.NodeID) {
	clusterID, err := uuid.NewV7()
	if err != nil {
		k.logger.Error("Failed to create new cluster ID", "error", err)
		return
	}

	k.lastSequence = config.KvsSectorHostNodeSequence
	member := make(map[config.KvsSequence]*shared.NodeID)
	member[config.KvsSectorHostNodeSequence] = k.localNodeID
	for _, nodeID := range nextNodeIDs {
		k.lastSequence++
		member[k.lastSequence] = nodeID
		k.memberStates[k.lastSequence] = &memberStateEntry{
			nodeID: nodeID,
			state:  memberStateCreating,
		}
	}

	sectorKey := &config.KvsSectorKey{
		SectorID: clusterID,
		Sequence: config.KvsSectorHostNodeSequence,
	}
	k.hostingSector = sectorKey
	k.allocateCluster(sectorKey, k.localNodeID, false, member)
}

func (k *KVS) allocateCluster(
	sectorKey *config.KvsSectorKey,
	head *shared.NodeID,
	append bool,
	members map[config.KvsSequence]*shared.NodeID,
) {
	s := &sector{}
	k.sectors[*sectorKey] = s
	k.sectorUpdated = true

	raftNode := newRaftNode(&raftNodeConfig{
		logger:    k.logger,
		manager:   k,
		store:     s,
		sectorKey: sectorKey,
		join:      append,
		member:    members,
	})

	store := newStore(&storeConfig{
		sectorKey: sectorKey,
		handler:   s,
		head:      head,
	})

	s.raft = raftNode
	s.store = store

	raftNode.start(k.ctx)
}

func (k *KVS) applyConfigRaft() error {
	raftNode := k.sectors[*k.hostingSector].raft

	for sequence, member := range k.memberStates {
		switch member.state {
		case memberStateAppendingRaft:
			return raftNode.appendNode(sequence, member.nodeID)

		case memberStateRemoving:
			return raftNode.removeNode(sequence)
		}
	}

	return nil
}

type kvsOperationHandler struct {
	receiver func(res *proto.KvsOperationResponse, err error)
}

func (h *kvsOperationHandler) OnResponse(packet *shared.Packet) {
	h.receiver(packet.Content.GetKvsOperationResponse(), nil)
}

func (h *kvsOperationHandler) OnError(code constants.PacketErrorCode, message string) {
	h.receiver(nil, fmt.Errorf("packet error %d: %s", code, message))
}

func (k *KVS) sendOperation(command proto.KvsOperation_Command, key string, value []byte, receiver func(res *proto.KvsOperationResponse, err error)) {
	dst := shared.NewHashedNodeID([]byte(key))

	content := &proto.PacketContent{
		Content: &proto.PacketContent_KvsOperation{
			KvsOperation: &proto.KvsOperation{
				Command: command,
				Key:     key,
				Value:   value,
			},
		},
	}

	k.transferer.Request(dst, shared.PacketModeNone, content, &kvsOperationHandler{
		receiver: receiver,
	})
}

func (k *KVS) sendSettingMessage() {
	for sequence, ms := range k.memberStates {
		switch ms.state {
		case memberStateNormal, memberStateRemoving, memberStateAppendingNode:
			continue

		case memberStateCreating, memberStateAppendingRaft:
			members := make(map[uint64]*proto.NodeID)
			members[uint64(config.KvsSectorHostNodeSequence)] = k.localNodeID.Proto()
			for sequence, ms := range k.memberStates {
				members[uint64(sequence)] = ms.nodeID.Proto()
			}

			var command proto.RaftConfig_Command
			if ms.state == memberStateCreating {
				command = proto.RaftConfig_COMMAND_CREATE
			} else { // memberStateAppendingRaft
				command = proto.RaftConfig_COMMAND_APPEND
			}

			k.transferer.RequestOneWay(
				ms.nodeID,
				shared.PacketModeExplicit|shared.PacketModeNoRetry,
				&proto.PacketContent{
					Content: &proto.PacketContent_RaftConfig{
						RaftConfig: &proto.RaftConfig{
							SectorId: MustMarshalUUID(k.hostingSector.SectorID),
							Sequence: uint64(sequence),
							Command:  command,
							Members:  members,
						},
					},
				},
			)
		}
	}
}

func (k *KVS) recvOperation(packet *shared.Packet) {
	content := packet.Content.GetKvsOperation()
	command := content.Command
	key := content.Key
	value := content.Value
	store := k.sectors[*k.hostingSector].store

	switch command {
	case proto.KvsOperation_COMMAND_GET:
		data, err := store.get(key)

		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			if err == config.ErrorKvsStoreKeyNotFound {
				e = proto.KvsOperationResponse_ERROR_NOT_FOUND
			} else {
				e = proto.KvsOperationResponse_ERROR_UNKNOWN
			}
		}

		k.transferer.Response(packet, &proto.PacketContent{
			Content: &proto.PacketContent_KvsOperationResponse{
				KvsOperationResponse: &proto.KvsOperationResponse{
					Error: e,
					Value: data,
				},
			},
		})

	case proto.KvsOperation_COMMAND_SET:
		err := store.set(key, value)
		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			e = proto.KvsOperationResponse_ERROR_UNKNOWN
		}

		k.transferer.Response(packet, &proto.PacketContent{
			Content: &proto.PacketContent_KvsOperationResponse{
				KvsOperationResponse: &proto.KvsOperationResponse{
					Error: e,
				},
			},
		})

	case proto.KvsOperation_COMMAND_PATCH:
		err := store.patch(key, value)
		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			e = proto.KvsOperationResponse_ERROR_UNKNOWN
		}

		k.transferer.Response(packet, &proto.PacketContent{
			Content: &proto.PacketContent_KvsOperationResponse{
				KvsOperationResponse: &proto.KvsOperationResponse{
					Error: e,
				},
			},
		})

	case proto.KvsOperation_COMMAND_DELETE:
		err := store.delete(key)
		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			e = proto.KvsOperationResponse_ERROR_UNKNOWN
		}

		k.transferer.Response(packet, &proto.PacketContent{
			Content: &proto.PacketContent_KvsOperationResponse{
				KvsOperationResponse: &proto.KvsOperationResponse{
					Error: e,
				},
			},
		})

	default:
		k.transferer.Response(packet, &proto.PacketContent{
			Content: &proto.PacketContent_KvsOperationResponse{
				KvsOperationResponse: &proto.KvsOperationResponse{
					Error: proto.KvsOperationResponse_ERROR_UNKNOWN,
				},
			},
		})
	}
}

func (k *KVS) recvConfig(packet *shared.Packet) {
	content := packet.Content.GetRaftConfig()
	var sectorID uuid.UUID
	if err := sectorID.UnmarshalBinary(content.SectorId); err != nil {
		k.logger.Warn("Failed to parse promoter sectorID", "error", err)
		return
	}
	sequence := config.KvsSequence(content.Sequence)
	command := content.Command
	members := make(map[config.KvsSequence]*shared.NodeID)
	for sequence, nodeID := range content.Members {
		var err error
		members[config.KvsSequence(sequence)], err = shared.NewNodeIDFromProto(nodeID)
		if err != nil {
			k.logger.Warn("Failed to create NodeID from proto", "error", err, "nodeID", nodeID)
			return
		}
	}

	k.mtx.Lock()
	defer k.mtx.Unlock()

	sectorKey := config.KvsSectorKey{
		SectorID: sectorID,
		Sequence: sequence,
	}
	switch command {
	case proto.RaftConfig_COMMAND_CREATE, proto.RaftConfig_COMMAND_APPEND:
		if _, ok := k.sectors[sectorKey]; !ok {
			append := true
			if command == proto.RaftConfig_COMMAND_CREATE {
				fmt.Println("✅ received CREATE")
				append = false
			}
			k.allocateCluster(&sectorKey, packet.SrcNodeID, append, members)
		}

	default:
		k.logger.Warn("Unknown RaftConfig command", "command", command)
		return
	}

	// send response when the node is created or appended
	response := &proto.PacketContent{
		Content: &proto.PacketContent_RaftConfigResponse{
			RaftConfigResponse: &proto.RaftConfigResponse{
				SectorId: MustMarshalUUID(sectorID),
				Sequence: uint64(sequence),
			},
		},
	}
	k.transferer.RequestOneWay(packet.SrcNodeID, shared.PacketModeExplicit|shared.PacketModeNoRetry, response)
}

func (k *KVS) recvConfigResponse(packet *shared.Packet) {
	content := packet.Content.GetRaftConfigResponse()
	sectorID, err := uuid.ParseBytes(content.SectorId)
	if err != nil {
		k.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}
	sequence := config.KvsSequence(content.Sequence)

	k.mtx.Lock()
	defer k.mtx.Unlock()

	// ignore response if it does not match the current node
	if k.hostingSector == nil || k.hostingSector.SectorID != sectorID {
		return
	}

	if entry, ok := k.memberStates[sequence]; ok && entry.nodeID.Equal(packet.SrcNodeID) {
		entry.state = memberStateNormal
	}
}

func (k *KVS) recvMessage(packet *shared.Packet) {
	content := packet.Content.GetRaftMessage()
	sectorID, err := uuid.ParseBytes(content.SectorId)
	if err != nil {
		k.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}

	key := config.KvsSectorKey{
		SectorID: sectorID,
		Sequence: config.KvsSequence(content.Sequence),
	}

	k.mtx.RLock()
	n, ok := k.sectors[key]
	k.mtx.RUnlock()
	if !ok {
		return
	}

	if err := n.raft.processMessage(content); err != nil {
		k.logger.Error("Failed to process Raft message", "error", err)
	}
}

func (k *KVS) raftNodeError(sectorKey *config.KvsSectorKey, err error) {
	panic(fmt.Sprintf("raftNodeError not implemented: %s", err))
}

func (k *KVS) raftNodeSendMessage(dstNodeID *shared.NodeID, message *proto.RaftMessage) {
	k.transferer.RequestOneWay(
		dstNodeID,
		shared.PacketModeExplicit|shared.PacketModeNoRetry,
		&proto.PacketContent{
			Content: &proto.PacketContent_RaftMessage{
				RaftMessage: message,
			},
		})
}

func (k *KVS) raftNodeApplyProposal(sectorKey *config.KvsSectorKey, proposal *proto.RaftProposalManagement) {
	_, ok := k.sectors[*sectorKey]
	if !ok {
		return
	}

	panic("raftNodeApplyProposal not implemented")
}

func (k *KVS) raftNodeAppendNode(sectorKey *config.KvsSectorKey, sequence config.KvsSequence, nodeID *shared.NodeID) {
	if k.hostingSector == nil || *sectorKey != *k.hostingSector {
		return
	}

	k.mtx.Lock()
	defer k.mtx.Unlock()

	member, ok := k.memberStates[sequence]
	// unknown
	if !ok {
		k.logger.Warn("Unknown node sequence for appending", "sequence", sequence, "nodeID", nodeID)
		k.memberStates[sequence] = &memberStateEntry{
			nodeID: nodeID,
			state:  memberStateRemoving,
		}
		return
	}

	if member.state == memberStateAppendingRaft {
		member.state = memberStateAppendingNode
	} else {
		k.logger.Warn("Unexpected state for appending node", "sequence", sequence, "state", member.state, "nodeID", nodeID)
	}
}

func (k *KVS) raftNodeRemoveNode(sectorKey *config.KvsSectorKey, sequence config.KvsSequence) {
	k.mtx.RLock()
	sector, ok := k.sectors[*sectorKey]
	k.mtx.RUnlock()
	if !ok {
		return
	}

	if *sectorKey == *k.hostingSector {
		delete(k.memberStates, sequence)
	}

	sector.raft.stop()
	k.mtx.Lock()
	defer k.mtx.Unlock()
	delete(k.sectors, *sectorKey)
}

func (k *KVS) takeObservation() {
	k.mtx.RLock()
	defer k.mtx.RUnlock()
	if !k.sectorUpdated {
		return
	}

	sectorInfos := make(map[config.KvsSectorKey]*config.ObservationSectorInfo)
	for sectorKey, sector := range k.sectors {
		tail := ""
		if sector.store.tail != nil {
			tail = sector.store.tail.String()
		}
		sectorInfos[sectorKey] = &config.ObservationSectorInfo{
			Head: sector.store.head.String(),
			Tail: tail,
		}
	}

	k.observation.ChangeKvsSectors(sectorInfos)
}
