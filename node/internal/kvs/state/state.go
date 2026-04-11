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
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/types"
)

type State struct {
	mux            sync.Mutex
	infrastructure Infrastructure
	lifespan       time.Duration
	received       types.KvsState // the latest state received from the seed
	sent           types.KvsState
	timestamp      time.Time
}

type Config struct {
	Infrastructure Infrastructure
	Lifespan       time.Duration
}

func NewState(config *Config) *State {
	return &State{
		infrastructure: config.Infrastructure,
		sent:           types.KvsStateUnknown,
		lifespan:       config.Lifespan,
	}
}

func (s *State) Get(ctx context.Context) (types.KvsState, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if time.Since(s.timestamp) > s.lifespan {
		s.timestamp = time.Time{}
	}

	if s.timestamp.IsZero() {
		ss := s.sent
		if ss == types.KvsStateUnknown {
			ss = types.KvsStateInactive
		}
		rs, err := s.infrastructure.send(ctx, ss)
		if err != nil {
			return types.KvsStateUnknown, err
		}
		s.sent = ss
		s.received = rs
		s.timestamp = time.Now()
	}

	return s.received, nil
}

func (s *State) Set(ctx context.Context, state types.KvsState) error {
	if state == types.KvsStateUnknown {
		panic("logic error: state should not be KvsStateUnknown")
	}

	s.mux.Lock()
	defer s.mux.Unlock()

	if s.sent == state {
		return nil
	}

	rs, err := s.infrastructure.send(ctx, state)
	if err != nil {
		return err
	}
	s.sent = state
	s.received = rs
	s.timestamp = time.Now()

	return nil
}
