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
package sector

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3"
)

type emptyRaftLogger struct{}

var _ raft.Logger = (*emptyRaftLogger)(nil)

func newEmptyRaftLogger() *emptyRaftLogger { return &emptyRaftLogger{} }

func (e *emptyRaftLogger) Debug(v ...interface{})                   {}
func (e *emptyRaftLogger) Debugf(format string, v ...interface{})   {}
func (e *emptyRaftLogger) Error(v ...interface{})                   {}
func (e *emptyRaftLogger) Errorf(format string, v ...interface{})   {}
func (e *emptyRaftLogger) Info(v ...interface{})                    {}
func (e *emptyRaftLogger) Infof(format string, v ...interface{})    {}
func (e *emptyRaftLogger) Warning(v ...interface{})                 {}
func (e *emptyRaftLogger) Warningf(format string, v ...interface{}) {}
func (e *emptyRaftLogger) Fatal(v ...interface{})                   {}
func (e *emptyRaftLogger) Fatalf(format string, v ...interface{})   {}
func (e *emptyRaftLogger) Panic(v ...interface{})                   {}
func (e *emptyRaftLogger) Panicf(format string, v ...interface{})   {}

type transfererHandlerHelper struct{}

var _ transferer.Handler = &transfererHandlerHelper{}

func (h *transfererHandlerHelper) TransfererSendPacket(p *networkTypes.Packet) {
	// packets to dead peers are dropped
}

func (h *transfererHandlerHelper) TransfererRelayPacket(nid *types.NodeID, p *networkTypes.Packet) {
	panic("not used in this test")
}

type sectorHandlerHelper struct {
	mtx        sync.Mutex
	terminated []kvsTypes.SectorKey
}

var _ SectorHandler = &sectorHandlerHelper{}

func (h *sectorHandlerHelper) SectorError(sectorKey *kvsTypes.SectorKey, err error) {}

func (h *sectorHandlerHelper) SectorAppendNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
}

func (h *sectorHandlerHelper) SectorRemoveNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo) {
}

func (h *sectorHandlerHelper) SectorTerminated(sectorKey *kvsTypes.SectorKey) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.terminated = append(h.terminated, *sectorKey)
}

func (h *sectorHandlerHelper) terminatedCount() int {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return len(h.terminated)
}

type storeHelper struct{}

var _ kvsTypes.Store = &storeHelper{}

func (s *storeHelper) AllocateSector(sectorKey *kvsTypes.SectorKey) error { return nil }
func (s *storeHelper) ReleaseSector(sectorKey *kvsTypes.SectorKey) error  { return nil }
func (s *storeHelper) Get(sectorKey *kvsTypes.SectorKey, key string) ([]byte, error) {
	return nil, kvsTypes.ErrorStoreKeyNotFound
}
func (s *storeHelper) Set(sectorKey *kvsTypes.SectorKey, key string, value []byte) error { return nil }
func (s *storeHelper) Patch(sectorKey *kvsTypes.SectorKey, key string, value []byte) error {
	return nil
}
func (s *storeHelper) Delete(sectorKey *kvsTypes.SectorKey, key string) error { return nil }

// newTestSector builds a hosting sector whose raft members are the local node
// and the given peers. Peers other than the local node never run, so a sector
// with any dead peer forming a majority can never elect a leader nor commit.
func newTestSector(t *testing.T, ctx context.Context, localNodeID *types.NodeID, handler *sectorHandlerHelper, members map[kvsTypes.SectorNo]*types.NodeID) *Sector {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tr := transferer.NewTransferer(&transferer.Config{
		Logger:  logger,
		Handler: &transfererHandlerHelper{},
	})
	tr.Start(ctx, localNodeID)

	return NewSector(&SectorConfig{
		Logger:     logger,
		RaftLogger: newEmptyRaftLogger(),
		Handler:    handler,
		Outbound:   consensus.NewOutbound(tr),
		Store:      &storeHelper{},
		SectorKey: &kvsTypes.SectorKey{
			SectorID: kvsTypes.SectorID(uuid.New()),
			SectorNo: kvsTypes.HostNodeSectorNo,
		},
		IsHosting: true,
		Join:      false,
		Members:   members,
		Head:      localNodeID,
	})
}

