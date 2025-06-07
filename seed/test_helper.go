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
	"github.com/stretchr/testify/require"
)

type AssignmentHandlerHelper struct {
	T             *testing.T
	AssignNodeF   func(ctx context.Context) (*shared.NodeID, error)
	UnassignNodeF func(nodeID *shared.NodeID)
}

var _ AssignmentHandler = (*AssignmentHandlerHelper)(nil)

func (h *AssignmentHandlerHelper) AssignNode(ctx context.Context) (*shared.NodeID, error) {
	require.NotNil(h.T, h.AssignNodeF, "AssignNodeF must not be nil")
	return h.AssignNodeF(ctx)
}

func (h *AssignmentHandlerHelper) UnassignNode(nodeID *shared.NodeID) {
	require.NotNil(h.T, h.UnassignNodeF, "UnassignNodeF must not be nil")
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
	require.NotNil(h.T, h.GetNodeReportsF, "GetNodeReportsF must not be nil")
	return h.GetNodeReportsF(ctx, from, to)
}

func (h *MultiSeedHandlerHelper) ReportDisconnected(ctx context.Context, target *shared.NodeID, from []*shared.NodeID) error {
	require.NotNil(h.T, h.ReportDisconnectedF, "ReportDisconnectedF must not be nil")
	return h.ReportDisconnectedF(ctx, target, from)
}

func (h *MultiSeedHandlerHelper) ClearDisconnected(ctx context.Context, from *shared.NodeID, target []*shared.NodeID) error {
	require.NotNil(h.T, h.ClearDisconnectedF, "ClearDisconnectedF must not be nil")
	return h.ClearDisconnectedF(ctx, from, target)
}

func (h *MultiSeedHandlerHelper) GetNodeCount(ctx context.Context) (uint64, error) {
	require.NotNil(h.T, h.GetNodeCountF, "GetNodeCountF must not be nil")
	return h.GetNodeCountF(ctx)
}

func (h *MultiSeedHandlerHelper) RelaySignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	require.NotNil(h.T, h.RelaySignalF, "RelaySignalF must not be nil")
	return h.RelaySignalF(ctx, signal, relayToNext)
}
