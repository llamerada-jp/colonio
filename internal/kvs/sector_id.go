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
	"github.com/google/uuid"
	"github.com/llamerada-jp/colonio/config"
)

func MustMarshalSectorID(sectorID config.SectorID) []byte {
	data, err := uuid.UUID(sectorID).MarshalBinary()
	if err != nil {
		panic(err)
	}
	return data
}

func UnmarshalSectorID(data []byte) (config.SectorID, error) {
	var sectorID uuid.UUID
	err := sectorID.UnmarshalBinary(data)
	if err != nil {
		return config.SectorID(uuid.Nil), err
	}
	return config.SectorID(sectorID), nil
}
