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
package datastore

import (
	"encoding/json"
	"time"
)

type Reader struct {
	rawReader RawReader
}

func NewReader(rawReader RawReader) *Reader {
	return &Reader{
		rawReader: rawReader,
	}
}

func (r *Reader) Read(v any) (*time.Time, string, error) {
	timestamp, nodeID, raw, err := r.rawReader.Read()
	if len(raw) == 0 {
		return timestamp, nodeID, err
	}

	if err := json.Unmarshal(raw, v); err != nil {
		return nil, "", err
	}

	return timestamp, nodeID, nil
}

func (r *Reader) Close() {
	r.rawReader.Close()
}
