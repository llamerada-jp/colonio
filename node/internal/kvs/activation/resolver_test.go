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
package activation

import (
	"context"
	"errors"
	"testing"
	"time"

	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/assert"
)

type mockInOutbound struct {
	calls   []kvsTypes.SectorState
	returns []kvsTypes.EntireState
	err     error
}

var _ OutboundPort = &mockInOutbound{}

func (m *mockInOutbound) send(_ context.Context, state kvsTypes.SectorState) (kvsTypes.EntireState, error) {
	m.calls = append(m.calls, state)
	if m.err != nil {
		return kvsTypes.EntireStateUnknown, m.err
	}

	if len(m.returns) > 0 {
		r := m.returns[0]
		m.returns = m.returns[1:]
		return r, nil
	}

	return kvsTypes.EntireStateUnknown, nil
}

func (m *mockInOutbound) getCalls() []kvsTypes.SectorState {
	if m.calls == nil {
		return []kvsTypes.SectorState{}
	}
	return m.calls
}

func TestResolver_ResolveEntireState(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(s *Resolver)
		returns        []kvsTypes.EntireState
		err            error
		expectState    kvsTypes.EntireState
		expectErr      bool
		expectCalls    []kvsTypes.SectorState
		expectSent     kvsTypes.SectorState
		expectReceived kvsTypes.EntireState
		expectTSZero   bool
	}{
		{
			name:           "FirstCallSendsInactive",
			setup:          func(_ *Resolver) {},
			returns:        []kvsTypes.EntireState{kvsTypes.EntireStateActive},
			expectState:    kvsTypes.EntireStateActive,
			expectCalls:    []kvsTypes.SectorState{kvsTypes.SectorStateInactive},
			expectSent:     kvsTypes.SectorStateInactive,
			expectReceived: kvsTypes.EntireStateActive,
		},
		{
			name: "ValidCacheSkipsSend",
			setup: func(s *Resolver) {
				s.sectorState = kvsTypes.SectorStateActive
				s.entireState = kvsTypes.EntireStateInactive
				s.resolvedAt = time.Now()
			},
			expectState:    kvsTypes.EntireStateInactive,
			expectCalls:    []kvsTypes.SectorState{},
			expectSent:     kvsTypes.SectorStateActive,
			expectReceived: kvsTypes.EntireStateInactive,
		},
		{
			name: "ExpiredCacheSendsAgain",
			setup: func(s *Resolver) {
				s.sectorState = kvsTypes.SectorStateActive
				s.entireState = kvsTypes.EntireStateInactive
				s.resolvedAt = time.Now().Add(-2 * time.Second)
			},
			returns:        []kvsTypes.EntireState{kvsTypes.EntireStateActive},
			expectState:    kvsTypes.EntireStateActive,
			expectCalls:    []kvsTypes.SectorState{kvsTypes.SectorStateActive},
			expectSent:     kvsTypes.SectorStateActive,
			expectReceived: kvsTypes.EntireStateActive,
		},
		{
			name: "ZeroCacheTTLInvalidatesImmediately",
			setup: func(s *Resolver) {
				s.cacheTTL = 0
				s.sectorState = kvsTypes.SectorStateActive
				s.entireState = kvsTypes.EntireStateInactive
				s.resolvedAt = time.Now()
			},
			returns:        []kvsTypes.EntireState{kvsTypes.EntireStateActive},
			expectState:    kvsTypes.EntireStateActive,
			expectCalls:    []kvsTypes.SectorState{kvsTypes.SectorStateActive},
			expectSent:     kvsTypes.SectorStateActive,
			expectReceived: kvsTypes.EntireStateActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbound := &mockInOutbound{returns: tt.returns, err: tt.err}
			s := NewResolver(&Config{Outbound: outbound, CacheTTL: time.Second})
			tt.setup(s)

			got, err := s.ResolveEntireState(t.Context())

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectState, got)
			assert.Equal(t, tt.expectCalls, outbound.getCalls())
			assert.Equal(t, tt.expectSent, s.sectorState)
			assert.Equal(t, tt.expectReceived, s.entireState)
			if tt.expectTSZero {
				assert.True(t, s.resolvedAt.IsZero())
			} else {
				assert.False(t, s.resolvedAt.IsZero())
			}
		})
	}
}

