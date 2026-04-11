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
package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/llamerada-jp/colonio/types"
	"github.com/stretchr/testify/assert"
)

type mockInfrastructure struct {
	calls   []types.KvsState
	returns []types.KvsState
	err     error
}

var _ Infrastructure = &mockInfrastructure{}

func (m *mockInfrastructure) send(_ context.Context, state types.KvsState) (types.KvsState, error) {
	m.calls = append(m.calls, state)
	if m.err != nil {
		return types.KvsStateUnknown, m.err
	}

	if len(m.returns) > 0 {
		r := m.returns[0]
		m.returns = m.returns[1:]
		return r, nil
	}

	return types.KvsStateUnknown, nil
}

func (m *mockInfrastructure) getCalls() []types.KvsState {
	if m.calls == nil {
		return []types.KvsState{}
	}
	return m.calls
}

func TestState_Get(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(s *State)
		returns        []types.KvsState
		err            error
		expectState    types.KvsState
		expectErr      bool
		expectCalls    []types.KvsState
		expectSent     types.KvsState
		expectReceived types.KvsState
		expectTSZero   bool
	}{
		{
			name:           "FirstCallSendsInactive",
			setup:          func(_ *State) {},
			returns:        []types.KvsState{types.KvsStateActive},
			expectState:    types.KvsStateActive,
			expectCalls:    []types.KvsState{types.KvsStateInactive},
			expectSent:     types.KvsStateInactive,
			expectReceived: types.KvsStateActive,
		},
		{
			name: "ValidCacheSkipsSend",
			setup: func(s *State) {
				s.sent = types.KvsStateActive
				s.received = types.KvsStateInactive
				s.timestamp = time.Now()
			},
			expectState:    types.KvsStateInactive,
			expectCalls:    []types.KvsState{},
			expectSent:     types.KvsStateActive,
			expectReceived: types.KvsStateInactive,
		},
		{
			name: "ExpiredCacheSendsAgain",
			setup: func(s *State) {
				s.sent = types.KvsStateActive
				s.received = types.KvsStateInactive
				s.timestamp = time.Now().Add(-2 * time.Second)
			},
			returns:        []types.KvsState{types.KvsStateActive},
			expectState:    types.KvsStateActive,
			expectCalls:    []types.KvsState{types.KvsStateActive},
			expectSent:     types.KvsStateActive,
			expectReceived: types.KvsStateActive,
		},
		{
			name:           "SendError",
			setup:          func(_ *State) {},
			err:            errors.New("mock error"),
			expectState:    types.KvsStateUnknown,
			expectErr:      true,
			expectCalls:    []types.KvsState{types.KvsStateInactive},
			expectSent:     types.KvsStateUnknown,
			expectReceived: types.KvsStateUnknown,
			expectTSZero:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			infra := &mockInfrastructure{returns: tt.returns, err: tt.err}
			s := NewState(&Config{Infrastructure: infra, Lifespan: time.Second})
			tt.setup(s)

			got, err := s.Get(t.Context())

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectState, got)
			assert.Equal(t, tt.expectCalls, infra.getCalls())
			assert.Equal(t, tt.expectSent, s.sent)
			assert.Equal(t, tt.expectReceived, s.received)
			if tt.expectTSZero {
				assert.True(t, s.timestamp.IsZero())
			} else {
				assert.False(t, s.timestamp.IsZero())
			}
		})
	}
}

func TestState_Set(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(s *State)
		setState       types.KvsState
		returns        []types.KvsState
		err            error
		expectErr      bool
		expectCalls    []types.KvsState
		expectSent     types.KvsState
		expectReceived types.KvsState
	}{
		{
			name: "SameStateSkipsSend",
			setup: func(s *State) {
				s.sent = types.KvsStateActive
				s.received = types.KvsStateInactive
				s.timestamp = time.Now()
			},
			setState:       types.KvsStateActive,
			expectCalls:    []types.KvsState{},
			expectSent:     types.KvsStateActive,
			expectReceived: types.KvsStateInactive,
		},
		{
			name:           "StateChangeSends",
			setup:          func(_ *State) {},
			setState:       types.KvsStateActive,
			returns:        []types.KvsState{types.KvsStateInactive},
			expectCalls:    []types.KvsState{types.KvsStateActive},
			expectSent:     types.KvsStateActive,
			expectReceived: types.KvsStateInactive,
		},
		{
			name: "SendErrorKeepsOldState",
			setup: func(s *State) {
				s.sent = types.KvsStateInactive
				s.received = types.KvsStateActive
				s.timestamp = time.Now()
			},
			setState:       types.KvsStateActive,
			err:            errors.New("mock error"),
			expectErr:      true,
			expectCalls:    []types.KvsState{types.KvsStateActive},
			expectSent:     types.KvsStateInactive,
			expectReceived: types.KvsStateActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			infra := &mockInfrastructure{returns: tt.returns, err: tt.err}
			s := NewState(&Config{Infrastructure: infra, Lifespan: time.Second})
			tt.setup(s)

			before := s.timestamp
			err := s.Set(t.Context(), tt.setState)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectCalls, infra.getCalls())
			assert.Equal(t, tt.expectSent, s.sent)
			assert.Equal(t, tt.expectReceived, s.received)
			if len(tt.expectCalls) == 0 || tt.expectErr {
				assert.Equal(t, before, s.timestamp)
			} else {
				assert.False(t, s.timestamp.IsZero())
			}
		})
	}
}

func TestState_Set_PanicsOnUnknown(t *testing.T) {
	infra := &mockInfrastructure{}
	s := NewState(&Config{Infrastructure: infra, Lifespan: time.Second})

	assert.PanicsWithValue(t, "logic error: state should not be KvsStateUnknown", func() {
		_ = s.Set(t.Context(), types.KvsStateUnknown)
	})

	assert.Empty(t, infra.getCalls())
}
