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
)

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
