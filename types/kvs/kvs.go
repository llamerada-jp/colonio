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
	// ErrorStoreSectorExists is returned (wrapped) by Store.AllocateSector
	// when the sector is already allocated.
	ErrorStoreSectorExists = fmt.Errorf("sector already exists")
	// ErrorStoreSectorNotFound is returned (wrapped) by Store.ReleaseSector
	// when the sector is not allocated.
	ErrorStoreSectorNotFound = fmt.Errorf("sector not found")
)

// Store is the backend that holds the records of each sector.
//
// The sector logic calls AllocateSector for a sector that may already be
// allocated, and ReleaseSector for one that may never have been allocated.
// Implementations must report these cases with ErrorStoreSectorExists and
// ErrorStoreSectorNotFound so they can be told apart from real failures.
type Store interface {
	AllocateSector(sectorKey *SectorKey) error
	ReleaseSector(sectorKey *SectorKey) error

	Get(sectorKey *SectorKey, key string) ([]byte, error)
	Set(sectorKey *SectorKey, key string, value []byte) error
	Patch(sectorKey *SectorKey, key string, value []byte) error
	Delete(sectorKey *SectorKey, key string) error
}

type GetResult struct {
	Data []byte
	Err  error
}
