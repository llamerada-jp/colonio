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
package misc

import (
	"testing"

	"github.com/llamerada-jp/colonio/internal/shared"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/assert"
)

func TestMapHelper_GetNextByMap(t *testing.T) {
	nodeIDs := testUtil.UniqueNodeIDs(3)
	shared.SortNodeIDs(nodeIDs)

	tests := []struct {
		name            string
		nodeID          *shared.NodeID
		nodeMap         map[shared.NodeID]int
		defaultVal      int
		expectedNextID  *shared.NodeID
		expectedNextVal int
	}{
		{
			name:            "empty map",
			nodeID:          nodeIDs[0],
			nodeMap:         map[shared.NodeID]int{},
			defaultVal:      1,
			expectedNextID:  nil,
			expectedNextVal: 1,
		},
		{
			name:            "only the nodeID not in map",
			nodeID:          nodeIDs[0],
			nodeMap:         map[shared.NodeID]int{*nodeIDs[0]: 10},
			defaultVal:      1,
			expectedNextID:  nil,
			expectedNextVal: 1,
		},
		{
			name:   "normal case 1",
			nodeID: nodeIDs[1],
			nodeMap: map[shared.NodeID]int{
				*nodeIDs[0]: 0,
				*nodeIDs[1]: 10,
				*nodeIDs[2]: 20,
			},
			defaultVal:      1,
			expectedNextID:  nodeIDs[2],
			expectedNextVal: 20,
		},
		{
			name:   "normal case 2",
			nodeID: nodeIDs[2],
			nodeMap: map[shared.NodeID]int{
				*nodeIDs[0]: 0,
				*nodeIDs[1]: 10,
				*nodeIDs[2]: 20,
			},
			defaultVal:      1,
			expectedNextID:  nodeIDs[0],
			expectedNextVal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeID := tt.nodeID // Assuming nodeID is a string for simplicity
			nextID, nextVal := GetNextByMap(nodeID, tt.nodeMap, tt.defaultVal)

			if tt.expectedNextID == nil {
				assert.Nil(t, nextID)
			} else {
				assert.Equal(t, *tt.expectedNextID, *nextID)
			}
			assert.Equal(t, tt.expectedNextVal, nextVal)
		})
	}
}
