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
package kvs

import (
	"sync"
	"testing"

	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
)

type transfererHandlerHelper struct {
	mtx        sync.Mutex
	sendPacket []*networkTypes.Packet
	// relay packet is not used in this test
}

var _ transferer.Handler = &transfererHandlerHelper{}

func (h *transfererHandlerHelper) TransfererSendPacket(p *networkTypes.Packet) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.sendPacket = append(h.sendPacket, p)
}

func (h *transfererHandlerHelper) TransfererRelayPacket(nid *types.NodeID, p *networkTypes.Packet) {
	panic("not used in this test")
}

type kvsHandlerHelper struct {
	mtx                  sync.Mutex
	isStable             bool
	backwardNextNodeIDs  []*types.NodeID
	frontwardNextNodeIDs []*types.NodeID
	// SectorState is not used in this test yet
}

var _ Handler = &kvsHandlerHelper{}

/* Unused yet.
func (h *kvsHandlerHelper) setStability(isStable bool, backwardNextNodeIDs, frontwardNextNodeIDs []*shared.NodeID) {
h.mtx.Lock()
defer h.mtx.Unlock()
h.isStable = isStable
h.backwardNextNodeIDs = backwardNextNodeIDs
h.frontwardNextNodeIDs = frontwardNextNodeIDs
}
//*/

func (h *kvsHandlerHelper) KvsGetStability() (bool, []*types.NodeID, []*types.NodeID) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.isStable, h.backwardNextNodeIDs, h.frontwardNextNodeIDs
}

// Placeholder to keep the test file valid until more KVS-level tests are added.
func TestKVS_placeholder(_ *testing.T) {}
