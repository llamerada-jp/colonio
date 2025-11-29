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

type ObservationSectorInfo struct {
	Head string
	Tail string
}

// Handler is an interface for observing the internal status of colonio.
// These interfaces are for debugging and simulation and are not intended for normal use.
// They are not guaranteed to work or be compatible.
type ObservationHandler struct {
	OnChangeConnectedNodes    func(map[string]struct{})
	OnChangeKvsSectors        func(map[KvsSectorKey]*ObservationSectorInfo)
	OnUpdateRequiredNodeIDs1D func(map[string]struct{})
	OnUpdateRequiredNodeIDs2D func(map[string]struct{})
}

type ObservationCaller interface {
	ChangeConnectedNodes(map[string]struct{})
	ChangeKvsSectors(map[KvsSectorKey]*ObservationSectorInfo)
	UpdateRequiredNodeIDs1D(map[string]struct{})
	UpdateRequiredNodeIDs2D(map[string]struct{})
}

var _ ObservationCaller = &ObservationHandler{}

func (h *ObservationHandler) ChangeConnectedNodes(p1 map[string]struct{}) {
	if h.OnChangeConnectedNodes != nil {
		go h.OnChangeConnectedNodes(p1)
	}
}

func (h *ObservationHandler) ChangeKvsSectors(p1 map[KvsSectorKey]*ObservationSectorInfo) {
	if h.OnChangeKvsSectors != nil {
		go h.OnChangeKvsSectors(p1)
	}
}

func (h *ObservationHandler) UpdateRequiredNodeIDs1D(p1 map[string]struct{}) {
	if h.OnUpdateRequiredNodeIDs1D != nil {
		go h.OnUpdateRequiredNodeIDs1D(p1)
	}
}

func (h *ObservationHandler) UpdateRequiredNodeIDs2D(p1 map[string]struct{}) {
	if h.OnUpdateRequiredNodeIDs2D != nil {
		go h.OnUpdateRequiredNodeIDs2D(p1)
	}
}
