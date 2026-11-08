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
package base

const (
	StateStart  = "start"
	StateNormal = ""
	StateStop   = "stop"
)

type SectorInfo struct {
	SectorID string `json:"sector_id"`
	SectorNo uint64 `json:"sector_no"`
	Head     string `json:"head"`
	Tail     string `json:"tail,omitempty"`
}

type Record struct {
	State             string       `json:"state"`
	IsOnline          bool         `json:"is_online"`
	IsStable          bool         `json:"is_stable"`
	ConnectedNodeIDs  []string     `json:"connected_node_ids"`
	RequiredNodeIDs1D []string     `json:"required_node_ids_1d"`
	RequiredNodeIDs2D []string     `json:"required_node_ids_2d"`
	SectorInfos       []SectorInfo `json:"sector_infos"`
	// message post time (uuid string : RFC3339Nano string)
	Post map[string]string `json:"post"`
	// message receive time (uuid string : RFC3339Nano string)
	Receive map[string]string `json:"receive"`
}

type RecordInterface interface {
	GetRecord() *Record
}

func (r *Record) GetRecord() *Record {
	return r
}
