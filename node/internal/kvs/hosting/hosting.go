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
	"time"

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
	// Since is when the entry entered the current state; members that stay in
	// a non-Normal state longer than memberSetupTimeout are removed and later
	// re-added under a fresh sectorNo.
	Since time.Time
}

// SectorHandler is implemented by KVS to perform sector operations on behalf of the Manager.
type SectorHandler interface {
	HostingAllocateSector(sectorKey *kvsTypes.SectorKey, head *types.NodeID, isHosting bool, join bool, members map[kvsTypes.SectorNo]*types.NodeID)
	HostingApplyAppendNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID)
	HostingApplyRemoveNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo)
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
	// memberSetupTimeout bounds how long a member may stay in a non-Normal
	// state. A member that cannot finish its setup (e.g. a learner that never
	// catches up, or a target that rejects a re-delivered setting message
	// because the sector key is tombstoned) is removed; if the node is still
	// in the routing view it is re-added under a FRESH sectorNo, which is the
	// only safe way to restart a raft member from an empty log.
	memberSetupTimeout time.Duration

	mtx              sync.RWMutex // protects: hostingSectorKey, lastSectorNo, memberStates
	hostingSectorKey *kvsTypes.SectorKey
	lastSectorNo     kvsTypes.SectorNo
	memberStates     map[kvsTypes.SectorNo]*MemberStateEntry
}

func NewManager(conf *Config) *Manager {
	return &Manager{
		logger:             conf.Logger,
		outbound:           conf.Outbound,
		memberSetupTimeout: 30 * time.Second,
		memberStates:       make(map[kvsTypes.SectorNo]*MemberStateEntry),
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
	// Invariant: routing never lists the local node as its own neighbor
	// (routing1D excludes localNodeID from the neighbor views). A violation is
	// a logic error; detect it here like initHostSector does. Note the check
	// must be on the input: inside getNodesToBeChanged the local node is
	// already in memberMap (host slot), which would silently mask it.
	for _, nodeID := range nextNodeIDs {
		if nodeID.Equal(m.localNodeID) {
			panic("localNodeID found in nextNodeIDs")
		}
	}

	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.hostingSectorKey == nil {
		m.initHostSector(nextNodeIDs)
	}

	m.reapStaleMembers()

	toAppend, toRemove := m.getNodesToBeChanged(nextNodeIDs)
	for _, nodeID := range toAppend {
		m.lastSectorNo++
		m.memberStates[m.lastSectorNo] = &MemberStateEntry{
			NodeID: nodeID,
			State:  MemberStateAppending,
			Since:  time.Now(),
		}
	}
	for sec := range toRemove {
		m.memberStates[sec].State = MemberStateRemoving
		m.memberStates[sec].Since = time.Now()
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

// reapStaleMembers marks members that stayed in a setup state (non-Normal)
// longer than memberSetupTimeout as Removing. Removing frees the raft member
// ID; if the node is still in the routing view, getNodesToBeChanged re-adds it
// under a fresh sectorNo. This is the recovery path for learners that never
// catch up (dead/slow nodes) and for targets that reject re-delivered setting
// messages because their local replica was terminated (sector tombstone).
// Call with m.mtx locked.
func (m *Manager) reapStaleMembers() {
	now := time.Now()
	for sec, entry := range m.memberStates {
		if sec == kvsTypes.HostNodeSectorNo ||
			entry.State == MemberStateNormal || entry.State == MemberStateRemoving {
			continue
		}
		if entry.Since.IsZero() {
			entry.Since = now
			continue
		}
		if now.Sub(entry.Since) < m.memberSetupTimeout {
			continue
		}
		m.logger.Info("remove a member that could not finish setup",
			"sectorNo", sec, "nodeID", entry.NodeID.String(), "state", entry.State)
		entry.State = MemberStateRemoving
		entry.Since = now
	}
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
			entry.Since = time.Now()
		case MemberStateConfiguredConsensus:
			entry.State = MemberStateNormal
			entry.Since = time.Now()
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
			Since:  time.Now(),
		}
		return
	}

	if sectorNo == kvsTypes.HostNodeSectorNo {
		if member.State == MemberStateCreating {
			member.State = MemberStateNormal
			member.Since = time.Now()
		} else {
			m.logger.Warn("Unexpected state for host node", "sectorNo", sectorNo, "state", member.State, "nodeID", nodeID)
		}
		return
	}

	switch member.State {
	case MemberStateCreating, MemberStateAppending:
		member.State = MemberStateConfiguredConsensus
		member.Since = time.Now()
	case MemberStateConfiguredNode:
		member.State = MemberStateNormal
		member.Since = time.Now()
	default:
		m.logger.Warn("Unexpected state for appending node", "sectorNo", sectorNo, "state", member.State, "nodeID", nodeID)
	}
}

// OnSectorRemoveNode handles the Raft consensus notification that a node was removed from the hosting sector.
func (m *Manager) OnSectorRemoveNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.hostingSectorKey == nil || *sectorKey != *m.hostingSectorKey {
		return
	}

	// Tell the removed node about its removal out-of-band: once the removal
	// applies, the group stops messaging the removed member, so it can never
	// learn of the removal from the raft log itself. Without this it keeps its
	// replica with stale state until the leaderless force-terminate backstop
	// reaps it 30-60s later (シミュレーション 2026-07-06: この残留 replica の
	// tail 不一致が残存する赤描画の主因だった). Best-effort one-shot: a lost
	// packet just falls back to the force-terminate path.
	if entry, ok := m.memberStates[sectorNo]; ok && !entry.NodeID.Equal(m.localNodeID) {
		go m.outbound.sendSectorManageMember(&SectorManageMemberParam{
			DstNodeID: entry.NodeID,
			SectorID:  m.hostingSectorKey.SectorID,
			SectorNo:  sectorNo,
			Command:   proto.SectorManageMember_COMMAND_REMOVE,
		})
	}

	delete(m.memberStates, sectorNo)
}

func (m *Manager) OnSectorTerminated(sectorKey *kvsTypes.SectorKey) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.hostingSectorKey != nil && *sectorKey == *m.hostingSectorKey {
		m.hostingSectorKey = nil
		m.lastSectorNo = 0
		m.memberStates = make(map[kvsTypes.SectorNo]*MemberStateEntry)
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
		Since:  time.Now(),
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
			Since:  time.Now(),
		}
	}

	sectorKey := &kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(sectorID),
		SectorNo: kvsTypes.HostNodeSectorNo,
	}
	m.hostingSectorKey = sectorKey
	m.handler.HostingAllocateSector(sectorKey, m.localNodeID, true, false, members)
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
			m.handler.HostingApplyAppendNode(*m.hostingSectorKey, sectorNo, member.NodeID)
		case MemberStateRemoving:
			m.handler.HostingApplyRemoveNode(*m.hostingSectorKey, sectorNo)
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
