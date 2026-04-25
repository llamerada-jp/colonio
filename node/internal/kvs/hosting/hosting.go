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
package hosting

import (
	"log/slog"
	"sync"

	"github.com/google/uuid"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

type MemberState int

const (
	MemberStateNormal              MemberState = iota
	MemberStateCreating                        // Creating a new Sector cluster with initial members
	MemberStateAppending                       // Appending a new member to the existing Sector
	MemberStateConfiguredNode                  // Configured node member but not yet joined to the Sector
	MemberStateConfiguredConsensus             // Get consensus of Sector but not configured to the node
	MemberStateRemoving
)

type MemberStateEntry struct {
	NodeID *types.NodeID
	State  MemberState
}

// SectorHandler is implemented by KVS to perform sector operations on behalf of the Manager.
type SectorHandler interface {
	AllocateSector(sectorKey *kvsTypes.SectorKey, head *types.NodeID, isHosting bool, join bool, members map[kvsTypes.SectorNo]*types.NodeID)
	ApplyAppendNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID)
	ApplyRemoveNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo)
}

// OutboundPort is implemented by KVS to send messages to remote nodes.
type OutboundPort interface {
	sendSectorManageMember(param *SectorManageMemberParam)
}

type SectorManageMemberParam struct {
	DstNodeID *types.NodeID
	SectorID  kvsTypes.SectorID
	SectorNo  kvsTypes.SectorNo
	Command   proto.SectorManageMember_Command
	Members   map[kvsTypes.SectorNo]*types.NodeID
}

type Config struct {
	Logger   *slog.Logger
	Outbound OutboundPort
}

// Manager controls the hosting sector: which sector this node hosts, the sector number counter,
// and the membership state machine for each member of the hosting sector's Raft cluster.
type Manager struct {
	logger      *slog.Logger
	handler     SectorHandler
	outbound    OutboundPort
	localNodeID *types.NodeID

	mtx              sync.RWMutex // protects: hostingSectorKey, lastSectorNo, memberStates
	hostingSectorKey *kvsTypes.SectorKey
	lastSectorNo     kvsTypes.SectorNo
	memberStates     map[kvsTypes.SectorNo]*MemberStateEntry
}

func NewManager(conf *Config) *Manager {
	return &Manager{
		logger:       conf.Logger,
		outbound:     conf.Outbound,
		memberStates: make(map[kvsTypes.SectorNo]*MemberStateEntry),
	}
}

func (m *Manager) Start(handler SectorHandler, localNodeID *types.NodeID) {
	m.handler = handler
	m.localNodeID = localNodeID
}

// GetHostingSectorKey returns the sector key for the sector this node is hosting,
// or nil if no hosting sector has been established yet.
func (m *Manager) GetHostingSectorKey() *kvsTypes.SectorKey {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return m.hostingSectorKey
}

// ManageMember adjusts the Raft membership of the hosting sector to match nextNodeIDs.
// Returns true when all members are in a stable (Normal) state.
func (m *Manager) ManageMember(nextNodeIDs []*types.NodeID) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.hostingSectorKey == nil {
		m.initHostSector(nextNodeIDs)
	}

	toAppend, toRemove := m.getNodesToBeChanged(nextNodeIDs)
	for _, nodeID := range toAppend {
		m.lastSectorNo++
		m.memberStates[m.lastSectorNo] = &MemberStateEntry{
			NodeID: nodeID,
			State:  MemberStateAppending,
		}
	}
	for sec := range toRemove {
		m.memberStates[sec].State = MemberStateRemoving
	}

	m.applyMemberSectors()
	m.sendSettingMessage()

	for _, entry := range m.memberStates {
		if entry.State != MemberStateNormal {
			return false
		}
	}
	return true
}

// sectorManageMemberResponse handles a response from a remote node confirming sector membership configuration.
func (m *Manager) sectorManageMemberResponse(srcNodeID *types.NodeID, sectorID kvsTypes.SectorID, sectorNo kvsTypes.SectorNo) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.hostingSectorKey == nil || m.hostingSectorKey.SectorID != sectorID {
		return
	}

	if entry, ok := m.memberStates[sectorNo]; ok && entry.NodeID.Equal(srcNodeID) {
		switch entry.State {
		case MemberStateCreating, MemberStateAppending:
			entry.State = MemberStateConfiguredNode
		case MemberStateConfiguredConsensus:
			entry.State = MemberStateNormal
		case MemberStateNormal, MemberStateConfiguredNode:
			// do nothing
		default:
			m.logger.Warn("Unexpected state for Raft config response", "sectorNo", sectorNo, "state", entry.State, "nodeID", srcNodeID.String())
		}
	}
}

