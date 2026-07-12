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
	"testing"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/require"
)

// TestSector_dataplane_endToEnd drives the full write path through a real
// single-member raft group: Operations.Set proposes the operation, the
// consensus loop commits and applies it, and only then the call returns.
// Reads are served from the locally applied store (read-your-writes).
func TestSector_dataplane_endToEnd(t *testing.T) {
	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)

	store := newRecordStoreHelper()
	s := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, store)
	s.Start(t.Context())
	defer s.Stop()

	// before activation every operation is rejected as retryable
	operator := s.GetOperator()
	require.ErrorIs(t, operator.Set("key1", []byte("value1")), kvsTypes.ErrorSectorNotReady)

	// activate with tail == head: the sector covers the whole ring
	s.Activate(*localNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	// write → committed through raft → applied → acknowledged
	require.NoError(t, operator.Set("key1", []byte("value1")))

	// read-your-writes on the host
	value, err := operator.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)

	// the applied write reached the shared store, so it is part of what
	// splits/merges/snapshots export
	stored, err := store.Get(&s.sectorKey, "key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), stored)

	// overwrite and delete complete the lifecycle
	require.NoError(t, operator.Set("key1", []byte("value2")))
	value, err = operator.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value2"), value)

	require.NoError(t, operator.Delete("key1"))
	_, err = operator.Get("key1")
	require.ErrorIs(t, err, kvsTypes.ErrorStoreKeyNotFound)
	require.ErrorIs(t, operator.Delete("key1"), kvsTypes.ErrorStoreKeyNotFound)
}

// TestSector_dataplane_mergeFencedByPrepareMerge: once a prepare_merge is
// committed (merge lock held), writes are rejected as retryable until the
// lock is released — a write applied while the absorber exports the records
// would be acknowledged but lost.
func TestSector_dataplane_mergeFencedByPrepareMerge(t *testing.T) {
	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)
	holder := types.NewNormalNodeID(0xc000000000000000, 0)

	s := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, newRecordStoreHelper())
	s.Start(t.Context())
	defer s.Stop()

	s.Activate(*localNodeID)
	require.Eventually(t, func() bool {
		return s.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)

	operator := s.GetOperator()
	require.NoError(t, operator.Set("key1", []byte("value1")))

	// the merge lock fences writes...
	require.NoError(t, s.PrepareMerge(holder))
	require.ErrorIs(t, operator.Set("key2", []byte("value2")), kvsTypes.ErrorSectorNotReady)

	// ...but reads stay available
	value, err := operator.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)

	// releasing the lock reopens writes (the release is proposed internally
	// by checkMergeRelease after mergeReleaseDuration; apply it directly here
	// instead of waiting 30s)
	require.NoError(t, s.ConsensusApplyProposal(&proto.ConsensusProposal{
		Content: &proto.ConsensusProposal_ReleaseMerge{
			ReleaseMerge: &proto.ReleaseMerge{Handler: holder.Proto()},
		},
	}))
	require.NoError(t, operator.Set("key2", []byte("value2")))
}

// TestSector_dataplane_snapshotIncludesOperationWrites: records written via
// the data plane are part of the sector snapshot, and a replica restored from
// it serves them (the snapshot path and the data plane share the same store
// state).
func TestSector_dataplane_snapshotIncludesOperationWrites(t *testing.T) {
	localNodeID := types.NewNormalNodeID(0x4000000000000000, 0)

	src := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, newRecordStoreHelper())
	src.Start(t.Context())
	defer src.Stop()

	src.Activate(*localNodeID)
	require.Eventually(t, func() bool {
		return src.GetTailAddress() != nil
	}, 10*time.Second, 100*time.Millisecond)
	require.NoError(t, src.GetOperator().Set("key1", []byte("value1")))

	data, err := src.ConsensusGetSnapshot()
	require.NoError(t, err)

	dst := newSnapshotTestSector(t, localNodeID, &sectorHandlerHelper{}, newRecordStoreHelper())
	require.NoError(t, dst.ConsensusApplySnapshot(data))

	value, err := dst.GetOperator().Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)
}
