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
	"sync"
	"time"

	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

type Resolver struct {
	mux         sync.Mutex
	outbound    OutboundPort
	cacheTTL    time.Duration
	entireState kvsTypes.EntireState // the latest state received from the seed
	sectorState kvsTypes.SectorState
	resolvedAt  time.Time
}

type Config struct {
	Outbound OutboundPort
	CacheTTL time.Duration
}

func NewResolver(config *Config) *Resolver {
	return &Resolver{
		outbound:    config.Outbound,
		sectorState: kvsTypes.SectorStateInactive,
		cacheTTL:    config.CacheTTL,
	}
}

func (s *Resolver) ResolveEntireState(ctx context.Context) (kvsTypes.EntireState, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if time.Since(s.resolvedAt) > s.cacheTTL {
		s.resolvedAt = time.Time{}
	}

	if s.resolvedAt.IsZero() {
		rs, err := s.outbound.send(ctx, s.sectorState)
		if err != nil {
			return kvsTypes.EntireStateUnknown, err
		}
		s.entireState = rs
		s.resolvedAt = time.Now()
	}

	return s.entireState, nil
}

func (s *Resolver) SetSectorState(ctx context.Context, state kvsTypes.SectorState) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if !s.resolvedAt.IsZero() && s.sectorState == state {
		return nil
	}

	rs, err := s.outbound.send(ctx, state)
	if err != nil {
		return err
	}
	s.sectorState = state
	s.entireState = rs
	s.resolvedAt = time.Now()

	return nil
}
