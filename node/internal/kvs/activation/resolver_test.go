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
	calls   []kvsTypes.ActivationState
	returns []kvsTypes.ActivationState
	err     error
}

var _ OutboundPort = &mockInOutbound{}

func (m *mockInOutbound) send(_ context.Context, state kvsTypes.ActivationState) (kvsTypes.ActivationState, error) {
	m.calls = append(m.calls, state)
	if m.err != nil {
		return kvsTypes.ActivationStateUnknown, m.err
	}

	if len(m.returns) > 0 {
		r := m.returns[0]
		m.returns = m.returns[1:]
		return r, nil
	}

	return kvsTypes.ActivationStateUnknown, nil
}

func (m *mockInOutbound) getCalls() []kvsTypes.ActivationState {
	if m.calls == nil {
		return []kvsTypes.ActivationState{}
	}
	return m.calls
}

func TestResolver_ResolveEntireState(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(s *Resolver)
		returns        []kvsTypes.ActivationState
		err            error
		expectState    kvsTypes.ActivationState
		expectErr      bool
		expectCalls    []kvsTypes.ActivationState
		expectSent     kvsTypes.ActivationState
		expectReceived kvsTypes.ActivationState
		expectTSZero   bool
	}{
		{
			name:           "FirstCallSendsInactive",
			setup:          func(_ *Resolver) {},
			returns:        []kvsTypes.ActivationState{kvsTypes.ActivationStateActive},
			expectState:    kvsTypes.ActivationStateActive,
			expectCalls:    []kvsTypes.ActivationState{kvsTypes.ActivationStateInactive},
			expectSent:     kvsTypes.ActivationStateInactive,
			expectReceived: kvsTypes.ActivationStateActive,
		},
		{
			name: "ValidCacheSkipsSend",
			setup: func(s *Resolver) {
				s.sectorState = kvsTypes.ActivationStateActive
				s.entireState = kvsTypes.ActivationStateInactive
				s.resolvedAt = time.Now()
			},
			expectState:    kvsTypes.ActivationStateInactive,
			expectCalls:    []kvsTypes.ActivationState{},
			expectSent:     kvsTypes.ActivationStateActive,
			expectReceived: kvsTypes.ActivationStateInactive,
		},
		{
			name: "ExpiredCacheSendsAgain",
			setup: func(s *Resolver) {
				s.sectorState = kvsTypes.ActivationStateActive
				s.entireState = kvsTypes.ActivationStateInactive
				s.resolvedAt = time.Now().Add(-2 * time.Second)
			},
			returns:        []kvsTypes.ActivationState{kvsTypes.ActivationStateActive},
			expectState:    kvsTypes.ActivationStateActive,
			expectCalls:    []kvsTypes.ActivationState{kvsTypes.ActivationStateActive},
			expectSent:     kvsTypes.ActivationStateActive,
			expectReceived: kvsTypes.ActivationStateActive,
		},
		{
			name:           "SendError",
			setup:          func(_ *Resolver) {},
			err:            errors.New("mock error"),
			expectState:    kvsTypes.ActivationStateUnknown,
			expectErr:      true,
			expectCalls:    []kvsTypes.ActivationState{kvsTypes.ActivationStateInactive},
			expectSent:     kvsTypes.ActivationStateUnknown,
			expectReceived: kvsTypes.ActivationStateUnknown,
			expectTSZero:   true,
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
		setState       kvsTypes.ActivationState
		returns        []kvsTypes.ActivationState
		err            error
		expectErr      bool
		expectCalls    []kvsTypes.ActivationState
		expectSent     kvsTypes.ActivationState
		expectReceived kvsTypes.ActivationState
	}{
		{
			name: "SameStateSkipsSend",
			setup: func(s *Resolver) {
				s.sectorState = kvsTypes.ActivationStateActive
				s.entireState = kvsTypes.ActivationStateInactive
				s.resolvedAt = time.Now()
			},
			setState:       kvsTypes.ActivationStateActive,
			expectCalls:    []kvsTypes.ActivationState{},
			expectSent:     kvsTypes.ActivationStateActive,
			expectReceived: kvsTypes.ActivationStateInactive,
		},
		{
			name:           "StateChangeSends",
			setup:          func(_ *Resolver) {},
			setState:       kvsTypes.ActivationStateActive,
			returns:        []kvsTypes.ActivationState{kvsTypes.ActivationStateInactive},
			expectCalls:    []kvsTypes.ActivationState{kvsTypes.ActivationStateActive},
			expectSent:     kvsTypes.ActivationStateActive,
			expectReceived: kvsTypes.ActivationStateInactive,
		},
		{
			name: "SendErrorKeepsOldState",
			setup: func(s *Resolver) {
				s.sectorState = kvsTypes.ActivationStateInactive
				s.entireState = kvsTypes.ActivationStateActive
				s.resolvedAt = time.Now()
			},
			setState:       kvsTypes.ActivationStateActive,
			err:            errors.New("mock error"),
			expectErr:      true,
			expectCalls:    []kvsTypes.ActivationState{kvsTypes.ActivationStateActive},
			expectSent:     kvsTypes.ActivationStateInactive,
			expectReceived: kvsTypes.ActivationStateActive,
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

func TestResolver_SetSectorState_PanicsOnUnknown(t *testing.T) {
	outbound := &mockInOutbound{}
	s := NewResolver(&Config{Outbound: outbound, CacheTTL: time.Second})

	assert.PanicsWithValue(t, "logic error: state should not be SectorStateUnknown", func() {
		_ = s.SetSectorState(t.Context(), kvsTypes.ActivationStateUnknown)
	})

	assert.Empty(t, outbound.getCalls())
}
