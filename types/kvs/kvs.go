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

import (
	"fmt"
)

var (
	ErrorStoreInvalidNodeKey = fmt.Errorf("invalid node key")
	ErrorStoreKeyNotFound    = fmt.Errorf("key not found")
	// ErrorSectorNotReady is a retryable rejection of a KVS operation: the
	// sector is not activated yet, the key is outside the sector's current
	// range (stale routing, or a split/merge just moved it), or the range is
	// being handed over (split export in progress / merge lock held). Mapped
	// to KvsOperationResponse ERROR_PREPARING so the client can retry.
	ErrorSectorNotReady = fmt.Errorf("sector is not ready for the operation")
	// ErrorOperationResultUnknown marks a write whose outcome is genuinely
	// unknown: the proposal timed out on the host (or the response carried an
	// unexpected code), but raft gives no negative acknowledgment, so it may
	// still commit later. Retrying blindly can apply the write twice; the
	// public client (node/kvs) only auto-retries this class for conditional
	// (CAS) operations.
	ErrorOperationResultUnknown = fmt.Errorf("operation result is unknown")
	// ErrorCasConflict means a conditional write (expected revision / expected
	// absence) found a different state at apply time. The store was left
	// untouched. Not blindly retryable: re-read the record and decide.
	ErrorCasConflict = fmt.Errorf("compare-and-swap conflict")
)

type Store interface {
	AllocateSector(sectorKey *SectorKey) error
	ReleaseSector(sectorKey *SectorKey) error

	Get(sectorKey *SectorKey, key string) ([]byte, error)
	Set(sectorKey *SectorKey, key string, value []byte) error
	Delete(sectorKey *SectorKey, key string) error
}

type GetResult struct {
	Data []byte
	// Revision is the record's revision, used as the expected value of a
	// conditional write (CAS). Assigned from the sector's revision counter;
	// never 0 for an existing record.
	Revision uint64
	Err      error
}

type SetResult struct {
	// Revision is the newly assigned revision of the written record.
	Revision uint64
	Err      error
}
