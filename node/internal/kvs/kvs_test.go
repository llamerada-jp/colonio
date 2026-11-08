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
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/kvs/hosting"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
	"github.com/stretchr/testify/require"
)

type transfererHandlerHelper struct {
	mtx        sync.Mutex
	sendPacket []*networkTypes.Packet
	// relay packet is not used in this test
}

var _ transferer.Handler = &transfererHandlerHelper{}

func (h *transfererHandlerHelper) TransfererSendPacket(p *networkTypes.Packet) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.sendPacket = append(h.sendPacket, p)
}

func (h *transfererHandlerHelper) TransfererRelayPacket(nid *types.NodeID, p *networkTypes.Packet) {
	panic("not used in this test")
}

type kvsHandlerHelper struct {
	mtx                  sync.Mutex
	isStable             bool
	backwardNextNodeIDs  []*types.NodeID
	frontwardNextNodeIDs []*types.NodeID
}

var _ Handler = &kvsHandlerHelper{}

func (h *kvsHandlerHelper) KvsGetStability() (bool, []*types.NodeID, []*types.NodeID) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.isStable, h.backwardNextNodeIDs, h.frontwardNextNodeIDs
}

type kvsOutboundHelper struct{}

var _ OutboundPort = &kvsOutboundHelper{}

func (o *kvsOutboundHelper) sendKvsOperation(param *operationParam) {}

func (o *kvsOutboundHelper) sendSectorManageMember(param *SectorManageMemberParam) {}

func (o *kvsOutboundHelper) sendSectorActivate(param *SectorActivateParam) chan error {
	c := make(chan error, 1)
	c <- nil
	return c
}

func (o *kvsOutboundHelper) sendSectorPrepareSplit(param *SectorSplitParam) chan error {
	c := make(chan error, 1)
	c <- nil
	return c
}

// newTestKVS builds a KVS whose hosting sector runs a real single-voter Raft
// group, without the network/seed dependencies of node.NewNode.
func newTestKVS(t *testing.T, ctx context.Context, localNodeID *types.NodeID, handler *kvsHandlerHelper) *KVS {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tr := transferer.NewTransferer(&transferer.Config{
		Logger:  logger,
		Handler: &transfererHandlerHelper{},
	})
	tr.Start(ctx, localNodeID)

	hostingManager := hosting.NewManager(&hosting.Config{
		Logger:   logger,
		Outbound: hosting.NewOutbound(tr),
	})

	k := NewKVS(&Config{
		Logger:            logger,
		Handler:           handler,
		Outbound:          &kvsOutboundHelper{},
		ConsensusOutbound: consensus.NewOutbound(tr),
		HostingManager:    hostingManager,
		Store:             NewSimpleStore(),
	})

	// Equivalent to KVS.Start without the subRoutine loop: the tests drive
	// each step explicitly to control the timing.
	k.localNodeID = localNodeID
	k.ctx = ctx
	k.hostingManager.Start(k, localNodeID)

	return k
}

// setupHostingSector creates the hosting sector with the local node as the
// only Raft voter and waits until the Raft group can accept proposals.
func setupHostingSector(t *testing.T, k *KVS) *kvsTypes.SectorKey {
	t.Helper()

	k.hostingManager.ManageMember(nil)
	hostingSectorKey := k.hostingManager.GetHostingSectorKey()
	require.NotNil(t, hostingSectorKey)

	// ManageMember returns true once the initial conf change is committed,
	// which also means the single-voter Raft group elected a leader.
	require.Eventually(t, func() bool {
		return k.hostingManager.ManageMember(nil)
	}, 10*time.Second, 100*time.Millisecond)

	return hostingSectorKey
}

