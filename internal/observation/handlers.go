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
package observation

type Handlers struct {
	OnChangeConnectedNodes    func(map[string]struct{})
	OnUpdateRequiredNodeIDs1D func(map[string]struct{})
	OnUpdateRequiredNodeIDs2D func(map[string]struct{})
}

type Caller interface {
	ChangeConnectedNodes(map[string]struct{})
	UpdateRequiredNodeIDs1D(map[string]struct{})
	UpdateRequiredNodeIDs2D(map[string]struct{})
}

var _ Caller = &Handlers{}

func (h *Handlers) ChangeConnectedNodes(p1 map[string]struct{}) {
	if h.OnChangeConnectedNodes != nil {
		go h.OnChangeConnectedNodes(p1)
	}
}

func (h *Handlers) UpdateRequiredNodeIDs1D(p1 map[string]struct{}) {
	if h.OnUpdateRequiredNodeIDs1D != nil {
		go h.OnUpdateRequiredNodeIDs1D(p1)
	}
}

func (h *Handlers) UpdateRequiredNodeIDs2D(p1 map[string]struct{}) {
	if h.OnUpdateRequiredNodeIDs2D != nil {
		go h.OnUpdateRequiredNodeIDs2D(p1)
	}
}
