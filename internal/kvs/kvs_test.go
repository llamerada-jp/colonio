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
	"testing"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/shared"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKVS_getNodesToBeChanged(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(5)

	tests := []struct {
		name            string
		memberStates    map[config.SectorNo]*memberStateEntry
		nextNodeIDs     []*shared.NodeID
		expectedAppends []*shared.NodeID
		expectedRemoves map[config.SectorNo]struct{}
	}{
		{
			name:            "empty",
			memberStates:    map[config.SectorNo]*memberStateEntry{},
			nextNodeIDs:     []*shared.NodeID{},
			expectedAppends: []*shared.NodeID{},
			expectedRemoves: map[config.SectorNo]struct{}{},
		},
		{
			name: "new members",
			memberStates: map[config.SectorNo]*memberStateEntry{
				config.KvsHostNodeSectorNo: {
					nodeID: nodeIDs[0],
					state:  memberStateNormal,
				},
			},
			nextNodeIDs:     []*shared.NodeID{nodeIDs[1], nodeIDs[2]},
			expectedAppends: []*shared.NodeID{nodeIDs[1], nodeIDs[2]},
			expectedRemoves: map[config.SectorNo]struct{}{},
		},
		{
			name: "remove members",
			memberStates: map[config.SectorNo]*memberStateEntry{
				1: {
					nodeID: nodeIDs[0],
					state:  memberStateNormal,
				},
				2: {
					nodeID: nodeIDs[1],
					state:  memberStateNormal,
				},
				3: { // to be removed
					nodeID: nodeIDs[2],
					state:  memberStateNormal,
				},
			},
			nextNodeIDs:     []*shared.NodeID{nodeIDs[1]},
			expectedAppends: []*shared.NodeID{},
			expectedRemoves: map[config.SectorNo]struct{}{
				3: {},
			},
		},
		{
			name: "mixed",
			memberStates: map[config.SectorNo]*memberStateEntry{
				1: {
					nodeID: nodeIDs[0],
					state:  memberStateNormal,
				},
				2: {
					nodeID: nodeIDs[1],
					state:  memberStateNormal,
				},
				3: { // to be removed
					nodeID: nodeIDs[2],
					state:  memberStateNormal,
				},
			},
			nextNodeIDs:     []*shared.NodeID{nodeIDs[1], nodeIDs[3]},
			expectedAppends: []*shared.NodeID{nodeIDs[3]},
			expectedRemoves: map[config.SectorNo]struct{}{
				3: {},
			},
		},
		{
			name: "removing node is appended again",
			memberStates: map[config.SectorNo]*memberStateEntry{
				1: {
					nodeID: nodeIDs[0],
					state:  memberStateNormal,
				},
				2: {
					nodeID: nodeIDs[1],
					state:  memberStateRemoving,
				},
				3: {
					nodeID: nodeIDs[2],
					state:  memberStateRemoving,
				},
			},
			nextNodeIDs:     []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[3]},
			expectedAppends: []*shared.NodeID{nodeIDs[1], nodeIDs[3]},
			expectedRemoves: map[config.SectorNo]struct{}{},
		},
		{
			name: "appending node is ignored from appends",
			memberStates: map[config.SectorNo]*memberStateEntry{
				1: {
					nodeID: nodeIDs[0],
					state:  memberStateNormal,
				},
				2: {
					nodeID: nodeIDs[1],
					state:  memberStateCreating,
				},
				3: {
					nodeID: nodeIDs[2], // to be removed
					state:  memberStateAppendingRaft,
				},
				4: {
					nodeID: nodeIDs[3],
					state:  memberStateNormal,
				},
			},
			nextNodeIDs: []*shared.NodeID{nodeIDs[0], nodeIDs[1], nodeIDs[3], nodeIDs[4]},
			expectedAppends: []*shared.NodeID{
				nodeIDs[4],
			},
			expectedRemoves: map[config.SectorNo]struct{}{
				3: {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kvs := &KVS{
				memberStates: tt.memberStates,
			}

			appends, removes := kvs.getNodesToBeChanged(tt.nextNodeIDs)

			require.Len(t, appends, len(tt.expectedAppends))
			for _, appendNodeID := range appends {
				assert.Contains(t, tt.expectedAppends, appendNodeID)
			}
			assert.Equal(t, tt.expectedRemoves, removes)
		})
	}
}
