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
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio/types"
)

type sectorInformationEntry struct {
	tailAddress types.NodeID
	timestamp   time.Time
}

type Broker struct {
	logger          *slog.Logger
	infrastructure  Infrastructure
	interval        time.Duration
	signal          chan struct{}
	localNodeID     *types.NodeID
	mtx             sync.Mutex
	tailAddress     *types.NodeID
	backwardNodeIDs []*types.NodeID
	frontwardNodeID types.NodeID
	siEntries       map[types.NodeID]sectorInformationEntry
}

type Config struct {
	Logger         *slog.Logger
	Infrastructure Infrastructure
	Interval       time.Duration
}

func NewBroker(config *Config) *Broker {
	return &Broker{
		logger:         config.Logger,
		infrastructure: config.Infrastructure,
		interval:       config.Interval,
		signal:         make(chan struct{}, 1),
		siEntries:      make(map[types.NodeID]sectorInformationEntry),
	}
}

func (b *Broker) Start(ctx context.Context, localNodeID *types.NodeID) {
	b.localNodeID = localNodeID

	// Send sector information to backward nodes periodically and when the sector information is updated.
	go func() {
		timer := time.NewTimer(b.interval)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-b.signal:
				b.sendSectorInformation()
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(b.interval)

			case <-timer.C:
				b.sendSectorInformation()
				timer.Reset(b.interval)
			}
		}
	}()
}

func (b *Broker) sendSectorInformation() {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	for _, dst := range b.backwardNodeIDs {
		b.infrastructure.sendSectorInformation(dst, b.tailAddress)
	}
}

// UpdateNextNodeIDs updates next node IDs of the sector and notifies the change to backward nodes.
func (b *Broker) UpdateNextNodeIDs(backward, frontward []*types.NodeID) {
	b.mtx.Lock()
	changed := !slices.EqualFunc(backward, b.backwardNodeIDs, func(a, b *types.NodeID) bool {
		return a.Equal(b)
	})
	b.backwardNodeIDs = backward
	if len(frontward) > 0 {
		b.frontwardNodeID = *frontward[0]
	}
	b.mtx.Unlock()

	if changed {
		b.notify()
	}
}

// UpdateTailAddress updates tail address of the sector and notifies the change to backward nodes.
func (b *Broker) UpdateTailAddress(tailAddress *types.NodeID) {
	b.mtx.Lock()
	changed := !tailAddress.Equal(b.tailAddress)
	b.tailAddress = tailAddress
	b.mtx.Unlock()

	if changed {
		b.notify()
	}
}

// notify sends a non-blocking signal to trigger sendSectorInformation.
// Safe to call outside the lock; redundant signals are harmless.
func (b *Broker) notify() {
	select {
	case b.signal <- struct{}{}:
	default:
	}
}

// GetFrontwardState returns frontward node ID and tail address of the sector.
// If the sector information is not available, it returns nil.
func (b *Broker) GetFrontwardState() (*types.NodeID, *types.NodeID) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// Cleanup expired entries.
	now := time.Now()
	for nodeID, entry := range b.siEntries {
		if now.Sub(entry.timestamp) > b.interval*2 {
			delete(b.siEntries, nodeID)
		}
	}

	if entry, ok := b.siEntries[b.frontwardNodeID]; ok {
		fwd := b.frontwardNodeID
		ta := entry.tailAddress
		return &fwd, &ta
	}

	return nil, nil
}

type sectorInformationParam struct {
	srcNodeID   *types.NodeID
	tailAddress *types.NodeID
}

// Called from gateway.
func (b *Broker) processSectorInformation(param *sectorInformationParam) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	b.siEntries[*param.srcNodeID] = sectorInformationEntry{
		tailAddress: *param.tailAddress,
		timestamp:   time.Now(),
	}
}
