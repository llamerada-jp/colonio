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
package operator

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/require"
)

// pushCollector records the watch events the operator sent out.
type pushCollector struct {
	mtx    sync.Mutex
	pushes []struct {
		dst   types.NodeID
		event *proto.KvsWatchEvent
	}
}

func (p *pushCollector) push(dst *types.NodeID, event *proto.KvsWatchEvent) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.pushes = append(p.pushes, struct {
		dst   types.NodeID
		event *proto.KvsWatchEvent
	}{dst: *dst, event: event})
}

func (p *pushCollector) take() []*proto.KvsWatchEvent {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	events := make([]*proto.KvsWatchEvent, 0, len(p.pushes))
	for _, push := range p.pushes {
		events = append(events, push.event)
	}
	p.pushes = nil
	return events
}

func newTestWatchOperator(handler Handler, collector *pushCollector) *Operator {
	head := types.NewNormalNodeID(0x4000000000000000, 0)
	return NewOperator(&Config{
		SectorKey: &kvsTypes.SectorKey{
			SectorID: kvsTypes.SectorID(uuid.New()),
			SectorNo: kvsTypes.HostNodeSectorNo,
		},
		Handler:        handler,
		Store:          newStoreHelper(),
		PushWatchEvent: collector.push,
		Head:           head,
	})
}

func TestOperator_watchSubscribeState(t *testing.T) {
	var o *Operator
	collector := &pushCollector{}
	o = newTestWatchOperator(echoHandler(&o), collector)
	require.NoError(t, o.SetRange(o.head)) // whole ring

	watcher := types.NewNormalNodeID(1, 2)

	// absent record: exists=false, no value
	state, err := o.WatchSubscribe("key", watcher, 1, 0)
	require.NoError(t, err)
	require.False(t, state.Exists)

	revision, err := o.Set("key", []byte("value"), nil)
	require.NoError(t, err)

	// current state with the value (since=0 → the watcher wants it)
	state, err = o.WatchSubscribe("key", watcher, 1, 0)
	require.NoError(t, err)
	require.True(t, state.Exists)
	require.Equal(t, revision, state.Revision)
	require.False(t, state.ValueOmitted)
	require.Equal(t, []byte("value"), state.Value)

	// unchanged since the reported revision → the value is omitted
	state, err = o.WatchSubscribe("key", watcher, 1, revision)
	require.NoError(t, err)
	require.True(t, state.Exists)
	require.Equal(t, revision, state.Revision)
	require.True(t, state.ValueOmitted)
	require.Nil(t, state.Value)
}

func TestOperator_watchEvents(t *testing.T) {
	var o *Operator
	collector := &pushCollector{}
	o = newTestWatchOperator(echoHandler(&o), collector)
	require.NoError(t, o.SetRange(o.head))

	watcher := types.NewNormalNodeID(1, 2)
	owner := types.NewNormalNodeID(3, 4)
	_, err := o.WatchSubscribe("key", watcher, 7, 0)
	require.NoError(t, err)

	// SET → value event
	revision, err := o.Set("key", []byte("v1"), nil)
	require.NoError(t, err)
	events := collector.take()
	require.Len(t, events, 1)
	require.Equal(t, uint64(7), events[0].WatchId)
	require.Equal(t, "key", events[0].Key)
	require.Equal(t, []byte("v1"), events[0].Value)
	require.Equal(t, revision, events[0].Revision)
	require.False(t, events[0].Deleted)
	require.False(t, events[0].Locked)

	// lock acquire on the existing record: grant is NOT emitted
	grant, err := o.LockAcquire("key", owner, 30*time.Second)
	require.NoError(t, err)
	require.Empty(t, collector.take())

	// renewal: not emitted either
	_, err = o.LockAcquire("key", owner, 30*time.Second)
	require.NoError(t, err)
	require.Empty(t, collector.take())

	// release → lock-cleared event with the unchanged revision
	require.NoError(t, o.LockRelease("key", owner, grant.Generation))
	events = collector.take()
	require.Len(t, events, 1)
	require.Equal(t, revision, events[0].Revision)
	require.False(t, events[0].Deleted)
	require.False(t, events[0].Locked)
	require.Equal(t, []byte("v1"), events[0].Value)

	// DELETE → deleted event carrying the removed record's revision
	require.NoError(t, o.Delete("key", nil))
	events = collector.take()
	require.Len(t, events, 1)
	require.True(t, events[0].Deleted)
	require.Equal(t, revision, events[0].Revision)
	require.Nil(t, events[0].Value)

	// lock acquire on an absent key → creation event (locked empty record)
	grant, err = o.LockAcquire("key", owner, 30*time.Second)
	require.NoError(t, err)
	events = collector.take()
	require.Len(t, events, 1)
	require.False(t, events[0].Deleted)
	require.True(t, events[0].Locked)
	require.Equal(t, grant.Generation, events[0].Revision)

	// no subscription, no event
	o.WatchCancel("key", watcher, 7)
	require.NoError(t, o.LockRelease("key", owner, grant.Generation))
	_, err = o.Set("key", []byte("v2"), nil)
	require.NoError(t, err)
	require.Empty(t, collector.take())
}

