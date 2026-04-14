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
package types

// SectorNo is a number assigned sequentially to nodes constituting a Sector.
// It is unique within the Sector but not unique across the entire colonio cluster.
// This number is used as the node ID for etcd raft in the internal implementation.
// As a convention, avoid using 0, and are not reused within the same Sector.
// When the same node leaves a Sector and rejoins it, a different number is assigned.
type SectorNo uint64

const (
	// Each Sector has one host node. The SectorNo of that node is fixed at 1.
	// The host node does not necessarily match the Raft leader.
	HostNodeSectorNo = SectorNo(1)
)
