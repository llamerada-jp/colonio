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

// ICEServer is a configuration for ICE server. It is used to establish a WebRTC connection.
type ICEServer struct {
	// URLs is a list of URLs of the ICE server.
	URLs []string `json:"urls,omitempty"`
	// Username is a username for the ICE server.
	Username string `json:"username,omitempty"`
	// Credential is a credential for the ICE server.
	Credential string `json:"credential,omitempty"`
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
	// The seed & nodes will disconnect from node that does not send any packets within the timeout.
	SessionTimeout time.Duration `json:"-"`

	// PollingTimeout is used to determine the timeout of the polling.
	// `polling` request for the seed will be returned after the timeout if there is no packet to relay.
	// The value should be less than `sessionTimeout`.
	PollingTimeout time.Duration `json:"-"`

	// IceServers is a list of ICE servers.
	// In Colonio, multiple ICE servers can be used to establish a WebRTC connection.
	// Each entry contains the URLs of the ICE server, the username, and the credential.
	IceServers []ICEServer `json:"iceServers,omitempty"`

	// KeepaliveInterval is the interval to send a ping packet to tell living the node for each nodes.
	// Keepalive packet is be tried to send  when no packet with content has been sent.
	// The value should be less than `sessionTimeout`.
	KeepaliveInterval time.Duration `json:"-"`

	// BufferInterval is maximum interval for buffering packets between nodes.
	// If packets exceeding WebRTCPacketBaseBytes are stored in the buffer even if it is less than interval,
	// the packets will be flush.
	// If you set 0, disables packet buffering and tries to transport the packet as immediately as possible.
	BufferInterval time.Duration `json:"-"`

	// WebRTCPacketSizeBase is a reference value for the packet size to be sent in WebRTC communication,
	// since WebRTC data channel may fail to send too large packets.
	// If you set 0, 512KiB will be set as the default value.
	// For simplification of the internal implementation, the packet size actually sent may be
	// larger than this value. Therefore, please set this value with a margin.
	// This value is provided as a fallback, although it may not be necessary depending
	// on the WebRTC library implementation. In such a case, you can disable this value
	// the pseudo setting by setting a very large value.
	WebRTCPacketBaseBytes int `json:"webrtcPacketBaseBytes,omitempty"`

	HopCountMax *uint32 `json:"hopCountMax,omitempty"`

	CoordSystem2d *CoordSystem2D `json:"coordSystem2D,omitempty"`
	Routing       Routing        `json:"routing"`
	Kvs           *Kvs           `json:"kvs,omitempty"`
	Spread        *Spread        `json:"spread,omitempty"`
}

func (c *Cluster) MarshalJSON() ([]byte, error) {
	type Alias Cluster

	return json.Marshal(&struct {
		*Alias
		SessionTimeout    string `json:"sessionTimeout"`
		PollingTimeout    string `json:"pollingTimeout"`
		KeepaliveInterval string `json:"keepaliveInterval"`
		BufferInterval    string `json:"bufferInterval"`
	}{
		Alias:             (*Alias)(c),
		SessionTimeout:    c.SessionTimeout.String(),
		PollingTimeout:    c.PollingTimeout.String(),
		KeepaliveInterval: c.KeepaliveInterval.String(),
		BufferInterval:    c.BufferInterval.String(),
	})
}

func (c *Cluster) UnmarshalJSON(b []byte) error {
	type Alias Cluster

	aux := &struct {
		*Alias
		SessionTimeout    string `json:"sessionTimeout"`
		PollingTimeout    string `json:"pollingTimeout"`
		KeepaliveInterval string `json:"keepaliveInterval"`
		BufferInterval    string `json:"bufferInterval"`
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}

	var err error
	// SessionTimeout
	c.SessionTimeout, err = time.ParseDuration(aux.SessionTimeout)
	if err != nil {
		return fmt.Errorf("failed to parse `sessionTimeout`: %w", err)
	}
	// PollingTimeout
	c.PollingTimeout, err = time.ParseDuration(aux.PollingTimeout)
	if err != nil {
		return fmt.Errorf("failed to parse `pollingTimeout`: %w", err)
	}
	// KeepaliveInterval
	c.KeepaliveInterval, err = time.ParseDuration(aux.KeepaliveInterval)
	if err != nil {
		return fmt.Errorf("failed to parse `keepaliveInterval`: %w", err)
	}
	// BufferInterval
	c.BufferInterval, err = time.ParseDuration(aux.BufferInterval)
	if err != nil {
		return fmt.Errorf("failed to parse `bufferInterval`: %w", err)
	}
	// WebRTCPacketBaseBytes
	if c.WebRTCPacketBaseBytes == 0 {
		c.WebRTCPacketBaseBytes = 512 * 1024
	}

	return nil
}

func (c *Cluster) Validate() error {
	if c.IceServers == nil || len(c.IceServers) == 0 {
		return errors.New("config value of `iceServers` required")
	}

	if c.WebRTCPacketBaseBytes < 0 {
		return errors.New("config value of `webrtcPacketBaseBytes` must be greater than or equal to 0")
	}

	if c.CoordSystem2d == nil && c.Spread != nil {
		return errors.New("`spread` module require `coordSystem2D` configurations")
	}

	return nil
}
