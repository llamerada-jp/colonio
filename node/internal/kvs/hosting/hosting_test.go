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
	"testing"
	"time"

	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sectorHandlerHelper struct{}

var _ SectorHandler = &sectorHandlerHelper{}

func (h *sectorHandlerHelper) HostingAllocateSector(sectorKey *kvsTypes.SectorKey, head *types.NodeID, isHosting bool, join bool, members map[kvsTypes.SectorNo]*types.NodeID) {
}

func (h *sectorHandlerHelper) HostingApplyAppendNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
}

func (h *sectorHandlerHelper) HostingApplyRemoveNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo) {
}

type outboundHelper struct{}

var _ OutboundPort = &outboundHelper{}

func (o *outboundHelper) sendSectorManageMember(param *SectorManageMemberParam) {}

// TestManager_ManageMember_panicsOnLocalNodeID covers the guard against the
// local node in nextNodeIDs, both before and after the hosting sector exists.
func TestManager_ManageMember_panicsOnLocalNodeID(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, otherNodeID := nodeIDs[0], nodeIDs[1]

	newManager := func() *Manager {
		m := NewManager(&Config{
			Logger:   testUtil.Logger(t),
			Outbound: &outboundHelper{},
		})
		m.Start(&sectorHandlerHelper{}, localNodeID)
		return m
	}

	t.Run("before the hosting sector exists", func(t *testing.T) {
		m := newManager()
		require.PanicsWithValue(t, "localNodeID found in nextNodeIDs", func() {
			m.ManageMember([]*types.NodeID{otherNodeID, localNodeID})
		})
	})

	t.Run("after the hosting sector exists", func(t *testing.T) {
		m := newManager()
		m.ManageMember([]*types.NodeID{otherNodeID})
		require.NotNil(t, m.GetHostingSectorKey())

		require.PanicsWithValue(t, "localNodeID found in nextNodeIDs", func() {
			m.ManageMember([]*types.NodeID{otherNodeID, localNodeID})
		})
	})
}

// TestManager_ManageMember_reapsStaleMember covers a member that never
// finishes its setup: after memberSetupTimeout it is marked Removing, and the
// node is added again under a new sectorNo.
func TestManager_ManageMember_reapsStaleMember(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, memberNodeID := nodeIDs[0], nodeIDs[1]

	m := NewManager(&Config{
		Logger:   testUtil.Logger(t),
		Outbound: &outboundHelper{},
	})
	m.memberSetupTimeout = 50 * time.Millisecond
	m.Start(&sectorHandlerHelper{}, localNodeID)

	sectorNosOf := func(nodeID *types.NodeID) map[kvsTypes.SectorNo]MemberState {
		m.mtx.RLock()
		defer m.mtx.RUnlock()
		sectorNos := map[kvsTypes.SectorNo]MemberState{}
		for sectorNo, entry := range m.memberStates {
			if entry.NodeID.Equal(nodeID) {
				sectorNos[sectorNo] = entry.State
			}
		}
		return sectorNos
	}

	m.ManageMember([]*types.NodeID{memberNodeID})
	before := sectorNosOf(memberNodeID)
	require.Len(t, before, 1)
	var oldSectorNo kvsTypes.SectorNo
	for sectorNo, state := range before {
		oldSectorNo = sectorNo
		require.Equal(t, MemberStateCreating, state)
	}

	// before the timeout nothing changes
	m.ManageMember([]*types.NodeID{memberNodeID})
	require.Equal(t, before, sectorNosOf(memberNodeID))

	time.Sleep(100 * time.Millisecond)
	m.ManageMember([]*types.NodeID{memberNodeID})

	after := sectorNosOf(memberNodeID)
	require.Len(t, after, 2)
	require.Equal(t, MemberStateRemoving, after[oldSectorNo])
	for sectorNo, state := range after {
		if sectorNo != oldSectorNo {
			require.Greater(t, sectorNo, oldSectorNo)
			require.Equal(t, MemberStateAppending, state)
		}
	}
}

// TestManager_ManageMember_keepsNormalMember covers a member that finished its
// setup: it must not be reaped however long it has been a member.
func TestManager_ManageMember_keepsNormalMember(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, memberNodeID := nodeIDs[0], nodeIDs[1]

	m := NewManager(&Config{
		Logger:   testUtil.Logger(t),
		Outbound: &outboundHelper{},
	})
	m.memberSetupTimeout = 50 * time.Millisecond
	m.Start(&sectorHandlerHelper{}, localNodeID)

	m.ManageMember([]*types.NodeID{memberNodeID})
	sectorKey := m.GetHostingSectorKey()
	require.NotNil(t, sectorKey)
	m.OnSectorAppendNode(sectorKey, kvsTypes.HostNodeSectorNo, localNodeID)
	m.OnSectorAppendNode(sectorKey, kvsTypes.SectorNo(2), memberNodeID)
	m.sectorManageMemberResponse(memberNodeID, sectorKey.SectorID, kvsTypes.SectorNo(2))

	time.Sleep(100 * time.Millisecond)
	require.True(t, m.ManageMember([]*types.NodeID{memberNodeID}))
	require.Len(t, m.memberStates, 2)
}

