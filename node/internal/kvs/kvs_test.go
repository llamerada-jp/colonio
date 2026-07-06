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

// TestKVS_sectorActivate_completes is a regression test: handling an inbound
// sector-activate request used to self-deadlock on KVS.mtx (the request
// handler held the write lock while activateHostingSector tried to lock it
// again), freezing the node and leaving every sector after the first one
// inactive.
func TestKVS_sectorActivate_completes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ring order: backwardNodeID < localNodeID < frontwardNodeID
	backwardNodeID := types.NewNormalNodeID(0x1000000000000000, 0)
	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	frontwardNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &kvsHandlerHelper{
		isStable:             true,
		backwardNextNodeIDs:  []*types.NodeID{backwardNodeID},
		frontwardNextNodeIDs: []*types.NodeID{frontwardNodeID},
	}

	k := newTestKVS(t, ctx, localNodeID, handler)
	hostingSectorKey := setupHostingSector(t, k)

	// Register the frontward node's sector replica, which is the activation
	// tail candidate. Its Raft group never gets quorum here, but the test
	// only needs its head address to be visible in k.sectors.
	frontwardSectorKey := kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(uuid.New()),
		SectorNo: kvsTypes.SectorNo(2),
	}
	err := k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_CREATE,
		sectorKey: frontwardSectorKey,
		head:      frontwardNodeID,
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

	// The activation proposal must be committed and the hosting sector must
	// become active with the frontward node as its tail.
	k.mtx.RLock()
	hostingSector := k.sectors[*hostingSectorKey]
	k.mtx.RUnlock()
	require.NotNil(t, hostingSector)
	require.Eventually(t, func() bool {
		tail := hostingSector.GetTailAddress()
		return tail != nil && tail.Equal(frontwardNodeID)
	}, 15*time.Second, 100*time.Millisecond)
}

// TestKVS_sectorActivate_ignoresInactiveSectorBetween is a regression test
// for a stall found in the simulator: a stopped node's sector replica stays
// in k.sectors as an inactive sector. When such a replica's head lies between
// the local node and its routing frontward neighbor, the overlap guard used
// to treat it like an active sector and rejected activation forever, freezing
// the activation chain at that point. Only ACTIVE sectors can overlap, so
// inactive replicas must not block activation (TLA+ ActivateFrontward guard
// quantifies over Actives only).
func TestKVS_sectorActivate_ignoresInactiveSectorBetween(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ring order: backward < local < dead < frontward
	backwardNodeID := types.NewNormalNodeID(0x1000000000000000, 0)
	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadNodeID := types.NewNormalNodeID(0x6000000000000000, 0)
	frontwardNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &kvsHandlerHelper{
		isStable: true,
		// the dead node is no longer part of the routing view
		backwardNextNodeIDs:  []*types.NodeID{backwardNodeID},
		frontwardNextNodeIDs: []*types.NodeID{frontwardNodeID},
	}

	k := newTestKVS(t, ctx, localNodeID, handler)
	hostingSectorKey := setupHostingSector(t, k)

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

	// replica of the live frontward node's sector (the tail candidate)
	err = k.sectorManageMember(&sectorManageMemberParam{
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
		t.Fatal("sectorActivate did not return")
	}

	k.mtx.RLock()
	hostingSector := k.sectors[*hostingSectorKey]
	k.mtx.RUnlock()
	require.NotNil(t, hostingSector)
	require.Eventually(t, func() bool {
		tail := hostingSector.GetTailAddress()
		return tail != nil && tail.Equal(frontwardNodeID)
	}, 15*time.Second, 100*time.Millisecond)
}

// TestKVS_sectorManageMember_rejectsTombstonedKey is a regression test for the
// raft panic observed in the simulator (run5, 2026-07-04): a replica that was
// locally terminated must never be re-created under the same
// {sectorID, sectorNo} with an empty log, because the group still remembers
// that raft member's progress and votes. Re-delivered SectorManageMember
// messages for a tombstoned key must be rejected; re-adding the node under a
// fresh sectorNo is the only safe path.
func TestKVS_sectorManageMember_rejectsTombstonedKey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	headNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &kvsHandlerHelper{isStable: true}
	k := newTestKVS(t, ctx, localNodeID, handler)

	sectorKey := kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(uuid.New()),
		SectorNo: kvsTypes.SectorNo(2),
	}
	members := map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: headNodeID,
		kvsTypes.SectorNo(2):      localNodeID,
	}

	require.NoError(t, k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_APPEND,
		sectorKey: sectorKey,
		head:      headNodeID,
		members:   members,
	}))

	// simulate the local (force) termination of the replica
	k.SectorTerminated(&sectorKey)
	require.Eventually(t, func() bool {
		k.mtx.RLock()
		defer k.mtx.RUnlock()
		_, ok := k.sectors[sectorKey]
		return !ok
	}, 5*time.Second, 50*time.Millisecond)

	// a re-delivered setting message for the same key must be rejected
	err := k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_APPEND,
		sectorKey: sectorKey,
		head:      headNodeID,
		members:   members,
	})
	require.Error(t, err)
	k.mtx.RLock()
	_, recreated := k.sectors[sectorKey]
	k.mtx.RUnlock()
	require.False(t, recreated)

	// the same node under a FRESH sectorNo is accepted
	freshKey := kvsTypes.SectorKey{
		SectorID: sectorKey.SectorID,
		SectorNo: kvsTypes.SectorNo(3),
	}
	require.NoError(t, k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_APPEND,
		sectorKey: freshKey,
		head:      headNodeID,
		members:   members,
	}))
	k.mtx.RLock()
	_, created := k.sectors[freshKey]
	k.mtx.RUnlock()
	require.True(t, created)
}

