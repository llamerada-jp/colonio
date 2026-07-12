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
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/require"
)

// recordStoreHelper is a functional in-memory store (unlike storeHelper,
// whose record operations are no-ops) so snapshot tests can verify that
// records survive the export/apply round trip and that stale records are
// replaced, not merged.
type recordStoreHelper struct {
	mtx     sync.Mutex
	sectors map[kvsTypes.SectorKey]map[string][]byte
}

var _ kvsTypes.Store = &recordStoreHelper{}

func newRecordStoreHelper() *recordStoreHelper {
	return &recordStoreHelper{
		sectors: make(map[kvsTypes.SectorKey]map[string][]byte),
	}
}

func (s *recordStoreHelper) AllocateSector(sectorKey *kvsTypes.SectorKey) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if _, exists := s.sectors[*sectorKey]; exists {
		return fmt.Errorf("sector already exists: %s", sectorKey.SectorID.String())
	}
	s.sectors[*sectorKey] = make(map[string][]byte)
	return nil
}

func (s *recordStoreHelper) ReleaseSector(sectorKey *kvsTypes.SectorKey) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if _, exists := s.sectors[*sectorKey]; !exists {
		return fmt.Errorf("sector does not exist: %s", sectorKey.SectorID.String())
	}
	delete(s.sectors, *sectorKey)
	return nil
}

func (s *recordStoreHelper) Get(sectorKey *kvsTypes.SectorKey, key string) ([]byte, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	records, exists := s.sectors[*sectorKey]
	if !exists {
		return nil, kvsTypes.ErrorStoreKeyNotFound
	}
	value, exists := records[key]
	if !exists {
		return nil, kvsTypes.ErrorStoreKeyNotFound
	}
	return value, nil
}

func (s *recordStoreHelper) Set(sectorKey *kvsTypes.SectorKey, key string, value []byte) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	records, exists := s.sectors[*sectorKey]
	if !exists {
		return fmt.Errorf("sector does not exist: %s", sectorKey.SectorID.String())
	}
	records[key] = value
	return nil
}

func (s *recordStoreHelper) Delete(sectorKey *kvsTypes.SectorKey, key string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	records, exists := s.sectors[*sectorKey]
	if !exists {
		return fmt.Errorf("sector does not exist: %s", sectorKey.SectorID.String())
	}
	delete(records, key)
	return nil
}

// newSnapshotTestSector builds a sector wired to a functional record store
// (newTestSector wires the no-op storeHelper, which cannot verify record
// round trips). The sector is NOT started: snapshot tests drive
// ConsensusApplyProposal / ConsensusGetSnapshot / ConsensusApplySnapshot
// directly, exactly as the consensus loop goroutine would, so no raft group
// is needed.
func newSnapshotTestSector(t *testing.T, localNodeID *types.NodeID, handler *sectorHandlerHelper, store *recordStoreHelper) *Sector {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
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
		Store:      store,
		SectorKey: &kvsTypes.SectorKey{
			SectorID: kvsTypes.SectorID(uuid.New()),
			SectorNo: kvsTypes.HostNodeSectorNo,
		},
		IsHosting: true,
		Join:      false,
		Members:   map[kvsTypes.SectorNo]*types.NodeID{kvsTypes.HostNodeSectorNo: localNodeID},
		Head:      localNodeID,
	})
}

func applyProposal(t *testing.T, s *Sector, proposal *proto.ConsensusProposal) {
	t.Helper()
	require.NoError(t, s.ConsensusApplyProposal(proposal))
}

func activateProposal(tail *types.NodeID) *proto.ConsensusProposal {
	return &proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Activate{
			Activate: &proto.Activate{Tail: tail.Proto()},
		},
	}
}

func importProposal(records map[string][]byte) *proto.ConsensusProposal {
	imp := &proto.Import{}
	for key, value := range records {
		imp.Records = append(imp.Records, &proto.Import_Record{Key: key, Value: value})
	}
	return &proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Import{Import: imp},
	}
}

func prepareMergeProposal(handler *types.NodeID) *proto.ConsensusProposal {
	return &proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_PrepareMerge{
			PrepareMerge: &proto.PrepareMerge{Handler: handler.Proto()},
		},
	}
}

