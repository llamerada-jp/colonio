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

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterMarshaller(t *testing.T) {
	str := `{
		"revision":1.04,
		"sessionTimeout":"24h0m0s100ms",
		"pollingTimeout":"1ms"
	}`
	var decoded1 Cluster
	err := json.Unmarshal([]byte(str), &decoded1)
	require.NoError(t, err)
	assert.Equal(t, 1.04, decoded1.Revision)
	assert.Equal(t, 24*time.Hour+100*time.Millisecond, decoded1.SessionTimeout)
	assert.Equal(t, 1*time.Millisecond, decoded1.PollingTimeout)

	source := &Cluster{
		Revision:       1,
		SessionTimeout: 1 * time.Millisecond,
		PollingTimeout: 48*time.Hour + 1*time.Millisecond,
	}

	raw, err := json.Marshal(source)
	require.NoError(t, err)

	var decoded2 Cluster
	err = json.Unmarshal(raw, &decoded2)
	require.NoError(t, err)

	assert.Equal(t, source.Revision, decoded2.Revision)
	assert.Equal(t, source.SessionTimeout, decoded2.SessionTimeout)
	assert.Equal(t, source.PollingTimeout, decoded2.PollingTimeout)
}
