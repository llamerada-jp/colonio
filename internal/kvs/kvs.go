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

const (
	coordinatingNodeSequence = uint64(1)
)

type Handler interface {
	KVSGetStability() (bool, []*shared.NodeID)
}

type Config struct {
	Logger     *slog.Logger
	Handler    Handler
	Store      config.KVSStore
	Transferer *transferer.Transferer
}

type memberEntry struct {
	nodeID *shared.NodeID
	state  memberState
}

type KVS struct {
	logger       *slog.Logger
	ctx          context.Context
	handler      Handler
	store        config.KVSStore
	transferer   *transferer.Transferer
	localNodeID  *shared.NodeID
	mtx          sync.RWMutex
	nodes        map[config.KVSNodeKey]*node
	coordinating *config.KVSNodeKey
	lastSequence uint64
	// currentNode's member state
	members map[uint64]*memberEntry
}

func NewKVS(conf *Config) *KVS {
	k := &KVS{
		logger:     conf.Logger,
		handler:    conf.Handler,
		store:      conf.Store,
		transferer: conf.Transferer,
		nodes:      make(map[config.KVSNodeKey]*node),
		members:    make(map[uint64]*memberEntry),
	}

	transferer.SetRequestHandler[proto.PacketContent_RaftConfig](k.transferer, k.recvConfig)
	transferer.SetRequestHandler[proto.PacketContent_RaftConfigResponse](k.transferer, k.recvConfigResponse)
	transferer.SetRequestHandler[proto.PacketContent_RaftMessage](k.transferer, k.recvMessage)

	return k
}

func (k *KVS) Start(ctx context.Context, localNodeID *shared.NodeID) error {
	k.localNodeID = localNodeID
	k.ctx = ctx

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				k.subRoutine()
			}
		}
	}()

	return nil
}

func (k *KVS) subRoutine() {
	isStable, nextNodeIDs := k.handler.KVSGetStability()
	if !isStable {
		return
	}

	k.mtx.Lock()
	defer k.mtx.Unlock()

	if k.coordinating == nil {
		k.createCluster(nextNodeIDs)
		if k.coordinating == nil {
			return
		}
	}

	toAppend, toRemove := k.getNodeDiff(nextNodeIDs)
	for _, nodeID := range toAppend {
		k.lastSequence++
		k.members[k.lastSequence] = &memberEntry{
			nodeID: nodeID,
			state:  memberStateAppendingRaft,
		}
	}
	for seq := range toRemove {
		k.members[seq].state = memberStateRemoving
	}

	if err := k.applyConfigRaft(); err != nil {
		k.logger.Warn("Failed to apply Raft config", "error", err)
	}

	k.sendSettingMessage()
}

