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
package seed

import (
	"context"
	"testing"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type AssignmentHandlerHelper struct {
	T             *testing.T
	AssignNodeF   func(ctx context.Context) (*shared.NodeID, error)
	UnassignNodeF func(nodeID *shared.NodeID)
}

var _ AssignmentHandler = (*AssignmentHandlerHelper)(nil)

func (h *AssignmentHandlerHelper) AssignNode(ctx context.Context) (*shared.NodeID, error) {
	if h.AssignNodeF == nil {
		h.T.FailNow()
	}
	return h.AssignNodeF(ctx)
}

func (h *AssignmentHandlerHelper) UnassignNode(nodeID *shared.NodeID) {
	if h.UnassignNodeF == nil {
		h.T.FailNow()
	}
	h.UnassignNodeF(nodeID)
}

type MultiSeedHandlerHelper struct {
	T                   *testing.T
	GetNodeReportsF     func(ctx context.Context, from, to *shared.NodeID) (map[shared.NodeID]*NodeReport, error)
	ReportDisconnectedF func(ctx context.Context, target *shared.NodeID, from []*shared.NodeID) error
	ClearDisconnectedF  func(ctx context.Context, from *shared.NodeID, target []*shared.NodeID) error
	GetNodeCountF       func(ctx context.Context) (uint64, error)
	RelaySignalF        func(ctx context.Context, signal *proto.Signal, relayToNext bool) error
}

var _ MultiSeedHandler = (*MultiSeedHandlerHelper)(nil)

func (h *MultiSeedHandlerHelper) GetNodeReports(ctx context.Context, from, to *shared.NodeID) (map[shared.NodeID]*NodeReport, error) {
	if h.GetNodeReportsF == nil {
		h.T.FailNow()
	}
	return h.GetNodeReportsF(ctx, from, to)
}

func (h *MultiSeedHandlerHelper) ReportDisconnected(ctx context.Context, target *shared.NodeID, from []*shared.NodeID) error {
	if h.ReportDisconnectedF == nil {
		h.T.FailNow()
	}
	return h.ReportDisconnectedF(ctx, target, from)
}

func (h *MultiSeedHandlerHelper) ClearDisconnected(ctx context.Context, from *shared.NodeID, target []*shared.NodeID) error {
	if h.ClearDisconnectedF == nil {
		h.T.FailNow()
	}
	return h.ClearDisconnectedF(ctx, from, target)
}

func (h *MultiSeedHandlerHelper) GetNodeCount(ctx context.Context) (uint64, error) {
	if h.GetNodeCountF == nil {
		h.T.FailNow()
	}
	return h.GetNodeCountF(ctx)
}

func (h *MultiSeedHandlerHelper) RelaySignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	if h.RelaySignalF == nil {
		h.T.FailNow()
	}
	return h.RelaySignalF(ctx, signal, relayToNext)
}
