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
	"errors"
	"fmt"
	"time"
)

type NodeAccessor struct {
	BufferInterval *uint32 `json:"bufferInterval,omitempty"`
	HopCountMax    *uint32 `json:"hopCountMax,omitempty"`
	PacketSize     *uint32 `json:"packetSize,omitempty"`
}

type CoordSystem2D struct {
	Type *string `json:"type,omitempty"`
	// for sphere
	Radius *float64 `json:"radius,omitempty"`
	// for plane
	XMin *float64 `json:"xMin,omitempty"`
	XMax *float64 `json:"xMax,omitempty"`
	YMin *float64 `json:"yMin,omitempty"`
	YMax *float64 `json:"yMax,omitempty"`
}

type IceServer struct {
	Urls       []string `json:"urls,omitempty"`
	Username   *string  `json:"username,omitempty"`
	Credential *string  `json:"credential,omitempty"`
}

type Routing struct {
	ForceUpdateCount        *uint32  `json:"forceUpdateCount,omitempty"`
	SeedConnectInterval     *uint32  `json:"seedConnectInterval,omitempty"`
	SeedConnectRate         *uint32  `json:"seedConnectRate,omitempty"`
	SeedDisconnectThreshold *uint32  `json:"seedDisconnectThreshold,omitempty"`
	SeedInfoKeepThreshold   *uint32  `json:"seedInfoKeepThreshold,omitempty"`
	SeedInfoNidsCount       *uint32  `json:"seedInfoNidsCount,omitempty"`
	SeedNextPosition        *float64 `json:"seedNextPosition,omitempty"`
	UpdatePeriod            *uint32  `json:"updatePeriod,omitempty"`
}

type Kvs struct {
	RetryMax         *uint32 `json:"retryMax,omitempty"`
	RetryIntervalMin *uint32 `json:"retryIntervalMin,omitempty"`
	RetryIntervalMax *uint32 `json:"retryIntervalMax,omitempty"`
}

type Spread struct {
	CacheTime *uint32 `json:"cacheTime,omitempty"`
}

type Cluster struct {
	// Revision is to describe the version of the configuration.
	// Node should be restarted when the revision between the seed and the node is different.
	// Example: 20240101.1
	Revision float64 `json:"revision"`

	// SessionTimeout is used to determine the timeout of the session.
	// The seed & nodes will disconnect the node if the node does not send any packets within the timeout.
	SessionTimeout time.Duration `json:"-"`

	// PollingTimeout is used to determine the timeout of the polling.
	// `polling` request for the seed will be returned after the timeout if there is no packet to relay.
	// The value should be less than `sessionTimeout`.
	PollingTimeout time.Duration  `json:"-"`
	NodeAccessor   *NodeAccessor  `json:"nodeAccessor,omitempty"`
	CoordSystem2d  *CoordSystem2D `json:"coordSystem2D,omitempty"`
	IceServers     []IceServer    `json:"iceServers,omitempty"`
	Routing        Routing        `json:"routing"`
	Kvs            *Kvs           `json:"kvs,omitempty"`
	Spread         *Spread        `json:"spread,omitempty"`
}

func (c *Cluster) MarshalJSON() ([]byte, error) {
	type Alias Cluster

	return json.Marshal(&struct {
		*Alias
		SessionTimeout string `json:"sessionTimeout"`
		PollingTimeout string `json:"pollingTimeout"`
	}{
		Alias:          (*Alias)(c),
		SessionTimeout: c.SessionTimeout.String(),
		PollingTimeout: c.PollingTimeout.String(),
	})
}

func (c *Cluster) UnmarshalJSON(b []byte) error {
	type Alias Cluster

	aux := &struct {
		*Alias
		SessionTimeout string `json:"sessionTimeout"`
		PollingTimeout string `json:"pollingTimeout"`
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}

	var err error
	c.SessionTimeout, err = time.ParseDuration(aux.SessionTimeout)
	if err != nil {
		return fmt.Errorf("failed to parse `sessionTimeout`: %w", err)
	}
	c.PollingTimeout, err = time.ParseDuration(aux.PollingTimeout)
	if err != nil {
		return fmt.Errorf("failed to parse `pollingTimeout`: %w", err)
	}

	return nil
}

func (c *Cluster) Validate() error {
	if c.IceServers == nil || len(c.IceServers) == 0 {
		return errors.New("config value of `node.iceServers` required")
	}

	if c.CoordSystem2d == nil && c.Spread != nil {
		return errors.New("`spread` module require `coordSystem2D` configurations")
	}

	return nil
}
