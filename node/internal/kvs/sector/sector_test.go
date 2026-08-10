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
	testUtil "github.com/llamerada-jp/colonio/test/util"
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

// TestSector_checkQuorumLoss covers checkQuorumLoss: a sector destroys itself
// locally once its raft group has had no leader for forceTerminateDuration,
// because a group that lost its quorum can never commit anything, not even a
// normal Terminate proposal. A healthy group with a leader must never be
// destroyed this way.
func TestSector_checkQuorumLoss(t *testing.T) {
	tests := []struct {
		name string
		// withDeadPeer adds a peer that never runs, so the group is a
		// 2-member majority that can never elect a leader.
		withDeadPeer bool
		// forceTerminateDuration must be longer than the time it normally
		// takes to elect a leader, or the "has a leader" case could destroy
		// the sector before the leader is elected.
		forceTerminateDuration time.Duration
		wantTerminated         bool
	}{
		{
			name:                   "destroys the sector when the group never elects a leader",
			withDeadPeer:           true,
			forceTerminateDuration: 1 * time.Second,
			wantTerminated:         true,
		},
		{
			name:                   "keeps the sector when the group has a leader",
			withDeadPeer:           false,
			forceTerminateDuration: 3 * time.Second,
			wantTerminated:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			nodeIDs := testUtil.UniqueNodeIDs(2)
			localNodeID := nodeIDs[0]
			members := map[kvsTypes.SectorNo]*types.NodeID{
				kvsTypes.HostNodeSectorNo: localNodeID,
			}
			if tt.withDeadPeer {
				members[kvsTypes.SectorNo(2)] = nodeIDs[1]
			}

			handler := &sectorHandlerHelper{}
			s := newTestSector(t, ctx, localNodeID, handler, members)
			s.proposalRetryDuration = 200 * time.Millisecond
			s.forceTerminateDuration = tt.forceTerminateDuration
			s.Start(ctx)

			if tt.wantTerminated {
				require.Eventually(t, func() bool {
					return handler.terminatedCount() > 0
				}, 10*time.Second, 100*time.Millisecond)

				s.mtx.RLock()
				defer s.mtx.RUnlock()
				require.True(t, s.terminated)
				require.True(t, s.stopped)
				return
			}

			// wait for the single-voter group to elect itself, then make
			// sure it stays untouched past forceTerminateDuration
			require.Eventually(t, func() bool {
				return s.consensus.Status().Lead != 0
			}, 10*time.Second, 100*time.Millisecond)
			time.Sleep(tt.forceTerminateDuration + time.Second)

			require.Equal(t, 0, handler.terminatedCount())
			require.Nil(t, s.GetTailAddress()) // untouched, still inactive
		})
	}
}

// TestSector_import_blockedOnQuorumLoss covers waitProposal: a blocking
// operation such as Import must not wait forever when its raft group cannot
// commit the proposal. It gives up with ErrProposalTimeout once
// proposalWaitTimeout elapses, or with ErrSectorStopped once checkQuorumLoss
// destroys the sector first. Either way, the pending proposal must be
// cleared so it is not retried forever.
func TestSector_import_blockedOnQuorumLoss(t *testing.T) {
	tests := []struct {
		name                   string
		proposalWaitTimeout    time.Duration
		forceTerminateDuration time.Duration
		wantErr                error
	}{
		{
			name:                   "returns ErrProposalTimeout once the wait times out",
			proposalWaitTimeout:    1 * time.Second,
			forceTerminateDuration: time.Hour, // keep the destroy path from firing first
			wantErr:                ErrProposalTimeout,
		},
		{
			name:                   "returns ErrSectorStopped once a forced local destroy wakes it up",
			proposalWaitTimeout:    time.Hour, // keep the timeout path from firing first
			forceTerminateDuration: 1 * time.Second,
			wantErr:                ErrSectorStopped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			nodeIDs := testUtil.UniqueNodeIDs(2)
			localNodeID, deadNodeID := nodeIDs[0], nodeIDs[1]

			handler := &sectorHandlerHelper{}
			s := newTestSector(t, ctx, localNodeID, handler, map[kvsTypes.SectorNo]*types.NodeID{
				kvsTypes.HostNodeSectorNo: localNodeID,
				kvsTypes.SectorNo(2):      deadNodeID,
			})
			s.proposalRetryDuration = 200 * time.Millisecond
			s.proposalWaitTimeout = tt.proposalWaitTimeout
			s.forceTerminateDuration = tt.forceTerminateDuration
			s.Start(ctx)

			done := make(chan error, 1)
			go func() {
				done <- s.Import(map[string][]byte{"key": []byte("value")})
			}()

			select {
			case err := <-done:
				require.ErrorIs(t, err, tt.wantErr)
			case <-time.After(10 * time.Second):
				t.Fatal("Import did not return")
			}

			require.False(t, s.HasManagementProposal())
			if tt.wantErr == ErrSectorStopped {
				require.Eventually(t, func() bool {
					return handler.terminatedCount() > 0
				}, 5*time.Second, 100*time.Millisecond)
			}
		})
	}
}
