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
	tailAddress *types.NodeID
}

type mockOutbound struct {
	mtx     sync.Mutex
	records []sendRecord
}

var _ OutboundPort = &mockOutbound{}

func (m *mockOutbound) sendSectorInformation(dst *types.NodeID, tailAddress *types.NodeID) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.records = append(m.records, sendRecord{dst: *dst, tailAddress: tailAddress})
}

func (m *mockOutbound) getRecords() []sendRecord {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.records
}

func (m *mockOutbound) resetRecords() {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.records = nil
}

func newTestBroker(t *testing.T, interval time.Duration, outbound *mockOutbound) *Broker {
	return NewBroker(&Config{
		Logger:   testUtil.Logger(t),
		Outbound: outbound,
		Interval: interval,
	})
}

func TestBroker_Start_PeriodicSend(t *testing.T) {
	outbound := &mockOutbound{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	b := newTestBroker(t, loopInterval, outbound)

	b.Start(t.Context(), nodeIDs[0])

	backward := []*types.NodeID{nodeIDs[1]}
	tailAddr := nodeIDs[2]
	b.UpdateTailAddress(tailAddr)
	b.UpdateNextNodeIDs(backward, nil)

	// wait for at least 3 sends (signal + timer fires)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.GreaterOrEqual(c, len(outbound.getRecords()), 3)
	}, 2*time.Second, 50*time.Millisecond)
}

func TestBroker_Start_StopsOnCtxCancel(t *testing.T) {
	outbound := &mockOutbound{}
	nodeIDs := testUtil.UniqueNodeIDs(2)
	b := newTestBroker(t, loopInterval, outbound)

	ctx, cancel := context.WithCancel(t.Context())
	b.Start(ctx, nodeIDs[0])

	backward := []*types.NodeID{nodeIDs[1]}
	b.UpdateNextNodeIDs(backward, nil)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.GreaterOrEqual(c, len(outbound.getRecords()), 1)
	}, time.Second, 10*time.Millisecond)

	cancel()
	outbound.resetRecords()

	// after cancel, no more sends should happen
	time.Sleep(noFireDuration)
	assert.Empty(t, outbound.getRecords())
}

func TestBroker_sendSectorInformation_SkippedOnSameValues(t *testing.T) {
	tests := []struct {
		name        string
		nilTailAddr bool
	}{
		{"NonNilTailAddress", false},
		{"NilTailAddress", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbound := &mockOutbound{}
			nodeIDs := testUtil.UniqueNodeIDs(3)
			b := newTestBroker(t, loopInterval, outbound)

			b.Start(t.Context(), nodeIDs[0])

			backward := []*types.NodeID{nodeIDs[1]}
			var tailAddr *types.NodeID
			if !tt.nilTailAddr {
				tailAddr = nodeIDs[2]
			}
			b.UpdateTailAddress(tailAddr)
			b.UpdateNextNodeIDs(backward, nil)
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				assert.GreaterOrEqual(c, len(outbound.getRecords()), 1)
			}, time.Second, 10*time.Millisecond)

			outbound.resetRecords()

			// set the same values again
			b.UpdateNextNodeIDs(backward, nil)
			b.UpdateTailAddress(tailAddr)

			// no signal should fire, wait a bit and confirm no sends
			time.Sleep(immediateDuration)
			assert.Empty(t, outbound.getRecords())
		})
	}
}

func TestBroker_sendSectorInformation_EmptyBackward(t *testing.T) {
	outbound := &mockOutbound{}
	nodeIDs := testUtil.UniqueNodeIDs(2)
	b := newTestBroker(t, loopInterval, outbound)

	b.Start(t.Context(), nodeIDs[0])

	b.UpdateTailAddress(nodeIDs[1])

	// backward is empty, so no sends even after timer fires
	time.Sleep(noFireDuration)
	assert.Empty(t, outbound.getRecords())
}

func TestBroker_UpdateNextNodeIDs_TriggersSend(t *testing.T) {
	tests := []struct {
		name        string
		nilTailAddr bool
	}{
		{"NonNilTailAddress", false},
		{"NilTailAddress", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbound := &mockOutbound{}
			nodeIDs := testUtil.UniqueNodeIDs(3)
			b := newTestBroker(t, loopInterval, outbound)

			b.Start(t.Context(), nodeIDs[0])

			var tailAddr *types.NodeID
			if !tt.nilTailAddr {
				tailAddr = nodeIDs[2]
			}
			b.UpdateTailAddress(tailAddr)
			// wait for signal-triggered send
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				// no backward nodes yet, so no sends expected from tailAddress update
				assert.Empty(c, outbound.getRecords())
			}, time.Second, 10*time.Millisecond)

			outbound.resetRecords()
			backward := []*types.NodeID{nodeIDs[1]}
			b.UpdateNextNodeIDs(backward, nil)

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				records := outbound.getRecords()
				assert.Len(c, records, 1)
				if len(records) == 1 {
					assert.Equal(c, *nodeIDs[1], records[0].dst)
					assert.Equal(c, tailAddr, records[0].tailAddress)
				}
			}, time.Second, 10*time.Millisecond)
		})
	}
}