// TestKVS_sectorActivate covers the sectorActivate -> activateHostingSector
// path.
func TestKVS_sectorActivate(t *testing.T) {
	tests := []struct {
		name string
		// registerDeadReplica adds an inactive sector replica (head between
		// local and frontward) before triggering activation.
		registerDeadReplica bool
	}{
		{
			// Both the inbound sector-activate handler and
			// activateHostingSector take KVS.mtx, so the handler must not
			// hold it while activating.
			name: "completes without obstruction",
		},
		{
			// A stopped node's sector replica stays in k.sectors as an
			// inactive sector, and its head may lie between the local node
			// and its routing frontward neighbor. Only active sectors can
			// overlap, so such a replica must not block activation (the TLA+
			// ActivateFrontward guard quantifies over Actives only).
			name:                "ignores inactive replica between local and frontward",
			registerDeadReplica: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// ring order: backward < local < (dead) < frontward, possibly
			// wrapping around the ring's zero boundary
			ringNodeIDs := testUtil.UniqueNodeIDsInRingOrder(4)
			backwardNodeID, localNodeID, deadNodeID, frontwardNodeID :=
				ringNodeIDs[0], ringNodeIDs[1], ringNodeIDs[2], ringNodeIDs[3]

			handler := &kvsHandlerHelper{
				isStable: true,
				// the dead node is no longer part of the routing view
				backwardNextNodeIDs:  []*types.NodeID{backwardNodeID},
				frontwardNextNodeIDs: []*types.NodeID{frontwardNodeID},
			}

			k := newTestKVS(t, ctx, localNodeID, handler)
			hostingSectorKey := setupHostingSector(t, k)

			if tt.registerDeadReplica {
				// inactive replica left behind by the dead node
				err := k.sectorManageMember(&sectorManageMemberParam{
					command: proto.SectorManageMember_COMMAND_CREATE,
					sectorKey: kvsTypes.SectorKey{
						SectorID: kvsTypes.SectorID(uuid.New()),
						SectorNo: kvsTypes.SectorNo(2),
					},
					head: deadNodeID,
					members: map[kvsTypes.SectorNo]*types.NodeID{
						kvsTypes.HostNodeSectorNo: deadNodeID,
						kvsTypes.SectorNo(2):      localNodeID,
					},
				})
				require.NoError(t, err)
			}

			// Register the frontward node's sector replica, which is the
			// activation tail candidate. Its Raft group never gets quorum
			// here, but the test only needs its head address to be visible
			// in k.sectors.
			err := k.sectorManageMember(&sectorManageMemberParam{
				command: proto.SectorManageMember_COMMAND_CREATE,
				sectorKey: kvsTypes.SectorKey{
					SectorID: kvsTypes.SectorID(uuid.New()),
					SectorNo: kvsTypes.SectorNo(2),
				},
				head: frontwardNodeID,
				members: map[kvsTypes.SectorNo]*types.NodeID{
					kvsTypes.HostNodeSectorNo: frontwardNodeID,
					kvsTypes.SectorNo(2):      localNodeID,
				},
			})
			require.NoError(t, err)

			done := make(chan bool, 1)
			go func() {
				done <- k.sectorActivate(backwardNodeID, hostingSectorKey.SectorID)
			}()

			select {
			case ok := <-done:
				require.True(t, ok)
			case <-time.After(10 * time.Second):
				t.Fatal("sectorActivate did not return: KVS.mtx self-deadlock")
			}

			// The activation proposal must be committed and the hosting
			// sector must become active with the frontward node as its
			// tail.
			k.mtx.RLock()
			hostingSector := k.sectors[*hostingSectorKey]
			k.mtx.RUnlock()
			require.NotNil(t, hostingSector)
			require.Eventually(t, func() bool {
				tail := hostingSector.GetTailAddress()
				return tail != nil && tail.Equal(frontwardNodeID)
			}, 15*time.Second, 100*time.Millisecond)
		})
	}
}

// TestKVS_sectorManageMember_rejectsTombstonedKey covers a setting message
// that arrives again after the local replica was terminated: it must not
// re-create the replica under the same key, since a raft member must never
// restart with an empty log under the same ID. The same node under a new
// sectorNo is accepted.
func TestKVS_sectorManageMember_rejectsTombstonedKey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, headNodeID := nodeIDs[0], nodeIDs[1]
	k := newTestKVS(t, ctx, localNodeID, &kvsHandlerHelper{isStable: true})

	sectorKey := kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(uuid.New()),
		SectorNo: kvsTypes.SectorNo(2),
	}
	manageMember := func(key kvsTypes.SectorKey) error {
		return k.sectorManageMember(&sectorManageMemberParam{
			command:   proto.SectorManageMember_COMMAND_APPEND,
			sectorKey: key,
			head:      headNodeID,
			members: map[kvsTypes.SectorNo]*types.NodeID{
				kvsTypes.HostNodeSectorNo: headNodeID,
				key.SectorNo:              localNodeID,
			},
		})
	}
	hasSector := func(key kvsTypes.SectorKey) bool {
		k.mtx.RLock()
		defer k.mtx.RUnlock()
		_, ok := k.sectors[key]
		return ok
	}

	require.NoError(t, manageMember(sectorKey))
	require.True(t, hasSector(sectorKey))

	k.SectorTerminated(&sectorKey)
	require.False(t, hasSector(sectorKey))

	require.Error(t, manageMember(sectorKey))
	require.False(t, hasSector(sectorKey))

	freshKey := kvsTypes.SectorKey{
		SectorID: sectorKey.SectorID,
		SectorNo: kvsTypes.SectorNo(3),
	}
	require.NoError(t, manageMember(freshKey))
	require.True(t, hasSector(freshKey))

	// a member ID the group has removed is never created, even if this node
	// did not hold it
	removedKey := kvsTypes.SectorKey{
		SectorID: sectorKey.SectorID,
		SectorNo: kvsTypes.SectorNo(4),
	}
	k.SectorRemoveNode(&freshKey, removedKey.SectorNo)
	require.Error(t, manageMember(removedKey))
	require.False(t, hasSector(removedKey))
}

