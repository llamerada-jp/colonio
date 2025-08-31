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

import "github.com/google/uuid"

// ICEServer is a configuration for ICE server. It is used to establish a WebRTC connection.
type ICEServer struct {
	// URLs is a list of URLs of the ICE server.
	URLs []string `json:"urls,omitempty"`
	// Username is a username for the ICE server.
	Username string `json:"username,omitempty"`
	// Credential is a credential for the ICE server.
	Credential string `json:"credential,omitempty"`
}

type KvsNodeKey struct {
	ClusterID uuid.UUID
	Sequence  uint64
}

type KvsStore interface {
	NewCluster(nodeKey *KvsNodeKey) error
	DeleteCluster(nodeKey *KvsNodeKey) error

	Get(nodeKey *KvsNodeKey, key string) ([]byte, error)
	Set(nodeKey *KvsNodeKey, key string, value []byte) error
	Patch(nodeKey *KvsNodeKey, key string, value []byte) error
	Delete(nodeKey *KvsNodeKey, key string) error
}
