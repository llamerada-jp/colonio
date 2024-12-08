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
	"io"
	"os"
	"time"
)

type FileWriter struct {
	closer  io.Closer
	encoder *json.Encoder
}

func NewFileWriter(fileName string) (*FileWriter, error) {
	w, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		return nil, err
	}

	return &FileWriter{
		closer:  w,
		encoder: json.NewEncoder(w),
	}, nil
}

func (f *FileWriter) Close() {
	f.closer.Close()
}

func (f *FileWriter) Push(timestamp *time.Time, nodeID string, record []byte) {
	f.encoder.Encode(&Entry{
		Timestamp: *timestamp,
		NodeID:    nodeID,
		Record:    record,
	})
}

type FileReader struct {
	closer  io.Closer
	decoder *json.Decoder
}

func NewFileReader(fileName string) (*FileReader, error) {
	r, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}

	return &FileReader{
		closer:  r,
		decoder: json.NewDecoder(r),
	}, nil
}

func (f *FileReader) Close() {
	f.closer.Close()
}

func (f *FileReader) Read(v any) (*time.Time, string, error) {
	if f.decoder.More() {
		var entry Entry
		if err := f.decoder.Decode(&entry); err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(entry.Record, v); err != nil {
			return nil, "", err
		}

		return &entry.Timestamp, entry.NodeID, nil
	}
	return nil, "", nil
}