func TestResolver_SetSectorState(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(s *Resolver)
		setState       kvsTypes.SectorState
		returns        []kvsTypes.EntireState
		err            error
		expectErr      bool
		expectCalls    []kvsTypes.SectorState
		expectSent     kvsTypes.SectorState
		expectReceived kvsTypes.EntireState
	}{
		{
			name: "SameStateSkipsSend",
			setup: func(s *Resolver) {
				s.sectorState = kvsTypes.SectorStateActive
				s.entireState = kvsTypes.EntireStateInactive
				s.resolvedAt = time.Now()
			},
			setState:       kvsTypes.SectorStateActive,
			expectCalls:    []kvsTypes.SectorState{},
			expectSent:     kvsTypes.SectorStateActive,
			expectReceived: kvsTypes.EntireStateInactive,
		},
		{
			name: "SameInactiveStateSkipsSend",
			setup: func(s *Resolver) {
				s.sectorState = kvsTypes.SectorStateInactive
				s.entireState = kvsTypes.EntireStateActive
				s.resolvedAt = time.Now()
			},
			setState:       kvsTypes.SectorStateInactive,
			expectCalls:    []kvsTypes.SectorState{},
			expectSent:     kvsTypes.SectorStateInactive,
			expectReceived: kvsTypes.EntireStateActive,
		},
		{
			name:           "StateChangeSends",
			setup:          func(_ *Resolver) {},
			setState:       kvsTypes.SectorStateActive,
			returns:        []kvsTypes.EntireState{kvsTypes.EntireStateInactive},
			expectCalls:    []kvsTypes.SectorState{kvsTypes.SectorStateActive},
			expectSent:     kvsTypes.SectorStateActive,
			expectReceived: kvsTypes.EntireStateInactive,
		},
		{
			name: "SendErrorKeepsOldState",
			setup: func(s *Resolver) {
				s.sectorState = kvsTypes.SectorStateInactive
				s.entireState = kvsTypes.EntireStateActive
				s.resolvedAt = time.Now()
			},
			setState:       kvsTypes.SectorStateActive,
			err:            errors.New("mock error"),
			expectErr:      true,
			expectCalls:    []kvsTypes.SectorState{kvsTypes.SectorStateActive},
			expectSent:     kvsTypes.SectorStateInactive,
			expectReceived: kvsTypes.EntireStateActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbound := &mockInOutbound{returns: tt.returns, err: tt.err}
			s := NewResolver(&Config{Outbound: outbound, CacheTTL: time.Second})
			tt.setup(s)

			before := s.resolvedAt
			err := s.SetSectorState(t.Context(), tt.setState)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectCalls, outbound.getCalls())
			assert.Equal(t, tt.expectSent, s.sectorState)
			assert.Equal(t, tt.expectReceived, s.entireState)
			if len(tt.expectCalls) == 0 || tt.expectErr {
				assert.Equal(t, before, s.resolvedAt)
			} else {
				assert.False(t, s.resolvedAt.IsZero())
			}
		})
	}
}

func TestResolver_ResolveEntireState_ConcurrentCalls(t *testing.T) {
	outbound := &mockInOutbound{
		returns: []kvsTypes.EntireState{
			kvsTypes.EntireStateActive,
			kvsTypes.EntireStateActive,
			kvsTypes.EntireStateActive,
		},
	}
	s := NewResolver(&Config{Outbound: outbound, CacheTTL: time.Second})

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := s.ResolveEntireState(t.Context())
			assert.NoError(t, err)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// All 10 goroutines should result in only 1 outbound send due to mutex and caching
	assert.Equal(t, 1, len(outbound.getCalls()))
	assert.Equal(t, kvsTypes.SectorStateInactive, outbound.getCalls()[0])
}