func (k *KVS) getNodeDiff(nextNodeIDs []*shared.NodeID) ([]*shared.NodeID, map[uint64]*memberEntry) {
	nextNodeIDMap := make(map[shared.NodeID]struct{})
	for _, nodeID := range nextNodeIDs {
		nextNodeIDMap[*nodeID] = struct{}{}
	}

	memberMap := make(map[shared.NodeID]uint64)
	for seq, entry := range k.members {
		if entry.state == memberStateRemoving {
			continue
		}
		memberMap[*entry.nodeID] = seq
	}

	toAppend := make([]*shared.NodeID, 0)
	toRemove := make(map[uint64]*memberEntry)
	for nodeID := range nextNodeIDMap {
		if _, ok := memberMap[nodeID]; !ok {
			toAppend = append(toAppend, &nodeID)
		}
	}
	for seq, entry := range k.members {
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

	k.lastSequence = coordinatingNodeSequence
	member := make(map[uint64]*shared.NodeID)
	member[coordinatingNodeSequence] = k.localNodeID
	for _, nodeID := range nextNodeIDs {
		k.lastSequence++
		member[k.lastSequence] = nodeID
		k.members[k.lastSequence] = &memberEntry{
			nodeID: nodeID,
			state:  memberStateCreating,
		}
	}

	nodeKey := &config.KVSNodeKey{
		ClusterID: clusterID,
		Sequence:  coordinatingNodeSequence,
	}
	k.coordinating = nodeKey
	k.allocateCluster(nodeKey, k.localNodeID, false, member)
}

func (k *KVS) allocateCluster(nodeKey *config.KVSNodeKey, head *shared.NodeID, append bool, members map[uint64]*shared.NodeID) {
	n := &node{}
	k.nodes[*nodeKey] = n

	raftNode := newRaftNode(&raftNodeConfig{
		logger:  k.logger,
		manager: k,
		store:   n,
		nodeKey: nodeKey,
		join:    append,
		member:  members,
	})

	store := newStore(&storeConfig{
		nodeKey: nodeKey,
		handler: n,
		head:    head,
	})

	n.raft = raftNode
	n.store = store

	raftNode.start(k.ctx)
}

func (k *KVS) applyConfigRaft() error {
	raftNode := k.nodes[*k.coordinating].raft

	for sequence, member := range k.members {
		switch member.state {
		case memberStateAppendingRaft:
			return raftNode.appendNode(sequence, member.nodeID)

		case memberStateRemoving:
			return raftNode.removeNode(sequence)
		}
	}

	return nil
}

func (k *KVS) sendSettingMessage() {
	for sequence, member := range k.members {
		if member.state == memberStateRemoving {
			continue
		}

		members := make(map[uint64]*proto.NodeID)
		members[coordinatingNodeSequence] = k.localNodeID.Proto()
		for ms, mm := range k.members {
			members[ms] = mm.nodeID.Proto()
		}

		var command proto.RaftConfigCommand
		if member.state == memberStateCreating {
			command = proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_CREATE
		} else {
			command = proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_APPEND
		}

		k.transferer.RequestOneWay(
			member.nodeID,
			shared.PacketModeExplicit|shared.PacketModeNoRetry,
			&proto.PacketContent{
				Content: &proto.PacketContent_RaftConfig{
					RaftConfig: &proto.RaftConfig{
						ClusterId: MustMarshalUUID(k.coordinating.ClusterID),
						Sequence:  sequence,
						Command:   command,
						Members:   members,
					},
				},
			},
		)
	}
}

func (k *KVS) recvConfig(packet *shared.Packet) {
	content := packet.Content.GetRaftConfig()
	clusterID, err := uuid.ParseBytes(content.ClusterId)
	if err != nil {
		k.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}
	sequence := content.Sequence
	command := content.Command
	members := make(map[uint64]*shared.NodeID)
	for seq, nodeID := range content.Members {
		var err error
		members[seq], err = shared.NewNodeIDFromProto(nodeID)
		if err != nil {
			k.logger.Warn("Failed to create NodeID from proto", "error", err, "nodeID", nodeID)
			return
		}
	}

	k.mtx.Lock()
	defer k.mtx.Unlock()

	nodeKey := config.KVSNodeKey{ClusterID: clusterID, Sequence: sequence}
	switch command {
	case proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_CREATE, proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_APPEND:
		if _, ok := k.nodes[nodeKey]; !ok {
			append := true
			if command == proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_CREATE {
				append = false
			}
			k.allocateCluster(&nodeKey, packet.SrcNodeID, append, members)
		}

	default:
		k.logger.Warn("Unknown RaftConfig command", "command", command)
		return
	}

	// send response when the node is created or appended
	response := &proto.PacketContent{
		Content: &proto.PacketContent_RaftConfigResponse{
			RaftConfigResponse: &proto.RaftConfigResponse{
				ClusterId: MustMarshalUUID(clusterID),
				Sequence:  sequence,
			},
		},
	}
	k.transferer.RequestOneWay(packet.SrcNodeID, shared.PacketModeExplicit|shared.PacketModeNoRetry, response)
}

func (k *KVS) recvConfigResponse(packet *shared.Packet) {
	content := packet.Content.GetRaftConfigResponse()
	clusterID, err := uuid.ParseBytes(content.ClusterId)
	if err != nil {
		k.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}
	sequence := content.Sequence

	k.mtx.Lock()
	defer k.mtx.Unlock()

	// ignore response if it does not match the current node
	if k.coordinating == nil || k.coordinating.ClusterID != clusterID {
		return
	}

	if entry, ok := k.members[sequence]; ok && entry.nodeID.Equal(packet.SrcNodeID) {
		entry.state = memberStateNormal
	}
}

func (k *KVS) recvMessage(packet *shared.Packet) {
	content := packet.Content.GetRaftMessage()
	clusterID, err := uuid.ParseBytes(content.ClusterId)
	if err != nil {
		k.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}

	key := config.KVSNodeKey{
		ClusterID: clusterID,
		Sequence:  content.Sequence,
	}

	k.mtx.RLock()
	n, ok := k.nodes[key]
	k.mtx.RUnlock()
	if !ok {
		return
	}

	if err := n.raft.processMessage(content); err != nil {
		k.logger.Error("Failed to process Raft message", "error", err)
	}
}

func (k *KVS) raftNodeError(nodeKey *config.KVSNodeKey, err error) {
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

func (k *KVS) raftNodeApplyProposal(nodeKey *config.KVSNodeKey, proposal *proto.RaftProposalManagement) {
	_, ok := k.nodes[*nodeKey]
	if !ok {
		return
	}

	panic("raftNodeApplyProposal not implemented")
}

func (k *KVS) raftNodeAppendNode(nodeKey *config.KVSNodeKey, sequence uint64, nodeID *shared.NodeID) {
	if *nodeKey != *k.coordinating {
		return
	}

	k.mtx.Lock()
	defer k.mtx.Unlock()

	member, ok := k.members[sequence]
	// unknown
	if !ok {
		k.logger.Warn("Unknown node sequence for appending", "sequence", sequence, "nodeID", nodeID)
		k.members[sequence] = &memberEntry{
			nodeID: nodeID,
			state:  memberStateRemoving,
		}
	}

	if member.state == memberStateAppendingRaft {
		member.state = memberStateAppendingNode
	} else {
		k.logger.Warn("Unexpected state for appending node", "sequence", sequence, "state", member.state, "nodeID", nodeID)
	}
}

func (k *KVS) raftNodeRemoveNode(nodeKey *config.KVSNodeKey, sequence uint64) {
	k.mtx.RLock()
	node, ok := k.nodes[*nodeKey]
	k.mtx.RUnlock()
	if !ok {
		return
	}

	if *nodeKey == *k.coordinating {
		delete(k.members, sequence)
	}

	node.raft.stop()
	k.mtx.Lock()
	defer k.mtx.Unlock()
	delete(k.nodes, *nodeKey)
}
