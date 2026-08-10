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

// TestKVS_sectorActivate covers regressions in the sectorActivate ->
// activateHostingSector path.
func TestKVS_sectorActivate(t *testing.T) {
	tests := []struct {
		name string
		// registerDeadReplica adds an inactive sector replica (head between
		// local and frontward) before triggering activation.
		registerDeadReplica bool
	}{
		{
			// Handling an inbound sector-activate request used to
			// self-deadlock on KVS.mtx (the request handler held the write
			// lock while activateHostingSector tried to lock it again),
			// freezing the node and leaving every sector after the first
			// one inactive.
			name: "completes without obstruction",
		},
		{
			// A stall found in the simulator: a stopped node's sector
			// replica stays in k.sectors as an inactive sector. When such a
			// replica's head lies between the local node and its routing
			// frontward neighbor, the overlap guard used to treat it like
			// an active sector and rejected activation forever, freezing
			// the activation chain at that point. Only ACTIVE sectors can
			// overlap, so inactive replicas must not block activation
			// (TLA+ ActivateFrontward guard quantifies over Actives only).
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

// TestKVS_activateHostingSector_singleNode is a regression test: a lone node
// could never activate its own sector because the emptiness check counted the
// hosting sector itself.
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
