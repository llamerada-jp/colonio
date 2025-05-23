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
package util

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
	T            *testing.T
	IsAloneF     func(ctx context.Context, nodeID *shared.NodeID) (bool, error)
	RelaySignalF func(ctx context.Context, signal *proto.Signal, relayToNext bool) error
}

func (h *MultiSeedHandlerHelper) IsAlone(ctx context.Context, nodeID *shared.NodeID) (bool, error) {
	if h.IsAloneF == nil {
		h.T.FailNow()
	}
	return h.IsAloneF(ctx, nodeID)
}

func (h *MultiSeedHandlerHelper) RelaySignal(ctx context.Context, signal *proto.Signal, relayToNext bool) error {
	if h.RelaySignalF == nil {
		h.T.FailNow()
	}
	return h.RelaySignalF(ctx, signal, relayToNext)
}
