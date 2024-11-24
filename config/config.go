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

	// The interval at which packets exchanging routing information are sent.
	// However, if necessary, packets may be sent at intervals shorter than the setting.
	RoutingExchangeInterval time.Duration `json:"-"`

	// The ratio of nodes that connect to the seed. For example, if 3 is set, the node will
	// try to connect to the seed if no node is found in the neighborhood of 3 that connects to the seed.
	// If set 0, all nodes will try to keep a connection to the seed.
	SeedConnectRate uint `json:"seedConnectRate,omitempty"`

	// Interval to review connections to SEED. If a node has been connected to seed for more than
	// a set amount of time and there is a node in the neighborhood that is already connected to seed,
	// the connection to seed may be disconnected. This value will be unused when `seedConnectRate` is 0.
	// If you set 0, 1min will be set as the default value.
	SeedReconnectDuration time.Duration `json:"-"`

	// HopLimit is the maximum number of hops that a packet can be relayed.
	// If you set 0, the default value of 64 will be set.
	HopLimit uint `json:"hopLimit,omitempty"`

	// Geometry is a configuration for the geometry of 2D position based network.
	// Some APIs may not be available and rise error if this configuration is not set.
	Geometry *Geometry `json:"geometry,omitempty"`

	Kvs *Kvs `json:"kvs,omitempty"`

	// Spread is a configuration for the spread module.
	Spread *Spread `json:"spread,omitempty"`
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

// Geometry is a configuration for the geometry of 2D position based network.
// Each setting is exclusive and only one of the geometry configuration is selected.
type Geometry struct {
	// Plane is a configuration for the plane geometry.
	// If this configuration is set, the 2D position based network will be setup as a plane space.
	Plane *GeometryPlane `json:"plane,omitempty"`
	// Sphere is a configuration for the sphere geometry.
	Sphere *GeometrySphere `json:"sphere,omitempty"`
}

type GeometryPlane struct {
	XMin float64 `json:"xMin"`
	XMax float64 `json:"xMax"`
	YMin float64 `json:"yMin"`
	YMax float64 `json:"yMax"`
}

type GeometrySphere struct {
	Radius float64 `json:"radius"`
}

type Kvs struct {
	RetryMax         *uint32 `json:"retryMax,omitempty"`
	RetryIntervalMin *uint32 `json:"retryIntervalMin,omitempty"`
	RetryIntervalMax *uint32 `json:"retryIntervalMax,omitempty"`
}

type Spread struct {
	// CacheLifetime is the lifetime of the cache. The spread algorithm is
	// so simple that the same packet may be received multiple times;
	// if the same packet is received within the cache lifetime, it can be suppressed
	// for reprocessing. This costs more memory.
	CacheLifetime time.Duration `json:"-"`

	// Before sending a large payload, you can check whether a cache exists
	// at the other node using knock packet. For small packets, it is more efficient
	// to send the packet without knocking.
	// If you set 0, disable the use of knock packets.
	SizeToUseKnock uint `json:"sizeToUseKnock,omitempty"`
}

func (c *Cluster) MarshalJSON() ([]byte, error) {
	type Alias Cluster

	return json.Marshal(&struct {
		*Alias
		SessionTimeout          string `json:"sessionTimeout"`
		PollingTimeout          string `json:"pollingTimeout"`
		KeepaliveInterval       string `json:"keepaliveInterval"`
		BufferInterval          string `json:"bufferInterval"`
		RoutingExchangeInterval string `json:"routingExchangeInterval"`
		SeedReconnectDuration   string `json:"seedReconnectDuration"`
	}{
		Alias:                   (*Alias)(c),
		SessionTimeout:          c.SessionTimeout.String(),
		PollingTimeout:          c.PollingTimeout.String(),
		KeepaliveInterval:       c.KeepaliveInterval.String(),
		BufferInterval:          c.BufferInterval.String(),
		RoutingExchangeInterval: c.RoutingExchangeInterval.String(),
		SeedReconnectDuration:   c.SeedReconnectDuration.String(),
	})
}

