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
	mtx  sync.Mutex
	sent []*SectorManageMemberParam
}

var _ OutboundPort = &outboundHelper{}

func (o *outboundHelper) sendSectorManageMember(param *SectorManageMemberParam) {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	o.sent = append(o.sent, param)
}

func (o *outboundHelper) sentByCommand(command proto.SectorManageMember_Command) []*SectorManageMemberParam {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	params := []*SectorManageMemberParam{}
	for _, p := range o.sent {
		if p.Command == command {
			params = append(params, p)
		}
	}
	return params
}

// TestManager_ManageMember_panicsOnLocalNodeID asserts the invariant guard:
// routing must never list the local node as its own neighbor, and a violation
// is a logic error that should be detected loudly on the ManageMember path,
// same as initHostSector.
func TestManager_ManageMember_panicsOnLocalNodeID(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID := nodeIDs[0]
	otherNodeID := nodeIDs[1]

	m := NewManager(&Config{
		Logger:   testUtil.Logger(t),
		Outbound: &outboundHelper{},
	})
	m.Start(&sectorHandlerHelper{}, localNodeID)

	// normal path: initialize the hosting sector with a proper neighbor
	m.ManageMember([]*types.NodeID{otherNodeID})

	require.PanicsWithValue(t, "localNodeID found in nextNodeIDs", func() {
		m.ManageMember([]*types.NodeID{otherNodeID, localNodeID})
	})
}

// TestManager_ManageMember_reapsStaleMember verifies the recovery path for
// members that cannot finish their setup (dead/slow learners, or targets that
// reject re-delivered setting messages due to a sector tombstone): after
// memberSetupTimeout the member is marked Removing, and the node is re-added
// under a FRESH sectorNo on the following tick.
func TestManager_ManageMember_reapsStaleMember(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID := nodeIDs[0]
	memberNodeID := nodeIDs[1]

	m := NewManager(&Config{
		Logger:   testUtil.Logger(t),
		Outbound: &outboundHelper{},
	})
	m.memberSetupTimeout = 50 * time.Millisecond
	m.Start(&sectorHandlerHelper{}, localNodeID)

	m.ManageMember([]*types.NodeID{memberNodeID})

	var memberSectorNo kvsTypes.SectorNo
	for sec, entry := range m.memberStates {
		if entry.NodeID.Equal(memberNodeID) {
			memberSectorNo = sec
		}
	}
	require.NotZero(t, memberSectorNo)
	require.Equal(t, MemberStateCreating, m.memberStates[memberSectorNo].State)

	// the member never finishes its setup; after the timeout it must be
	// reaped and re-added under a fresh sectorNo
	time.Sleep(100 * time.Millisecond)
	m.ManageMember([]*types.NodeID{memberNodeID})

	require.Equal(t, MemberStateRemoving, m.memberStates[memberSectorNo].State)
	var newSectorNo kvsTypes.SectorNo
	for sec, entry := range m.memberStates {
		if sec != memberSectorNo && entry.NodeID.Equal(memberNodeID) {
			newSectorNo = sec
		}
	}
	require.NotZero(t, newSectorNo)
	require.Greater(t, newSectorNo, memberSectorNo)
	require.Equal(t, MemberStateAppending, m.memberStates[newSectorNo].State)
}

// TestManager_OnSectorRemoveNode_notifiesRemovedMember verifies the
// out-of-band removal notification (シミュレーション 2026-07-06): once the
// RemoveNode conf change applies, the group stops messaging the removed
// member, so the host must tell it directly — otherwise its replica lingers
// with stale state until the 30s leaderless force terminate reaps it, which
// showed up as the dominant red (replica tail mismatch) in the simulator.
func TestManager_OnSectorRemoveNode_notifiesRemovedMember(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID := nodeIDs[0]
	memberNodeID := nodeIDs[1]

	outbound := &outboundHelper{}
	m := NewManager(&Config{
		Logger:   testUtil.Logger(t),
		Outbound: outbound,
	})
	m.Start(&sectorHandlerHelper{}, localNodeID)

	m.ManageMember([]*types.NodeID{memberNodeID})
	hostingSectorKey := m.GetHostingSectorKey()
	require.NotNil(t, hostingSectorKey)
	var memberSectorNo kvsTypes.SectorNo
	for sec, entry := range m.memberStates {
		if entry.NodeID.Equal(memberNodeID) {
			memberSectorNo = sec
		}
	}
	require.NotZero(t, memberSectorNo)

	// the removal conf change applies on the host
	m.OnSectorRemoveNode(hostingSectorKey, memberSectorNo)

	// the member entry is gone and the removed node is notified (async)
	m.mtx.RLock()
	_, exists := m.memberStates[memberSectorNo]
	m.mtx.RUnlock()
	require.False(t, exists)
	require.Eventually(t, func() bool {
		return len(outbound.sentByCommand(proto.SectorManageMember_COMMAND_REMOVE)) == 1
	}, 5*time.Second, 50*time.Millisecond)
	sent := outbound.sentByCommand(proto.SectorManageMember_COMMAND_REMOVE)[0]
	require.True(t, sent.DstNodeID.Equal(memberNodeID))
	require.Equal(t, hostingSectorKey.SectorID, sent.SectorID)
	require.Equal(t, memberSectorNo, sent.SectorNo)

	// a removal for a foreign sector must not notify anyone
	foreignKey := &kvsTypes.SectorKey{
		SectorID: hostingSectorKey.SectorID,
		SectorNo: kvsTypes.SectorNo(99),
	}
	m.OnSectorRemoveNode(foreignKey, memberSectorNo)
	time.Sleep(100 * time.Millisecond)
	require.Len(t, outbound.sentByCommand(proto.SectorManageMember_COMMAND_REMOVE), 1)
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
