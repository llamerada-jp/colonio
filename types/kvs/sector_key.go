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

import "fmt"

// Colonio manages the KVS address space by partitioning it. A sector refers to
// one of these partitioned segments. A Sector consists of a host Node and
// the replication Nodes before and after it. Both nodes are member of a Raft cluster.
// The Store acts as a state machine.
type SectorKey struct {
	SectorID SectorID
	SectorNo SectorNo
}

func (s *SectorKey) String() string {
	return fmt.Sprintf("%s/%d", s.SectorID.String(), s.SectorNo)
}
