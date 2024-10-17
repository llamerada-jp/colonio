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
package node_accessor

import (
	"testing"

	"github.com/llamerada-jp/colonio/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebRTCConfig(t *testing.T) {
	require.NotNil(t, defaultWebRTCConfigFactory)

	c1, err := defaultWebRTCConfigFactory([]config.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
		{
			URLs:       []string{"turn:turn.example.com"},
			Username:   "user",
			Credential: "pass",
		},
	})
	assert.NoError(t, err)

	c2, err := defaultWebRTCConfigFactory([]config.ICEServer{})
	assert.NoError(t, err)

	assert.NotEqual(t, c1.getConfigID(), c2.getConfigID())

	err = c1.destruct()
	assert.NoError(t, err)
	err = c2.destruct()
	assert.NoError(t, err)
}
