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
package raft

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type memberState int

const (
	memberStateNormal memberState = iota
	memberStateCreating
	memberStateAppending
	memberStateRemoving
)

type managerHandler interface {
	RaftGetStability() (bool, []*shared.NodeID)
}

type Config struct {
	Logger     *slog.Logger
	Handler    managerHandler
	Transferer *transferer.Transferer
}

type memberEntry struct {
	nodeID *shared.NodeID
	state  memberState
}

type nodeKey struct {
	clusterID uuid.UUID
	sequence  uint64
}

type Manager struct {
	logger       *slog.Logger
	handler      managerHandler
	transferer   *transferer.Transferer
	localNodeID  *shared.NodeID
	mtx          sync.RWMutex
	raftNodes    map[nodeKey]*node
	currentNode  *node
	lastSequence uint64
	// currentNode's member state
	members map[uint64]*memberEntry
}

func NewManager(config *Config) *Manager {
	m := &Manager{
		logger:     config.Logger,
		handler:    config.Handler,
		transferer: config.Transferer,
		raftNodes:  make(map[nodeKey]*node),
		members:    make(map[uint64]*memberEntry),
	}

	transferer.SetRequestHandler[proto.PacketContent_RaftConfig](m.transferer, m.recvConfig)
	transferer.SetRequestHandler[proto.PacketContent_RaftConfigResponse](m.transferer, m.recvConfigResponse)

	return m
}

func (m *Manager) Run(ctx context.Context, localNodeID *shared.NodeID) {
	m.localNodeID = localNodeID

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			m.subRoutine()
		}
	}
}

func (m *Manager) subRoutine() {
	isStable, nextNodeIDs := m.handler.RaftGetStability()
	if !isStable {
		return
	}

	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.currentNode == nil {
		m.lastSequence = 0
		m.createCluster(nextNodeIDs)
		if m.currentNode == nil {
			return
		}
	}

	toAppend, toRemove := m.getNodeDiff(nextNodeIDs)
	for _, nodeID := range toAppend {
		m.lastSequence++
		m.members[m.lastSequence] = &memberEntry{
			nodeID: nodeID,
			state:  memberStateAppending,
		}
		m.currentNode.appendNode(m.lastSequence, nodeID)
	}
	for seq := range toRemove {
		m.members[seq].state = memberStateRemoving
		m.currentNode.removeNode(seq)
	}

	m.sendSettingMessage()
}

func (m *Manager) getNodeDiff(nextNodeIDs []*shared.NodeID) ([]*shared.NodeID, map[uint64]*memberEntry) {
	nextNodeIDMap := make(map[shared.NodeID]struct{})
	for _, nodeID := range nextNodeIDs {
		nextNodeIDMap[*nodeID] = struct{}{}
	}

	memberMap := make(map[shared.NodeID]uint64)
	for seq, entry := range m.members {
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
	for seq, entry := range m.members {
		if entry.state == memberStateRemoving {
			continue
		}
		if _, ok := nextNodeIDMap[*entry.nodeID]; !ok {
			toRemove[seq] = entry
		}
	}
	return toAppend, toRemove
}

func (m *Manager) createCluster(nextNodeIDs []*shared.NodeID) {
	clusterID, err := uuid.NewV7()
	if err != nil {
		m.logger.Error("Failed to create new cluster ID", "error", err)
		return
	}

	member := make(map[uint64]*shared.NodeID)
	member[0] = m.localNodeID
	for _, nodeID := range nextNodeIDs {
		m.lastSequence++
		member[m.lastSequence] = nodeID
		m.members[m.lastSequence] = &memberEntry{
			nodeID: nodeID,
			state:  memberStateCreating,
		}
	}

	m.currentNode = m.allocateCluster(clusterID, false, 0, member)
}

func (m *Manager) allocateCluster(clusterID uuid.UUID, append bool, sequence uint64, members map[uint64]*shared.NodeID) *node {
	config := &nodeConfig{
		clusterID: clusterID,
		join:      append,
		sequence:  sequence,
		member:    members,
	}
	newNode := newNode(config)
	m.raftNodes[nodeKey{clusterID: clusterID, sequence: sequence}] = newNode

	if err := newNode.start(); err != nil {
		m.logger.Error("Failed to start node", "error", err)
		return nil
	}

	return newNode
}

func (m *Manager) sendSettingMessage() {
	for sequence, member := range m.members {
		if member.state == memberStateRemoving {
			continue
		}

		members := make(map[uint64]*proto.NodeID)
		members[0] = m.localNodeID.Proto()
		for ms, mm := range m.members {
			members[ms] = mm.nodeID.Proto()
		}

		var command proto.RaftConfigCommand
		if member.state == memberStateCreating {
			command = proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_CREATE
		} else {
			command = proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_APPEND
		}

		content := &proto.PacketContent{
			Content: &proto.PacketContent_RaftConfig{
				RaftConfig: &proto.RaftConfig{
					ClusterId: m.currentNode.clusterID.String(),
					Sequence:  sequence,
					Command:   command,
					Members:   members,
				},
			},
		}

		m.transferer.RequestOneWay(member.nodeID, shared.PacketModeExplicit|shared.PacketModeNoRetry, content)
	}
}

func (m *Manager) recvConfig(packet *shared.Packet) {
	content := packet.Content.GetRaftConfig()
	clusterID, err := uuid.Parse(content.ClusterId)
	if err != nil {
		m.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}
	sequence := content.Sequence
	command := content.Command
	members := make(map[uint64]*shared.NodeID)
	for seq, nodeID := range content.Members {
		var err error
		members[seq], err = shared.NewNodeIDFromProto(nodeID)
		if err != nil {
			m.logger.Warn("Failed to create NodeID from proto", "error", err, "nodeID", nodeID)
			return
		}
	}

	m.mtx.Lock()
	defer m.mtx.Unlock()

	nodeKey := nodeKey{clusterID: clusterID, sequence: sequence}
	switch command {
	case proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_CREATE, proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_APPEND:
		if _, ok := m.raftNodes[nodeKey]; !ok {
			append := true
			if command == proto.RaftConfigCommand_RAFT_CONFIG_COMMAND_CREATE {
				append = false
			}
			if err := m.allocateCluster(clusterID, append, sequence, members); err != nil {
				return
			}
		}

	default:
		m.logger.Warn("Unknown RaftConfig command", "command", command)
		return
	}

	// send response when the node is created or appended
	response := &proto.PacketContent{
		Content: &proto.PacketContent_RaftConfigResponse{
			RaftConfigResponse: &proto.RaftConfigResponse{
				ClusterId: clusterID.String(),
				Sequence:  sequence,
			},
		},
	}
	m.transferer.RequestOneWay(packet.SrcNodeID, shared.PacketModeExplicit|shared.PacketModeNoRetry, response)
}

func (m *Manager) recvConfigResponse(packet *shared.Packet) {
	content := packet.Content.GetRaftConfigResponse()
	clusterID, err := uuid.Parse(content.ClusterId)
	if err != nil {
		m.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}
	sequence := content.Sequence

	m.mtx.Lock()
	defer m.mtx.Unlock()

	// ignore response if it does not match the current node
	if m.currentNode == nil || m.currentNode.clusterID != clusterID {
		return
	}

	if entry, ok := m.members[sequence]; ok && entry.nodeID.Equal(packet.SrcNodeID) {
		entry.state = memberStateNormal
	}
}