func TestBroker_UpdateNextNodeIDs_FrontwardStored(t *testing.T) {
	outbound := &mockOutbound{}
	nodeIDs := testUtil.UniqueNodeIDs(4)
	b := newTestBroker(t, loopInterval, outbound)

	b.Start(t.Context(), nodeIDs[0])

	frontward := []*types.NodeID{nodeIDs[2], nodeIDs[3]}
	b.UpdateNextNodeIDs(nil, frontward)

	b.mtx.Lock()
	assert.Equal(t, *nodeIDs[2], b.frontwardNodeID)
	b.mtx.Unlock()
}

func TestBroker_UpdateNextNodeIDs_EmptyFrontward(t *testing.T) {
	outbound := &mockOutbound{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	b := newTestBroker(t, loopInterval, outbound)

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
	tests := []struct {
		name         string
		nilFinalTail bool
	}{
		{"NonNilTailAddress", false},
		{"NilTailAddress", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbound := &mockOutbound{}
			nodeIDs := testUtil.UniqueNodeIDs(4)
			b := newTestBroker(t, loopInterval, outbound)

			b.Start(t.Context(), nodeIDs[0])

			backward := []*types.NodeID{nodeIDs[1]}
			if tt.nilFinalTail {
				// Set non-nil initial to detect the change to nil.
				b.UpdateTailAddress(nodeIDs[2])
			}
			b.UpdateNextNodeIDs(backward, nil)
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				assert.GreaterOrEqual(c, len(outbound.getRecords()), 1)
			}, time.Second, 10*time.Millisecond)

			outbound.resetRecords()
			var finalTail *types.NodeID
			if !tt.nilFinalTail {
				finalTail = nodeIDs[3]
			}
			b.UpdateTailAddress(finalTail)

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				records := outbound.getRecords()
				assert.Len(c, records, 1)
				if len(records) == 1 {
					assert.Equal(c, *nodeIDs[1], records[0].dst)
					assert.Equal(c, finalTail, records[0].tailAddress)
				}
			}, time.Second, 10*time.Millisecond)
		})
	}
}

func TestBroker_GetFrontwardState(t *testing.T) {
	tests := []struct {
		name           string
		useSameSrc     bool
		nilTailAddress bool
		expectFwd      bool
		expectTail     bool
	}{
		{
			name:       "ReturnsMatchingEntry",
			useSameSrc: true,
			expectFwd:  true,
			expectTail: true,
		},
		{
			name:           "NilTailAddress",
			useSameSrc:     true,
			nilTailAddress: true,
			expectFwd:      true,
			expectTail:     false,
		},
		{
			name:       "NoMatchingEntry",
			useSameSrc: false,
			expectFwd:  false,
			expectTail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbound := &mockOutbound{}
			nodeIDs := testUtil.UniqueNodeIDs(4)
			b := newTestBroker(t, loopInterval, outbound)

			b.Start(t.Context(), nodeIDs[0])

			frontwardID := nodeIDs[1]
			b.UpdateNextNodeIDs(nil, []*types.NodeID{frontwardID})

			srcID := frontwardID
			if !tt.useSameSrc {
				srcID = nodeIDs[2]
			}
			var tailAddr *types.NodeID
			if !tt.nilTailAddress {
				tailAddr = nodeIDs[3]
			}
			b.processSectorInformation(&sectorInformationParam{
				srcNodeID:   srcID,
				tailAddress: tailAddr,
			})

			fwd, tail := b.GetFrontwardState()
			if tt.expectFwd {
				assert.NotNil(t, fwd)
				assert.Equal(t, *frontwardID, *fwd)
			} else {
				assert.Nil(t, fwd)
			}
			if tt.expectTail {
				assert.NotNil(t, tail)
				assert.Equal(t, *tailAddr, *tail)
			} else {
				assert.Nil(t, tail)
			}
		})
	}
}

func TestBroker_GetFrontwardState_ExpiredEntry(t *testing.T) {
	outbound := &mockOutbound{}
	nodeIDs := testUtil.UniqueNodeIDs(3)
	b := newTestBroker(t, loopInterval, outbound)

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
	tests := []struct {
		name             string
		firstNilTailAddr bool
		finalNilTailAddr bool
	}{
		{"NonNilToNonNil", false, false},
		{"NonNilToNil", false, true},
		{"NilToNonNil", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbound := &mockOutbound{}
			nodeIDs := testUtil.UniqueNodeIDs(4)
			b := newTestBroker(t, loopInterval, outbound)

			b.Start(t.Context(), nodeIDs[0])

			srcID := nodeIDs[1]
			b.UpdateNextNodeIDs(nil, []*types.NodeID{srcID})

			var firstTailAddr *types.NodeID
			if !tt.firstNilTailAddr {
				firstTailAddr = nodeIDs[2]
			}
			var finalTailAddr *types.NodeID
			if !tt.finalNilTailAddr {
				finalTailAddr = nodeIDs[3]
			}

			b.processSectorInformation(&sectorInformationParam{
				srcNodeID:   srcID,
				tailAddress: firstTailAddr,
			})
			b.processSectorInformation(&sectorInformationParam{
				srcNodeID:   srcID,
				tailAddress: finalTailAddr,
			})

			fwd, tail := b.GetFrontwardState()
			assert.NotNil(t, fwd)
			if tt.finalNilTailAddr {
				assert.Nil(t, tail)
			} else {
				assert.NotNil(t, tail)
				assert.Equal(t, *finalTailAddr, *tail)
			}
		})
	}
}