func (c *Cluster) UnmarshalJSON(b []byte) error {
	type Alias Cluster

	aux := &struct {
		*Alias
		SessionTimeout          string `json:"sessionTimeout"`
		PollingTimeout          string `json:"pollingTimeout"`
		KeepaliveInterval       string `json:"keepaliveInterval"`
		BufferInterval          string `json:"bufferInterval"`
		RoutingExchangeInterval string `json:"routingExchangeInterval"`
		SeedReconnectDuration   string `json:"seedReconnectDuration"`
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
	// RoutingExchangeInterval
	c.RoutingExchangeInterval, err = time.ParseDuration(aux.RoutingExchangeInterval)
	if err != nil {
		return fmt.Errorf("failed to parse `routingExchangeInterval`: %w", err)
	}
	// SeedReconnectDuration
	if len(aux.SeedReconnectDuration) == 0 {
		c.SeedReconnectDuration = 1 * time.Minute
	} else {
		c.SeedReconnectDuration, err = time.ParseDuration(aux.SeedReconnectDuration)
		if err != nil {
			return fmt.Errorf("failed to parse `seedReconnectDuration`: %w", err)
		}
		if c.SeedReconnectDuration == 0 {
			c.SeedReconnectDuration = 1 * time.Minute
		}
	}

	// HopLimit
	if c.HopLimit == 0 {
		c.HopLimit = 64
	}

	return nil
}

func (c *Cluster) Validate() error {
	if c.SessionTimeout <= 0 {
		return errors.New("config value of `sessionTimeout` must be greater than 0")
	}

	if c.PollingTimeout <= 0 || c.SessionTimeout <= c.PollingTimeout {
		return errors.New("config value of `pollingTimeout` must be within range of 0 to `sessionTimeout`")
	}

	for _, iceServer := range c.IceServers {
		if err := iceServer.validate(); err != nil {
			return err
		}
	}

	if c.KeepaliveInterval <= 0 || c.SessionTimeout <= c.KeepaliveInterval {
		return errors.New("config value of `keepaliveInterval` must be within range of 0 to `sessionTimeout`")
	}

	if c.BufferInterval < 0 {
		return errors.New("config value of `bufferInterval` must be greater than or equal to 0")
	}

	if c.WebRTCPacketBaseBytes < 0 {
		return errors.New("config value of `webrtcPacketBaseBytes` must be greater than or equal to 0")
	}

	if c.RoutingExchangeInterval <= 0 {
		return errors.New("config value of `routingExchangeInterval` must be greater than 0")
	}

	if !c.Geometry.isSet() && c.Spread != nil {
		return errors.New("`spread` module require `geometry` configurations")
	}

	if c.Geometry.isSet() {
		if err := c.Geometry.validate(); err != nil {
			return err
		}
	}

	return nil
}

func (s *ICEServer) validate() error {
	if len(s.URLs) == 0 {
		return errors.New("config value of `urls` required in `iceServers`")
	}

	for _, url := range s.URLs {
		if url == "" {
			return errors.New("config value of `urls` must not be empty in `iceServers`")
		}
	}

	return nil
}

func (g *Geometry) isSet() bool {
	if g == nil {
		return false
	}

	if g.Plane == nil && g.Sphere == nil {
		return false
	}

	return true
}

func (g *Geometry) validate() error {
	if g.Plane != nil && g.Sphere != nil {
		return errors.New("config value of `geometry` must be either `plane` or `sphere`")
	}

	if g.Plane != nil {
		return g.Plane.validate()
	}

	return g.Sphere.validate()
}

func (gp *GeometryPlane) validate() error {
	if gp.XMin >= gp.XMax {
		return errors.New("config value of `xMin` must be less than `xMax` in `geometry.plane`")
	}
	if gp.YMin >= gp.YMax {
		return errors.New("config value of `yMin` must be less than `yMax` in `geometry.plane`")
	}

	return nil
}

func (gs *GeometrySphere) validate() error {
	if gs.Radius <= 0 {
		return errors.New("config value of `radius` must be greater than 0 in `geometry.sphere`")
	}

	return nil
}

func (s *Spread) MarshalJSON() ([]byte, error) {
	type Alias Spread

	return json.Marshal(&struct {
		*Alias
		CacheLifetime string `json:"cacheLifetime"`
	}{
		Alias:         (*Alias)(s),
		CacheLifetime: s.CacheLifetime.String(),
	})
}

func (s *Spread) UnmarshalJSON(b []byte) error {
	type Alias Spread

	aux := &struct {
		*Alias
		CacheLifetime string `json:"cacheLifetime"`
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}

	var err error
	// CacheLifetime
	s.CacheLifetime, err = time.ParseDuration(aux.CacheLifetime)
	if err != nil {
		return fmt.Errorf("failed to parse `cacheLifetime`: %w", err)
	}

	return nil
}