func TestOperator_watchOutOfRange(t *testing.T) {
	var o *Operator
	collector := &pushCollector{}
	o = newTestWatchOperator(echoHandler(&o), collector)
	tail := types.NewNormalNodeID(0x8000000000000000, 0)
	require.NoError(t, o.SetRange(*tail))

	watcher := types.NewNormalNodeID(1, 2)
	outKey := findKey(t, func(hash *types.NodeID) bool { return !hash.IsBetween(&o.head, tail) })

	_, err := o.WatchSubscribe(outKey, watcher, 1, 0)
	require.ErrorIs(t, err, kvsTypes.ErrorSectorNotReady)
}

func TestOperator_watchPurgeOnRangeChange(t *testing.T) {
	var o *Operator
	collector := &pushCollector{}
	o = newTestWatchOperator(echoHandler(&o), collector)
	wideTail := types.NewNormalNodeID(0xC000000000000000, 0)
	require.NoError(t, o.SetRange(*wideTail))

	watcher := types.NewNormalNodeID(1, 2)
	tail := types.NewNormalNodeID(0x8000000000000000, 0)
	inKey := findKey(t, func(hash *types.NodeID) bool { return hash.IsBetween(&o.head, tail) })
	outKey := findKey(t, func(hash *types.NodeID) bool { return hash.IsBetween(tail, wideTail) })

	_, err := o.WatchSubscribe(inKey, watcher, 1, 0)
	require.NoError(t, err)
	_, err = o.WatchSubscribe(outKey, watcher, 2, 0)
	require.NoError(t, err)

	// shrink: the moved key's subscription is dropped silently
	require.NoError(t, o.SetRange(*tail))
	o.mtx.RLock()
	require.Len(t, o.watches, 1)
	_, ok := o.watches[inKey]
	o.mtx.RUnlock()
	require.True(t, ok)
	require.Empty(t, collector.take()) // migration is not deletion

	// terminate / snapshot restore path drops everything
	o.ClearRange()
	o.mtx.RLock()
	require.Empty(t, o.watches)
	o.mtx.RUnlock()
}

func TestOperator_watchLeaseExpiry(t *testing.T) {
	var o *Operator
	collector := &pushCollector{}
	o = newTestWatchOperator(echoHandler(&o), collector)
	require.NoError(t, o.SetRange(o.head))

	watcher := types.NewNormalNodeID(1, 2)
	_, err := o.WatchSubscribe("key", watcher, 1, 0)
	require.NoError(t, err)

	// a fresh lease survives the purge
	o.PurgeExpiredWatches()
	o.mtx.RLock()
	require.Len(t, o.watches, 1)
	o.mtx.RUnlock()

	// run the lease out (keepalives stopped)
	o.mtx.Lock()
	for key, subscribers := range o.watches {
		for wk := range subscribers {
			o.watches[key][wk] = time.Now().Add(-time.Second)
		}
	}
	o.mtx.Unlock()

	o.PurgeExpiredWatches()
	o.mtx.RLock()
	require.Empty(t, o.watches)
	o.mtx.RUnlock()

	// a re-subscription (the client keepalive) restores the lease
	_, err = o.WatchSubscribe("key", watcher, 1, 0)
	require.NoError(t, err)
	_, err = o.Set("key", []byte("v"), nil)
	require.NoError(t, err)
	require.Len(t, collector.take(), 1)
}