// TestKVS_sectorManageMember_removeDestroysReplica verifies the receiver side
// of the out-of-band removal notification (シミュレーション 2026-07-06): a
// removed member no longer receives anything from its group, so COMMAND_REMOVE
// must destroy the local replica immediately (instead of waiting 30-60s for
// the leaderless force terminate) and tombstone the key so a stale re-delivered
// setting message cannot resurrect it.
func TestKVS_sectorManageMember_removeDestroysReplica(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	headNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &kvsHandlerHelper{isStable: true}
	k := newTestKVS(t, ctx, localNodeID, handler)

	sectorKey := kvsTypes.SectorKey{
		SectorID: kvsTypes.SectorID(uuid.New()),
		SectorNo: kvsTypes.SectorNo(2),
	}
	members := map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: headNodeID,
		kvsTypes.SectorNo(2):      localNodeID,
	}
	require.NoError(t, k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_APPEND,
		sectorKey: sectorKey,
		head:      headNodeID,
		members:   members,
	}))

	// the host notifies this node that the member was removed; the replica is
	// destroyed via TerminateLocally → SectorTerminated (async)
	require.NoError(t, k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_REMOVE,
		sectorKey: sectorKey,
		head:      headNodeID,
	}))

	require.Eventually(t, func() bool {
		k.mtx.RLock()
		defer k.mtx.RUnlock()
		_, exists := k.sectors[sectorKey]
		return !exists
	}, 5*time.Second, 50*time.Millisecond)

	// a stale re-delivered setting message must not resurrect the replica
	err := k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_APPEND,
		sectorKey: sectorKey,
		head:      headNodeID,
		members:   members,
	})
	require.Error(t, err)

	// a REMOVE for a key this node never held only leaves a tombstone
	unknownKey := kvsTypes.SectorKey{
		SectorID: sectorKey.SectorID,
		SectorNo: kvsTypes.SectorNo(9),
	}
	require.NoError(t, k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_REMOVE,
		sectorKey: unknownKey,
		head:      headNodeID,
	}))
	err = k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_APPEND,
		sectorKey: unknownKey,
		head:      headNodeID,
		members:   members,
	})
	require.Error(t, err)
}

// TestKVS_sectorManageMember_removeRejectsHostingSector: the host slot is never
// removed from its own group (getNodesToBeChanged skips HostNodeSectorNo), so a
// REMOVE targeting the local hosting sector is bogus — accepting it would let a
// single unauthenticated packet destroy an active sector.
func TestKVS_sectorManageMember_removeRejectsHostingSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	srcNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &kvsHandlerHelper{isStable: true}
	k := newTestKVS(t, ctx, localNodeID, handler)
	hostingSectorKey := setupHostingSector(t, k)

	err := k.sectorManageMember(&sectorManageMemberParam{
		command:   proto.SectorManageMember_COMMAND_REMOVE,
		sectorKey: *hostingSectorKey,
		head:      srcNodeID,
	})
	require.Error(t, err)

	k.mtx.RLock()
	_, exists := k.sectors[*hostingSectorKey]
	k.mtx.RUnlock()
	require.True(t, exists)
}

// TestKVS_sectorPrepareSplit_noHostingSector is a regression test for a nil
// dereference observed in the simulator (run7, 2026-07-04): the hosting sector
// key is nil between a (force) termination of the hosting sector and its
// re-creation by ManageMember, and an inbound prepare-split request in that
// window crashed the process. It must be rejected instead.
func TestKVS_sectorPrepareSplit_noHostingSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	srcNodeID := types.NewNormalNodeID(0x1000000000000000, 0)

	handler := &kvsHandlerHelper{isStable: true}
	k := newTestKVS(t, ctx, localNodeID, handler)

	// no hosting sector has been created: GetHostingSectorKey() is nil
	require.Nil(t, k.hostingManager.GetHostingSectorKey())
	require.False(t, k.sectorPrepareSplit(srcNodeID, kvsTypes.SectorID(uuid.New())))
}

// TestKVS_activateHostingSector_singleNode is a regression test: a lone node
// could never activate its own sector because the emptiness check counted the
// hosting sector itself.
func TestKVS_activateHostingSector_singleNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)

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