// TestSector_snapshot_roundTrip covers the sector-layer snapshot
// (spec/kvs/snapshot.md Stage 2): the replicated state (records, tail,
// mergeBy) survives ConsensusGetSnapshot → ConsensusApplySnapshot, and the
// apply REPLACES the target's state — a stale record held only by the
// receiver must disappear.
func TestSector_snapshot_roundTrip(t *testing.T) {
	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	tail := types.NewNormalNodeID(0x8000000000000000, 0)
	mergeBy := types.NewNormalNodeID(0xc000000000000000, 0)

	// source sector: activated, holding records and a merge lock
	srcStore := newRecordStoreHelper()
	src := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, srcStore)
	applyProposal(t, src, activateProposal(tail))
	applyProposal(t, src, importProposal(map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
	}))
	applyProposal(t, src, prepareMergeProposal(mergeBy))

	data, err := src.ConsensusGetSnapshot()
	require.NoError(t, err)

	// destination sector: behind the group, holding a stale record that the
	// snapshot does not contain
	dstStore := newRecordStoreHelper()
	dst := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, dstStore)
	applyProposal(t, dst, importProposal(map[string][]byte{
		"stale": []byte("gone"),
	}))

	require.NoError(t, dst.ConsensusApplySnapshot(data))

	dst.mtx.RLock()
	require.NotNil(t, dst.tail)
	require.True(t, dst.tail.Equal(tail))
	require.NotNil(t, dst.mergeBy)
	require.True(t, dst.mergeBy.Equal(mergeBy))
	require.False(t, dst.terminated)
	dst.mtx.RUnlock()

	records, err := dst.operator.ExportAllRecords()
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
	}, records)

	// replacement, not merge: the stale record is gone from the store too
	_, err = dstStore.Get(&dst.sectorKey, "stale")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)

	// idempotent: applying the same snapshot again must not fail nor change state
	require.NoError(t, dst.ConsensusApplySnapshot(data))
	records, err = dst.operator.ExportAllRecords()
	require.NoError(t, err)
	require.Len(t, records, 2)
}

// TestSector_snapshot_inactiveWithRecords covers the split-in-progress shape:
// records were imported into a not-yet-activated sector (tail == nil), which
// is a legal snapshot state and must round-trip without inventing a tail.
func TestSector_snapshot_inactiveWithRecords(t *testing.T) {
	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)

	srcStore := newRecordStoreHelper()
	src := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, srcStore)
	applyProposal(t, src, importProposal(map[string][]byte{
		"key1": []byte("value1"),
	}))

	data, err := src.ConsensusGetSnapshot()
	require.NoError(t, err)

	dstStore := newRecordStoreHelper()
	dst := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, dstStore)
	require.NoError(t, dst.ConsensusApplySnapshot(data))

	dst.mtx.RLock()
	require.Nil(t, dst.tail)
	require.Nil(t, dst.mergeBy)
	dst.mtx.RUnlock()

	records, err := dst.operator.ExportAllRecords()
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{"key1": []byte("value1")}, records)
}

// TestSector_snapshot_terminated: a snapshot of a terminated sector must
// terminate the receiving replica (same effect as replaying the Terminate
// entry), and must not resurrect any state.
func TestSector_snapshot_terminated(t *testing.T) {
	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	tail := types.NewNormalNodeID(0x8000000000000000, 0)

	srcStore := newRecordStoreHelper()
	src := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, srcStore)
	applyProposal(t, src, activateProposal(tail))
	applyProposal(t, src, &proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_Terminate{Terminate: &proto.Terminate{}},
	})

	data, err := src.ConsensusGetSnapshot()
	require.NoError(t, err)

	dstHandler := &sectorHandlerHelper{}
	dstStore := newRecordStoreHelper()
	dst := newSnapshotTestSector(t, localNodeID, dstHandler, dstStore)
	applyProposal(t, dst, activateProposal(tail))

	require.NoError(t, dst.ConsensusApplySnapshot(data))

	dst.mtx.RLock()
	require.True(t, dst.terminated)
	require.True(t, dst.stopped)
	dst.mtx.RUnlock()

	// a stopped/terminated replica must not be resurrected by a later
	// (non-terminated) snapshot
	live := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, newRecordStoreHelper())
	applyProposal(t, live, activateProposal(tail))
	liveData, err := live.ConsensusGetSnapshot()
	require.NoError(t, err)
	require.NoError(t, dst.ConsensusApplySnapshot(liveData))
	dst.mtx.RLock()
	require.True(t, dst.terminated)
	dst.mtx.RUnlock()
}
