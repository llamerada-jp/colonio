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
package broker

import (
	"context"
	"sync"
	"testing"
	"time"

	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	"github.com/stretchr/testify/assert"
)

const (
	// loopInterval is the broker interval used for most tests.
	loopInterval = 300 * time.Millisecond
	// noFireDuration is a wait duration to confirm that no send occurs.
	noFireDuration = 500 * time.Millisecond
	// immediateDuration is a short wait for signal-triggered actions to complete.
	immediateDuration = 100 * time.Millisecond
)

type sendRecord struct {
	dst         types.NodeID
	tailAddress types.NodeID
}

type mockInfrastructure struct {
	mtx     sync.Mutex
	records []sendRecord
}

var _ Infrastructure = &mockInfrastructure{}

func (m *mockInfrastructure) sendSectorInformation(dst *types.NodeID, tailAddress *types.NodeID) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	var ta types.NodeID
	if tailAddress != nil {
		ta = *tailAddress
	}
	m.records = append(m.records, sendRecord{dst: *dst, tailAddress: ta})
}

func (m *mockInfrastructure) getRecords() []sendRecord {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.records
}

func (m *mockInfrastructure) resetRecords() {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.records = nil
}

func newTestBroker(t *testing.T, interval time.Duration, infra *mockInfrastructure) *Broker {
	return NewBroker(&Config{
		Logger:         testUtil.Logger(t),
		Infrastructure: infra,
		Interval:       interval,
	})
}

func TestBroker_Start_PeriodicSend(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	b := newTestBroker(t, loopInterval, infra)

	b.Start(t.Context(), nodeIDs[0])

	backward := []*types.NodeID{nodeIDs[1]}
	tailAddr := nodeIDs[2]
	b.UpdateTailAddress(tailAddr)
	b.UpdateNextNodeIDs(backward, nil)

	// wait for at least 3 sends (signal + timer fires)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.GreaterOrEqual(c, len(infra.getRecords()), 3)
	}, 2*time.Second, 50*time.Millisecond)
}

func TestBroker_Start_StopsOnCtxCancel(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(2)
	b := newTestBroker(t, loopInterval, infra)

	ctx, cancel := context.WithCancel(t.Context())
	b.Start(ctx, nodeIDs[0])

	backward := []*types.NodeID{nodeIDs[1]}
	b.UpdateNextNodeIDs(backward, nil)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.GreaterOrEqual(c, len(infra.getRecords()), 1)
	}, time.Second, 10*time.Millisecond)

	cancel()
	infra.resetRecords()

	// after cancel, no more sends should happen
	time.Sleep(noFireDuration)
	assert.Empty(t, infra.getRecords())
}

func TestBroker_sendSectorInformation_SkippedOnSameValues(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	b := newTestBroker(t, loopInterval, infra)

	b.Start(t.Context(), nodeIDs[0])

	backward := []*types.NodeID{nodeIDs[1]}
	tailAddr := nodeIDs[2]
	b.UpdateTailAddress(tailAddr)
	b.UpdateNextNodeIDs(backward, nil)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.GreaterOrEqual(c, len(infra.getRecords()), 1)
	}, time.Second, 10*time.Millisecond)

	infra.resetRecords()

	// set the same values again
	b.UpdateNextNodeIDs(backward, nil)
	b.UpdateTailAddress(tailAddr)

	// no signal should fire, wait a bit and confirm no sends
	time.Sleep(immediateDuration)
	assert.Empty(t, infra.getRecords())
}

func TestBroker_sendSectorInformation_EmptyBackward(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(2)
	b := newTestBroker(t, loopInterval, infra)

	b.Start(t.Context(), nodeIDs[0])

	b.UpdateTailAddress(nodeIDs[1])

	// backward is empty, so no sends even after timer fires
	time.Sleep(noFireDuration)
	assert.Empty(t, infra.getRecords())
}

func TestBroker_UpdateNextNodeIDs_TriggersSend(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	b := newTestBroker(t, loopInterval, infra)

	b.Start(t.Context(), nodeIDs[0])

	tailAddr := nodeIDs[2]
	b.UpdateTailAddress(tailAddr)
	// wait for signal-triggered send
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		// no backward nodes yet, so no sends expected from tailAddress update
		assert.Empty(c, infra.getRecords())
	}, time.Second, 10*time.Millisecond)

	infra.resetRecords()
	backward := []*types.NodeID{nodeIDs[1]}
	b.UpdateNextNodeIDs(backward, nil)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		records := infra.getRecords()
		assert.Len(c, records, 1)
		if len(records) == 1 {
			assert.Equal(c, *nodeIDs[1], records[0].dst)
			assert.Equal(c, *tailAddr, records[0].tailAddress)
		}
	}, time.Second, 10*time.Millisecond)
}