// TestSector_forceTerminate_onQuorumLoss covers the escape path for the stall
// found in the simulator (2026-07-04): a raft group that lost its quorum can
// never commit anything — including Terminate — so the replica must destroy
// itself locally once the group stays leaderless for forceTerminateDuration.
func TestSector_forceTerminate_onQuorumLoss(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
		kvsTypes.SectorNo(2):      deadNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.forceTerminateDuration = 1 * time.Second
	s.Start(ctx)

	require.Eventually(t, func() bool {
		return handler.terminatedCount() > 0
	}, 10*time.Second, 100*time.Millisecond)

	s.mtx.RLock()
	defer s.mtx.RUnlock()
	require.True(t, s.terminated)
	require.True(t, s.stopped)
}

// TestSector_forceTerminate_notFiredWithLeader checks the negative side of the
// quorum-loss detection: a healthy group (single voter, so the local node is
// always the leader once elected) must never be force-terminated.
func TestSector_forceTerminate_notFiredWithLeader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	// Longer than the initial election period so the transient leaderless
	// window right after start cannot trigger a false destroy.
	s.forceTerminateDuration = 3 * time.Second
	s.Start(ctx)

	// wait for the single-voter group to elect itself
	require.Eventually(t, func() bool {
		return s.consensus.Status().Lead != 0
	}, 10*time.Second, 100*time.Millisecond)

	time.Sleep(4 * time.Second)
	require.Equal(t, 0, handler.terminatedCount())
	require.Nil(t, s.GetTailAddress()) // untouched, still inactive
}

// TestSector_import_timeoutOnQuorumLoss covers the splitSector hang found in
// the simulator (2026-07-04): Import blocks on cond.Wait until the raft group
// applies the proposal, so a quorum-lost group used to block the caller (and
// KVS.mtxOperateSectors) forever. It must give up with ErrProposalTimeout and
// clear the pending proposal so operateSectors is not skipped forever.
func TestSector_import_timeoutOnQuorumLoss(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
		kvsTypes.SectorNo(2):      deadNodeID,
	})
	s.proposalWaitTimeout = 1 * time.Second
	s.forceTerminateDuration = time.Hour // keep the destroy path out of this test
	s.Start(ctx)

	done := make(chan error, 1)
	go func() {
		done <- s.Import(map[string][]byte{"key": []byte("value")})
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrProposalTimeout)
	case <-time.After(10 * time.Second):
		t.Fatal("Import did not return: cond.Wait without timeout")
	}

	require.False(t, s.HasManagementProposal())
}

// TestSector_import_unblockedByForceTerminate combines both new mechanisms:
// when a blocked operation outlives the quorum-loss detection, the forced
// local destroy must wake it up with ErrSectorStopped (not fake success).
func TestSector_import_unblockedByForceTerminate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	deadNodeID := types.NewNormalNodeID(0x8000000000000000, 0)

	handler := &sectorHandlerHelper{}
	s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
		kvsTypes.HostNodeSectorNo: localNodeID,
		kvsTypes.SectorNo(2):      deadNodeID,
	})
	s.proposalRetryDuration = 200 * time.Millisecond
	s.proposalWaitTimeout = time.Hour // let the destroy path win
	s.forceTerminateDuration = 1 * time.Second
	s.Start(ctx)

	done := make(chan error, 1)
	go func() {
		done <- s.Import(map[string][]byte{"key": []byte("value")})
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrSectorStopped)
	case <-time.After(10 * time.Second):
		t.Fatal("Import was not unblocked by the forced local destroy")
	}
	require.Eventually(t, func() bool {
		return handler.terminatedCount() > 0
	}, 5*time.Second, 100*time.Millisecond)
}