// OnSectorAppendNode handles the Raft consensus notification that a node was appended.
func (m *Manager) OnSectorAppendNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.hostingSectorKey == nil || *sectorKey != *m.hostingSectorKey {
		return
	}

	member, ok := m.memberStates[sectorNo]
	if !ok {
		m.logger.Warn("Unknown sectorNo for appending", "sectorNo", sectorNo, "nodeID", nodeID)
		m.memberStates[sectorNo] = &MemberStateEntry{
			NodeID: nodeID,
			State:  MemberStateRemoving,
		}
		return
	}

	if sectorNo == kvsTypes.HostNodeSectorNo {
		if member.State == MemberStateCreating {
			member.State = MemberStateNormal
		} else {
			m.logger.Warn("Unexpected state for host node", "sectorNo", sectorNo, "state", member.State, "nodeID", nodeID)
		}
		return
	}

	switch member.State {
	case MemberStateCreating, MemberStateAppending:
		member.State = MemberStateConfiguredConsensus
	case MemberStateConfiguredNode:
		member.State = MemberStateNormal
	default:
		m.logger.Warn("Unexpected state for appending node", "sectorNo", sectorNo, "state", member.State, "nodeID", nodeID)
	}
}

// OnSectorRemoveNode handles the Raft consensus notification that a node was removed from the hosting sector.
func (m *Manager) OnSectorRemoveNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.hostingSectorKey != nil && *sectorKey == *m.hostingSectorKey {
		delete(m.memberStates, sectorNo)
	}
}

func (m *Manager) initHostSector(nextNodeIDs []*types.NodeID) {
	sectorID, err := uuid.NewV7()
	if err != nil {
		panic("Failed to create new sector ID")
	}

	m.lastSectorNo = kvsTypes.HostNodeSectorNo
	members := make(map[kvsTypes.SectorNo]*types.NodeID)
	members[kvsTypes.HostNodeSectorNo] = m.localNodeID
	m.memberStates[kvsTypes.HostNodeSectorNo] = &MemberStateEntry{
		NodeID: m.localNodeID,
		State:  MemberStateCreating,
	}
	for _, nodeID := range nextNodeIDs {
		if nodeID.Equal(m.localNodeID) {
			panic("localNodeID found in nextNodeIDs")
		}
		m.lastSectorNo++
		members[m.lastSectorNo] = nodeID
		m.memberStates[m.lastSectorNo] = &MemberStateEntry{
			NodeID: nodeID,
			State:  MemberStateCreating,
		}
	}

	sectorKey := &kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(sectorID),
		SectorNo: kvsTypes.HostNodeSectorNo,
	}
	m.hostingSectorKey = sectorKey
	m.handler.AllocateSector(sectorKey, m.localNodeID, true, false, members)
}

func (m *Manager) getNodesToBeChanged(nextNodeIDs []*types.NodeID) ([]*types.NodeID, map[kvsTypes.SectorNo]struct{}) {
	nextNodeIDMap := make(map[types.NodeID]struct{})
	for _, nodeID := range nextNodeIDs {
		nextNodeIDMap[*nodeID] = struct{}{}
	}

	toAppend := make([]*types.NodeID, 0)
	memberMap := make(map[types.NodeID]kvsTypes.SectorNo)
	for sec, entry := range m.memberStates {
		if entry.State == MemberStateRemoving {
			continue
		}
		memberMap[*entry.NodeID] = sec
	}
	for nodeID := range nextNodeIDMap {
		if _, ok := memberMap[nodeID]; !ok {
			toAppend = append(toAppend, &nodeID)
		}
	}

	toRemove := make(map[kvsTypes.SectorNo]struct{})
	for sec, entry := range m.memberStates {
		if entry.State == MemberStateRemoving || sec == kvsTypes.HostNodeSectorNo {
			continue
		}
		if _, ok := nextNodeIDMap[*entry.NodeID]; !ok {
			toRemove[sec] = struct{}{}
		}
	}
	return toAppend, toRemove
}

func (m *Manager) applyMemberSectors() {
	for sectorNo, member := range m.memberStates {
		switch member.State {
		case MemberStateAppending:
			m.handler.ApplyAppendNode(*m.hostingSectorKey, sectorNo, member.NodeID)
		case MemberStateRemoving:
			m.handler.ApplyRemoveNode(*m.hostingSectorKey, sectorNo)
		}
	}
}

func (m *Manager) sendSettingMessage() {
	for sectorNo, ms := range m.memberStates {
		if sectorNo == kvsTypes.HostNodeSectorNo {
			continue
		}

		switch ms.State {
		case MemberStateNormal, MemberStateRemoving, MemberStateConfiguredNode:
			continue

		case MemberStateCreating, MemberStateAppending, MemberStateConfiguredConsensus:
			members := make(map[kvsTypes.SectorNo]*types.NodeID)
			members[kvsTypes.HostNodeSectorNo] = m.localNodeID
			for sn, ms := range m.memberStates {
				members[sn] = ms.NodeID
			}

			var command proto.SectorManageMember_Command
			if ms.State == MemberStateCreating {
				command = proto.SectorManageMember_COMMAND_CREATE
			} else { // MemberStateAppending or MemberStateConfiguredConsensus
				command = proto.SectorManageMember_COMMAND_APPEND
			}

			go m.outbound.sendSectorManageMember(&SectorManageMemberParam{
				DstNodeID: ms.NodeID,
				SectorID:  m.hostingSectorKey.SectorID,
				SectorNo:  sectorNo,
				Command:   command,
				Members:   members,
			})
		}
	}
}
