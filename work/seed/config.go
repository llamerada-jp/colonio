/*
 * Copyright 2019-2021 Yuji Ito <llamerada.jp@gmail.com>
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
package seed

import "errors"

type ConfigNodeAccessor struct {
	BufferInterval *uint32 `json:"bufferInterval,omitempty"`
	HopCountMax    *uint32 `json:"hopCountMax,omitempty"`
	PacketSize     *uint32 `json:"packetSize,omitempty"`
}

type ConfigCoordSystem2D struct {
	Type *string `json:"type,omitempty"`
	// for sphere
	Radius *float64 `json:"radius,omitempty"`
	// for plane
	XMin *float64 `json:"xMin,omitempty"`
	XMax *float64 `json:"xMax,omitempty"`
	YMin *float64 `json:"yMin,omitempty"`
	YMax *float64 `json:"yMax,omitempty"`
}

type ConfigIceServer struct {
	Urls       []string `json:"urls,omitempty"`
	Username   *string  `json:"username,omitempty"`
	Credential *string  `json:"credential,omitempty"`
}

type ConfigRouting struct {
	ForceUpdateCount         *uint32  `json:"forceUpdateCount,omitempty"`
	SeedConnectInterval      *uint32  `json:"seedConnectInterval,omitempty"`
	SeedConnectRate          *uint32  `json:"seedConnectRate,omitempty"`
	SeedDisconnectThreathold *uint32  `json:"seedDisconnectThreathold,omitempty"`
	SeedInfoKeepThreathold   *uint32  `json:"seedInfoKeepThreathold,omitempty"`
	SeedInfoNidsCount        *uint32  `json:"seedInfoNidsCount,omitempty"`
	SeedNextPosition         *float64 `json:"seedNextPosition,omitempty"`
	UpdatePeriod             *uint32  `json:"updatePeriod,omitempty"`
}

type ConfigModule struct {
	Type    string `json:"type"`
	Channel uint32 `json:"channel"`

	RetryMax         *uint32 `json:"retryMax,omitempty"`         // for map
	RetryIntervalMin *uint32 `json:"retryIntervalMin,omitempty"` // for map
	RetryIntervalMax *uint32 `json:"retryIntervalMax,omitempty"` // for map

	CacheTime *uint32 `json:"cacheTime,omitempty"` // for pubsub_2d
}

type ConfigNode struct {
	Revision      float64                 `json:"revision"`
	NodeAccessor  *ConfigNodeAccessor     `json:"nodeAccessor,omitempty"`
	CoordSystem2d *ConfigCoordSystem2D    `json:"coordSystem2D,omitempty"`
	IceServers    []ConfigIceServer       `json:"iceServers,omitempty"`
	Routing       *ConfigRouting          `json:"routing,omitempty"`
	Modules       map[string]ConfigModule `json:"modules,omitempty"`
}

type Config struct {
	Revision     float64     `json:"revision,omitempty"`
	PingInterval int64       `json:"pingInterval"`
	Timeout      int64       `json:"timeout"`
	Node         *ConfigNode `json:"node,omitempty"`
}

func (c *Config) validate() error {
	if c.PingInterval <= 0 {
		return errors.New("Config value of `pingInverval` must be larger then 0.")
	}

	if c.Timeout <= 0 {
		return errors.New("Config value of `inverval` must be larger then 0.")
	}

	if c.Node == nil {
		return errors.New("Config value of `node` must be map type structure.")

	} else {
		if c.Node.IceServers == nil || len(c.Node.IceServers) == 0 {
			return errors.New("Config value of `node.iceServers` required")
		}

		if c.Node.Routing == nil {
			return errors.New("Config value of `node.routing` required")
		}
	}

	return nil
}
