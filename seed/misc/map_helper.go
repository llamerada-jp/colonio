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

import "github.com/llamerada-jp/colonio/internal/shared"

// TODO: improve performance, this logic is worst: O(n)
func GetNextByMap[T any](nodeID *shared.NodeID, nodeMap map[shared.NodeID]T, defaultVal T) (*shared.NodeID, T) {
	minNodeID := (*shared.NodeID)(nil)
	nextNodeID := (*shared.NodeID)(nil)
	for n := range nodeMap {
		if n.Equal(nodeID) {
			continue
		}
		if minNodeID == nil || n.Smaller(minNodeID) {
			minNodeID = n.Copy()
		}
		if n.Smaller(nodeID) {
			continue
		}
		if nextNodeID == nil || n.Smaller(nextNodeID) {
			nextNodeID = n.Copy()
		}
	}

	if nextNodeID != nil {
		return nextNodeID, nodeMap[*nextNodeID]
	}
	if minNodeID != nil {
		return minNodeID, nodeMap[*minNodeID]
	}
	return nil, defaultVal
}