func TestManager_getNodesToBeChanged(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(5)

	tests := []struct {
		name            string
		memberStates    map[kvsTypes.SectorNo]*MemberStateEntry
		nextNodeIDs     []*types.NodeID
		expectedAppends []*types.NodeID
		expectedRemoves map[kvsTypes.SectorNo]struct{}
	}{
		{
			name:            "empty",
			memberStates:    map[kvsTypes.SectorNo]*MemberStateEntry{},
			nextNodeIDs:     []*types.NodeID{},
			expectedAppends: []*types.NodeID{},
			expectedRemoves: map[kvsTypes.SectorNo]struct{}{},
		},
		{
			name: "new members",
			memberStates: map[kvsTypes.SectorNo]*MemberStateEntry{
				kvsTypes.HostNodeSectorNo: {
					NodeID: nodeIDs[0],
					State:  MemberStateNormal,
				},
			},
			nextNodeIDs:     []*types.NodeID{nodeIDs[1], nodeIDs[2]},
			expectedAppends: []*types.NodeID{nodeIDs[1], nodeIDs[2]},
			expectedRemoves: map[kvsTypes.SectorNo]struct{}{},
		},
		{
			name: "remove members",
			memberStates: map[kvsTypes.SectorNo]*MemberStateEntry{
				1: {
					NodeID: nodeIDs[0],
					State:  MemberStateNormal,
				},
				2: {
					NodeID: nodeIDs[1],
					State:  MemberStateNormal,
				},
				3: { // to be removed
					NodeID: nodeIDs[2],
					State:  MemberStateNormal,
				},
			},
			nextNodeIDs:     []*types.NodeID{nodeIDs[1]},
			expectedAppends: []*types.NodeID{},
			expectedRemoves: map[kvsTypes.SectorNo]struct{}{
				3: {},
			},
		},
		{
			name: "mixed",
			memberStates: map[kvsTypes.SectorNo]*MemberStateEntry{
				1: {
					NodeID: nodeIDs[0],
					State:  MemberStateNormal,
				},
				2: {
					NodeID: nodeIDs[1],
					State:  MemberStateNormal,
				},
				3: { // to be removed
					NodeID: nodeIDs[2],
					State:  MemberStateNormal,
				},
			},
			nextNodeIDs:     []*types.NodeID{nodeIDs[1], nodeIDs[3]},
			expectedAppends: []*types.NodeID{nodeIDs[3]},
			expectedRemoves: map[kvsTypes.SectorNo]struct{}{
				3: {},
			},
		},
		{
			name: "removing node is appended again",
			memberStates: map[kvsTypes.SectorNo]*MemberStateEntry{
				1: {
					NodeID: nodeIDs[0],
					State:  MemberStateNormal,
				},
				2: {
					NodeID: nodeIDs[1],
					State:  MemberStateRemoving,
				},
				3: {
					NodeID: nodeIDs[2],
					State:  MemberStateRemoving,
				},
			},
			nextNodeIDs:     []*types.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[3]},
			expectedAppends: []*types.NodeID{nodeIDs[1], nodeIDs[3]},
			expectedRemoves: map[kvsTypes.SectorNo]struct{}{},
		},
		{
			name: "appending node is ignored from appends",
			memberStates: map[kvsTypes.SectorNo]*MemberStateEntry{
				1: {
					NodeID: nodeIDs[0],
					State:  MemberStateNormal,
				},
				2: {
					NodeID: nodeIDs[1],
					State:  MemberStateCreating,
				},
				3: {
					NodeID: nodeIDs[2], // to be removed
					State:  MemberStateAppending,
				},
				4: {
					NodeID: nodeIDs[3],
					State:  MemberStateNormal,
				},
			},
			nextNodeIDs: []*types.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[3], nodeIDs[4]},
			expectedAppends: []*types.NodeID{
				nodeIDs[4],
			},
			expectedRemoves: map[kvsTypes.SectorNo]struct{}{
				3: {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{
				memberStates: tt.memberStates,
			}

			appends, removes := m.getNodesToBeChanged(tt.nextNodeIDs)

			require.Len(t, appends, len(tt.expectedAppends))
			for _, appendNodeID := range appends {
				assert.Contains(t, tt.expectedAppends, appendNodeID)
			}
			assert.Equal(t, tt.expectedRemoves, removes)
		})
	}
}
