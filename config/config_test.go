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
		"pollingTimeout":"1ms",
		"keepaliveInterval":"10s",
		"bufferInterval":"2ms",
		"webRTCPacketBaseBytes":1024,
		"routingExchangeInterval": "10s",
		"seedConnectRate": 10,
		"seedReconnectDuration": "2m",
		"hopLimit": 10,
		"spread": {
			"cacheLifetime": "1m"
		}
	}`
	var decoded1 Cluster
	err := json.Unmarshal([]byte(str), &decoded1)
	require.NoError(t, err)
	assert.Equal(t, 1.04, decoded1.Revision)
	assert.Equal(t, 24*time.Hour+100*time.Millisecond, decoded1.SessionTimeout)
	assert.Equal(t, 1*time.Millisecond, decoded1.PollingTimeout)
	assert.Equal(t, 10*time.Second, decoded1.KeepaliveInterval)
	assert.Equal(t, 2*time.Millisecond, decoded1.BufferInterval)
	assert.Equal(t, 1024, decoded1.WebRTCPacketBaseBytes)
	assert.Equal(t, 10*time.Second, decoded1.RoutingExchangeInterval)
	assert.Equal(t, uint(10), decoded1.SeedConnectRate)
	assert.Equal(t, 2*time.Minute, decoded1.SeedReconnectDuration)
	assert.Equal(t, uint(10), decoded1.HopLimit)
	assert.Equal(t, 1*time.Minute, decoded1.Spread.CacheLifetime)

	source := &Cluster{
		Revision:                1,
		SessionTimeout:          1 * time.Millisecond,
		PollingTimeout:          48*time.Hour + 1*time.Millisecond,
		KeepaliveInterval:       1 * time.Second,
		BufferInterval:          55 * time.Millisecond,
		RoutingExchangeInterval: 30 * time.Second,
		Spread: &Spread{
			CacheLifetime: 2 * time.Minute,
		},
	}

	raw, err := json.Marshal(source)
	require.NoError(t, err)

	var decoded2 Cluster
	err = json.Unmarshal(raw, &decoded2)
	require.NoError(t, err)

	assert.Equal(t, source.Revision, decoded2.Revision)
	assert.Equal(t, source.SessionTimeout, decoded2.SessionTimeout)
	assert.Equal(t, source.PollingTimeout, decoded2.PollingTimeout)
	assert.Equal(t, source.BufferInterval, decoded2.BufferInterval)
	assert.Equal(t, 512*1024, decoded2.WebRTCPacketBaseBytes) // used default value
	assert.Equal(t, source.RoutingExchangeInterval, decoded2.RoutingExchangeInterval)
	assert.Equal(t, time.Minute, decoded2.SeedReconnectDuration) // default value
	assert.Equal(t, uint(64), decoded2.HopLimit)                 // default value
	assert.Equal(t, 2*time.Minute, decoded2.Spread.CacheLifetime)
}

func TestClusterValidate(t *testing.T) {
	validConfig := Cluster{
		Revision:       1,
		SessionTimeout: 10 * time.Minute,
		PollingTimeout: 5 * time.Minute,
		IceServers: []ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
		KeepaliveInterval:       5 * time.Minute,
		BufferInterval:          10 * time.Second,
		WebRTCPacketBaseBytes:   1024,
		RoutingExchangeInterval: 10 * time.Second,
		SeedConnectRate:         0,
	}

	tests := []struct {
		name    string
		c       func() *Cluster
		isValid bool
	}{
		{
			name: "valid",
			c: func() *Cluster {
				return &validConfig
			},
			isValid: true,
		},
		{
			name: "invalid sessionTimeout(<0)",
			c: func() *Cluster {
				c := validConfig
				c.SessionTimeout = 0
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid pollingTimeout(<0)",
			c: func() *Cluster {
				c := validConfig
				c.PollingTimeout = 0
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid pollingTimeout(>sessionTimeout)",
			c: func() *Cluster {
				c := validConfig
				c.PollingTimeout = 15 * time.Minute
				return &c
			},
			isValid: false,
		},
		{
			name: "iceServers are not set",
			c: func() *Cluster {
				c := validConfig
				c.IceServers = nil
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid iceServer(empty URL)",
			c: func() *Cluster {
				c := validConfig
				c.IceServers = []ICEServer{{
					Username: "username",
				}}
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid keepaliveInterval",
			c: func() *Cluster {
				c := validConfig
				c.KeepaliveInterval = 0
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid keepaliveInterval(>sessionTimeout)",
			c: func() *Cluster {
				c := validConfig
				c.KeepaliveInterval = 15 * time.Minute
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid bufferInterval",
			c: func() *Cluster {
				c := validConfig
				c.BufferInterval = -1
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid webRTCPacketBaseBytes",
			c: func() *Cluster {
				c := validConfig
				c.WebRTCPacketBaseBytes = -1
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid routingExchangeInterval",
			c: func() *Cluster {
				c := validConfig
				c.RoutingExchangeInterval = 0
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid geometry setting",
			c: func() *Cluster {
				c := validConfig
				c.Spread = &Spread{}
				return &c
			},
			isValid: false,
		},
		{
			name: "invalid geometry setting (empty)",
			c: func() *Cluster {
				c := validConfig
				c.Geometry = &Geometry{
					Plane:  &GeometryPlane{},
					Sphere: &GeometrySphere{},
				}
				return &c
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c().Validate()
			if tt.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestGeometryValidate(t *testing.T) {
	tests := []struct {
		name    string
		g       *Geometry
		isValid bool
	}{
		{
			name: "valid(plane)",
			g: &Geometry{
				Plane: &GeometryPlane{
					XMin: -10,
					XMax: 10,
					YMin: -10,
					YMax: 10,
				},
			},
			isValid: true,
		},
		{
			name: "valid(sphere)",
			g: &Geometry{
				Sphere: &GeometrySphere{
					Radius: 10,
				},
			},
			isValid: true,
		},
		{
			name: "has both plane and sphere",
			g: &Geometry{
				Plane: &GeometryPlane{
					XMin: -10,
					XMax: 10,
					YMin: -10,
					YMax: 10,
				},
				Sphere: &GeometrySphere{
					Radius: 10,
				},
			},
			isValid: false,
		},
		{
			name: "invalid plane(xMin >= xMax)",
			g: &Geometry{
				Plane: &GeometryPlane{
					XMin: 10,
					XMax: -10,
					YMin: -10,
					YMax: 10,
				},
			},
			isValid: false,
		},
		{
			name: "invalid plane(yMin >= yMax)",
			g: &Geometry{
				Plane: &GeometryPlane{
					XMin: -10,
					XMax: 10,
					YMin: 10,
					YMax: -10,
				},
			},
			isValid: false,
		},
		{
			name: "invalid sphere(radius <= 0)",
			g: &Geometry{
				Sphere: &GeometrySphere{
					Radius: 0,
				},
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.g.validate()
			if tt.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
