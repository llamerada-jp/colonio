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
package config

import (
	"fmt"

	"github.com/google/uuid"
)

var (
	ErrorKvsStoreInvalidNodeKey = fmt.Errorf("invalid node key")
	ErrorKvsStoreKeyNotFound    = fmt.Errorf("key not found")
)

// KvsSequence is a number assigned sequentially to nodes constituting a Sector.
// It is unique within the Sector but not unique across the entire colonio cluster.
// Numbers are not reused within the same Sector. When the same node leaves
// a Sector and rejoins it, a different number is assigned.
type KvsSequence uint64

const (
	// Each Sector has one host node. The Sequence of that node is fixed at 1.
	// The host node does not necessarily match the Raft leader.
	KvsSectorHostNodeSequence = KvsSequence(1)
)

// Colonio manages the KVS address space by partitioning it. A sector refers to
// one of these partitioned segments. A Sector consists of a host Node and
// the replication Nodes before and after it. Both nodes are member of a Raft cluster.
// The Store acts as a state machine.
type KvsSectorKey struct {
	// SectorID is a unique ID assigned to a Sector across the entire colonio cluster.
	// In colonio, sectors are rebuilt when nodes are replaced.
	// At most one valid Store holding any specific address can exist at any time.
	// But there may be Stores processing retirement. We distinguish them using UUID.
	SectorID uuid.UUID
	Sequence KvsSequence
}

type KvsStore interface {
	AllocateSector(sectorKey *KvsSectorKey) error
	ReleaseSector(sectorKey *KvsSectorKey) error

	Get(sectorKey *KvsSectorKey, key string) ([]byte, error)
	Set(sectorKey *KvsSectorKey, key string, value []byte) error
	Patch(sectorKey *KvsSectorKey, key string, value []byte) error
	Delete(sectorKey *KvsSectorKey, key string) error
}

type KvsGetResult struct {
	Data []byte
	Err  error
}
