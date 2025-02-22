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
package util

import (
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"slices"

	"github.com/llamerada-jp/colonio/internal/shared"
)

// make unique nodeIDs
func UniqueNodeIDs(count int) []*shared.NodeID {
	nodeIDs := make([]*shared.NodeID, count)
	exists := make(map[shared.NodeID]struct{})

	for i := range nodeIDs {
		for {
			nodeID := shared.NewRandomNodeID()
			_, ok := exists[*nodeID]
			if !ok {
				nodeIDs[i] = nodeID
				exists[*nodeID] = struct{}{}
				break
			}
		}
	}

	return nodeIDs
}

// make unique nodeIDs with min max range (min <= nodeID < max)
func UniqueNodeIDsWithRange(min, max *shared.NodeID, count int) []*shared.NodeID {
	if !min.Smaller(max) {
		panic("min must be smaller than max")
	}
	nodeIDs := make([]*shared.NodeID, count)
	exists := make(map[shared.NodeID]struct{})

	for i := range nodeIDs {
		for {
			nodeID := shared.NewRandomNodeID()
			if nodeID.Smaller(min) || !nodeID.Smaller(max) {
				continue
			}
			_, ok := exists[*nodeID]
			if !ok {
				nodeIDs[i] = nodeID
				exists[*nodeID] = struct{}{}
				break
			}
		}
	}

	return nodeIDs
}

func SortNodeIDs(nodeIDs []*shared.NodeID) {
	slices.SortFunc(nodeIDs, func(a, b *shared.NodeID) int {
		return a.Compare(b)
	})
}

// create client to accept self-signed certificate & cookie
func NewInsecureHttpClient() *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Jar: jar,
	}
}