// TestKVS_sectorManageMember_remove covers a removal notification from the
// head of a sector: the local replica is destroyed and its key tombstoned, so
// that a setting message delivered again cannot re-create it. A removal for a
// key this node does not hold only leaves a tombstone.
func TestKVS_sectorManageMember_remove(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, headNodeID := nodeIDs[0], nodeIDs[1]
	k := newTestKVS(t, ctx, localNodeID, &kvsHandlerHelper{isStable: true})

	manageMember := func(command proto.SectorManageMember_Command, key kvsTypes.SectorKey) error {
		return k.sectorManageMember(&sectorManageMemberParam{
			command:   command,
			sectorKey: key,
			head:      headNodeID,
			members: map[kvsTypes.SectorNo]*types.NodeID{
				kvsTypes.HostNodeSectorNo: headNodeID,
				key.SectorNo:              localNodeID,
			},
		})
	}
	hasSector := func(key kvsTypes.SectorKey) bool {
		k.mtx.RLock()
		defer k.mtx.RUnlock()
		_, ok := k.sectors[key]
		return ok
	}

	sectorKey := kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(uuid.New()),
		SectorNo: kvsTypes.SectorNo(2),
	}
	require.NoError(t, manageMember(proto.SectorManageMember_COMMAND_APPEND, sectorKey))
	require.True(t, hasSector(sectorKey))

	require.NoError(t, manageMember(proto.SectorManageMember_COMMAND_REMOVE, sectorKey))
	require.Eventually(t, func() bool {
		return !hasSector(sectorKey)
	}, 5*time.Second, 50*time.Millisecond)
	require.Error(t, manageMember(proto.SectorManageMember_COMMAND_APPEND, sectorKey))

	unknownKey := kvsTypes.SectorKey{
		SectorID: sectorKey.SectorID,
		SectorNo: kvsTypes.SectorNo(3),
	}
	require.NoError(t, manageMember(proto.SectorManageMember_COMMAND_REMOVE, unknownKey))
	require.Error(t, manageMember(proto.SectorManageMember_COMMAND_APPEND, unknownKey))
	require.False(t, hasSector(unknownKey))
}

// TestKVS_sectorManageMember_removeRejectsNonHead covers removal notifications
// that do not come from the head of the sector, including one for the local
// hosting sector, whose head is this node: they must not destroy the replica.
func TestKVS_sectorManageMember_removeRejectsNonHead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(3)
	localNodeID, headNodeID, otherNodeID := nodeIDs[0], nodeIDs[1], nodeIDs[2]
	k := newTestKVS(t, ctx, localNodeID, &kvsHandlerHelper{isStable: true})
	hostingSectorKey := setupHostingSector(t, k)

	sectorKey := kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(uuid.New()),
		SectorNo: kvsTypes.SectorNo(2),
	}
	require.NoError(t, k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_APPEND,
		sectorKey: sectorKey,
		head:      headNodeID,
		members: map[kvsTypes.SectorNo]*types.NodeID{
			kvsTypes.HostNodeSectorNo: headNodeID,
			kvsTypes.SectorNo(2):      localNodeID,
		},
	}))

	for _, key := range []kvsTypes.SectorKey{*hostingSectorKey, sectorKey} {
		require.Error(t, k.sectorManageMember(&sectorManageMemberParam{
			command:   proto.SectorManageMember_COMMAND_REMOVE,
			sectorKey: key,
			head:      otherNodeID,
		}))
	}

	time.Sleep(200 * time.Millisecond)
	k.mtx.RLock()
	defer k.mtx.RUnlock()
	for _, key := range []kvsTypes.SectorKey{*hostingSectorKey, sectorKey} {
		require.Contains(t, k.sectors, key)
		require.NotContains(t, k.sectorTombstones, key)
	}
}

