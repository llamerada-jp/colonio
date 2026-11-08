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
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
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

type outboundHelper struct {
	mtx    sync.Mutex
	params []*SectorManageMemberParam
}

var _ OutboundPort = &outboundHelper{}

func (o *outboundHelper) sendSectorManageMember(param *SectorManageMemberParam) {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	o.params = append(o.params, param)
}

// sent returns the setting messages sent to nodeID with command.
func (o *outboundHelper) sent(nodeID *types.NodeID, command proto.SectorManageMember_Command) []*SectorManageMemberParam {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	params := []*SectorManageMemberParam{}
	for _, param := range o.params {
		if param.DstNodeID.Equal(nodeID) && param.Command == command {
			params = append(params, param)
		}
	}
	return params
}

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

// TestManager_ManageMember_createCarriesInitialMembers covers the member table
// in CREATE. A node that receives CREATE bootstraps its raft node with that
// table, and raft requires every initial member to bootstrap with the same
// configuration. CREATE is sent again until the member answers, so it must
// keep carrying the initial members even after another member is appended;
// otherwise a member whose first CREATE was lost bootstraps with a different
// configuration and silently diverges from the group.
func TestManager_ManageMember_createCarriesInitialMembers(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	localNodeID, initialNodeID, appendedNodeID := nodeIDs[0], nodeIDs[1], nodeIDs[2]

	outbound := &outboundHelper{}
	m := NewManager(&Config{
		Logger:   testUtil.Logger(t),
		Outbound: outbound,
	})
	m.Start(&sectorHandlerHelper{}, localNodeID)

	m.ManageMember([]*types.NodeID{initialNodeID})
	// the initial member has not answered yet when another node is appended
	m.ManageMember([]*types.NodeID{initialNodeID, appendedNodeID})

	initialMembers := map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
		kvsTypes.SectorNo(2):      initialNodeID,
	}
	require.Eventually(t, func() bool {
		return len(outbound.sent(initialNodeID, proto.SectorManageMember_COMMAND_CREATE)) == 2
	}, time.Second, 10*time.Millisecond)
	for _, param := range outbound.sent(initialNodeID, proto.SectorManageMember_COMMAND_CREATE) {
		require.Equal(t, initialMembers, param.Members)
	}

	// APPEND carries the current members, including the appended one
	require.Eventually(t, func() bool {
		return len(outbound.sent(appendedNodeID, proto.SectorManageMember_COMMAND_APPEND)) == 1
	}, time.Second, 10*time.Millisecond)
	appendParam := outbound.sent(appendedNodeID, proto.SectorManageMember_COMMAND_APPEND)[0]
	require.Equal(t, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
		kvsTypes.SectorNo(2):      initialNodeID,
		kvsTypes.SectorNo(3):      appendedNodeID,
	}, appendParam.Members)
}

// TestManager_OnSectorRemoveNode_notifiesRemovedMember covers the removal of a
// member from the hosting sector: the removed node is told directly, since it
// no longer receives anything from the group. A removal in another sector is
// not the host's to notify.
func TestManager_OnSectorRemoveNode_notifiesRemovedMember(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, memberNodeID := nodeIDs[0], nodeIDs[1]

	outbound := &outboundHelper{}
	m := NewManager(&Config{
		Logger:   testUtil.Logger(t),
		Outbound: outbound,
	})
	m.Start(&sectorHandlerHelper{}, localNodeID)

	m.ManageMember([]*types.NodeID{memberNodeID})
	hostingSectorKey := m.GetHostingSectorKey()
	require.NotNil(t, hostingSectorKey)
	memberSectorNo := kvsTypes.SectorNo(2)

	otherKey := &kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(uuid.New()),
		SectorNo: kvsTypes.HostNodeSectorNo,
	}
	m.OnSectorRemoveNode(otherKey, memberSectorNo)

	m.OnSectorRemoveNode(hostingSectorKey, memberSectorNo)
	m.mtx.RLock()
	require.NotContains(t, m.memberStates, memberSectorNo)
	m.mtx.RUnlock()

	require.Eventually(t, func() bool {
		return len(outbound.sent(memberNodeID, proto.SectorManageMember_COMMAND_REMOVE)) == 1
	}, time.Second, 10*time.Millisecond)
	param := outbound.sent(memberNodeID, proto.SectorManageMember_COMMAND_REMOVE)[0]
	require.Equal(t, hostingSectorKey.SectorID, param.SectorID)
	require.Equal(t, memberSectorNo, param.SectorNo)

	time.Sleep(100 * time.Millisecond)
	require.Len(t, outbound.sent(memberNodeID, proto.SectorManageMember_COMMAND_REMOVE), 1)
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