func TestBroker_UpdateNextNodeIDs_FrontwardStored(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(4)
	b := newTestBroker(t, loopInterval, infra)

	b.Start(t.Context(), nodeIDs[0])

	frontward := []*types.NodeID{nodeIDs[2], nodeIDs[3]}
	b.UpdateNextNodeIDs(nil, frontward)

	b.mtx.Lock()
	assert.Equal(t, *nodeIDs[2], b.frontwardNodeID)
	b.mtx.Unlock()
}

func TestBroker_UpdateNextNodeIDs_EmptyFrontward(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	b := newTestBroker(t, loopInterval, infra)

	b.Start(t.Context(), nodeIDs[0])

	b.UpdateNextNodeIDs(nil, []*types.NodeID{nodeIDs[1]})
	b.mtx.Lock()
	saved := b.frontwardNodeID
	b.mtx.Unlock()

	// empty frontward should not overwrite
	b.UpdateNextNodeIDs(nil, nil)
	b.mtx.Lock()
	assert.Equal(t, saved, b.frontwardNodeID)
	b.mtx.Unlock()
}

func TestBroker_UpdateTailAddress_TriggersSend(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	b := newTestBroker(t, loopInterval, infra)

	b.Start(t.Context(), nodeIDs[0])

	backward := []*types.NodeID{nodeIDs[1]}
	b.UpdateNextNodeIDs(backward, nil)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.GreaterOrEqual(c, len(infra.getRecords()), 1)
	}, time.Second, 10*time.Millisecond)

	infra.resetRecords()
	tailAddr := nodeIDs[2]
	b.UpdateTailAddress(tailAddr)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		records := infra.getRecords()
		assert.Len(c, records, 1)
		if len(records) == 1 {
			assert.Equal(c, *nodeIDs[1], records[0].dst)
			assert.Equal(c, *tailAddr, records[0].tailAddress)
		}
	}, time.Second, 10*time.Millisecond)
}

func TestBroker_GetFrontwardState(t *testing.T) {
	tests := []struct {
		name         string
		useSameSrc   bool // whether processSectorInformation src matches frontward
		expectNonNil bool
	}{
		{
			name:         "ReturnsMatchingEntry",
			useSameSrc:   true,
			expectNonNil: true,
		},
		{
			name:         "NoMatchingEntry",
			useSameSrc:   false,
			expectNonNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			infra := &mockInfrastructure{}
			nodeIDs := testUtil.UniqueNodeIDs(4)
			b := newTestBroker(t, loopInterval, infra)

			b.Start(t.Context(), nodeIDs[0])

			frontwardID := nodeIDs[1]
			b.UpdateNextNodeIDs(nil, []*types.NodeID{frontwardID})

			srcID := frontwardID
			if !tt.useSameSrc {
				srcID = nodeIDs[2]
			}
			tailAddr := nodeIDs[3]
			b.processSectorInformation(&sectorInformationParam{
				srcNodeID:   srcID,
				tailAddress: tailAddr,
			})

			fwd, tail := b.GetFrontwardState()
			if tt.expectNonNil {
				assert.NotNil(t, fwd)
				assert.NotNil(t, tail)
				assert.Equal(t, *frontwardID, *fwd)
				assert.Equal(t, *tailAddr, *tail)
			} else {
				assert.Nil(t, fwd)
				assert.Nil(t, tail)
			}
		})
	}
}

func TestBroker_GetFrontwardState_ExpiredEntry(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	b := newTestBroker(t, loopInterval, infra)

	b.Start(t.Context(), nodeIDs[0])

	frontwardID := nodeIDs[1]
	b.UpdateNextNodeIDs(nil, []*types.NodeID{frontwardID})
	b.processSectorInformation(&sectorInformationParam{
		srcNodeID:   frontwardID,
		tailAddress: nodeIDs[2],
	})

	// wait for entry to expire (loopInterval * 2)
	time.Sleep(loopInterval*2 + immediateDuration)

	fwd, tail := b.GetFrontwardState()
	assert.Nil(t, fwd)
	assert.Nil(t, tail)
}

func TestBroker_processSectorInformation_Overwrite(t *testing.T) {
	infra := &mockInfrastructure{}
	nodeIDs := testUtil.UniqueNodeIDs(4)
	b := newTestBroker(t, loopInterval, infra)

	b.Start(t.Context(), nodeIDs[0])

	srcID := nodeIDs[1]
	b.UpdateNextNodeIDs(nil, []*types.NodeID{srcID})

	b.processSectorInformation(&sectorInformationParam{
		srcNodeID:   srcID,
		tailAddress: nodeIDs[2],
	})
	b.processSectorInformation(&sectorInformationParam{
		srcNodeID:   srcID,
		tailAddress: nodeIDs[3],
	})

	fwd, tail := b.GetFrontwardState()
	assert.NotNil(t, fwd)
	assert.Equal(t, *nodeIDs[3], *tail)
}