// TestKVS_sectorPrepareSplit_noHostingSector covers a prepare-split request
// that arrives while this node has no hosting sector, as between a termination
// of the hosting sector and its re-creation: it must be rejected.
func TestKVS_sectorPrepareSplit_noHostingSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeIDs := testUtil.UniqueNodeIDs(2)
	localNodeID, srcNodeID := nodeIDs[0], nodeIDs[1]
	k := newTestKVS(t, ctx, localNodeID, &kvsHandlerHelper{isStable: true})

	require.Nil(t, k.hostingManager.GetHostingSectorKey())
	require.False(t, k.sectorPrepareSplit(srcNodeID, kvsTypes.SectorID(uuid.New())))
}

// TestKVS_sectorPrepareSplit_onlyFromNearestBackward covers who may propose a
// split of the hosting sector: only the nearest backward node, whose hosting
// sector covers this node. A proposal from any other node is rejected and
// leaves no proposal recorded.
func TestKVS_sectorPrepareSplit_onlyFromNearestBackward(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ring order: far backward < near backward < local < frontward
	ringNodeIDs := testUtil.UniqueNodeIDsInRingOrder(4)
	farNodeID, nearNodeID, localNodeID, frontwardNodeID :=
		ringNodeIDs[0], ringNodeIDs[1], ringNodeIDs[2], ringNodeIDs[3]

	k := newTestKVS(t, ctx, localNodeID, &kvsHandlerHelper{
		isStable:             true,
		backwardNextNodeIDs:  []*types.NodeID{farNodeID, nearNodeID},
		frontwardNextNodeIDs: []*types.NodeID{frontwardNodeID},
	})
	hostingSectorKey := setupHostingSector(t, k)

	for _, srcNodeID := range []*types.NodeID{farNodeID, frontwardNodeID} {
		require.False(t, k.sectorPrepareSplit(srcNodeID, hostingSectorKey.SectorID))
		require.Nil(t, k.proposedSplittingNodeID)
	}

	require.True(t, k.sectorPrepareSplit(nearNodeID, hostingSectorKey.SectorID))
	require.True(t, k.proposedSplittingNodeID.Equal(nearNodeID))
}

func TestNearestBackward(t *testing.T) {
	id := func(n uint64) *types.NodeID { return types.NewNormalNodeID(n, 0) }

	tests := []struct {
		name    string
		local   *types.NodeID
		nodeIDs []*types.NodeID
		expect  *types.NodeID
	}{
		{
			name:    "empty",
			local:   id(5),
			nodeIDs: nil,
			expect:  nil,
		},
		{
			name:    "closest below",
			local:   id(5),
			nodeIDs: []*types.NodeID{id(2), id(4), id(3)},
			expect:  id(4),
		},
		{
			name:    "wraps around zero",
			local:   id(1),
			nodeIDs: []*types.NodeID{id(8), id(9), id(7)},
			expect:  id(9),
		},
		{
			name:    "below is closer than wrapped",
			local:   id(3),
			nodeIDs: []*types.NodeID{id(9), id(2)},
			expect:  id(2),
		},
		{
			name:    "local itself is ignored",
			local:   id(5),
			nodeIDs: []*types.NodeID{id(5), id(3)},
			expect:  id(3),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nearestBackward(tt.local, tt.nodeIDs)
			if tt.expect == nil {
				require.Nil(t, got)
				return
			}
			require.True(t, tt.expect.Equal(got), "got %v", got)
		})
	}
}

// TestKVS_activateHostingSector_singleNode covers a lone node: it activates its
// own sector over the whole ring, although its hosting sector is in k.sectors.
func TestKVS_activateHostingSector_singleNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := testUtil.UniqueNodeIDs(1)[0]

	handler := &kvsHandlerHelper{
		isStable: true,
	}

	k := newTestKVS(t, ctx, localNodeID, handler)
	hostingSectorKey := setupHostingSector(t, k)

	k.mtx.RLock()
	hostingSector := k.sectors[*hostingSectorKey]
	k.mtx.RUnlock()
	require.NotNil(t, hostingSector)

	done := make(chan struct{})
	go func() {
		defer close(done)
		k.activateHostingSector(hostingSector, nil, false)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("activateHostingSector did not return: KVS.mtx self-deadlock")
	}

	// The lone node covers the whole ring: tail == its own node ID.
	require.Eventually(t, func() bool {
		tail := hostingSector.GetTailAddress()
		return tail != nil && tail.Equal(localNodeID)
	}, 15*time.Second, 100*time.Millisecond)
}
