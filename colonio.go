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
package colonio

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/messaging"
	"github.com/llamerada-jp/colonio/internal/network"
	"github.com/llamerada-jp/colonio/internal/network/node_accessor"
	"github.com/llamerada-jp/colonio/internal/observation"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/internal/spread"
)

type MessagingOptions messaging.Options
type MessagingOptionSetter func(*MessagingOptions)
type MessagingResponseWriter messaging.ResponseWriter
type MessagingRequest struct {
	SourceNodeID string
	Message      []byte
	Options      *MessagingOptions
}

type SpreadOptions spread.Options
type SpreadOptionSetter func(*SpreadOptions)
type SpreadRequest struct {
	SourceNodeID string
	Message      []byte
	Options      *SpreadOptions
}

type Colonio interface {
	Start(ctx context.Context) error
	Stop()
	IsOnline() bool
	GetLocalNodeID() string
	UpdateLocalPosition(x, y float64) error
	// messaging
	MessagingPost(dst, name string, val []byte, setters ...MessagingOptionSetter) ([]byte, error)
	MessagingSetHandler(name string, handler func(*MessagingRequest, MessagingResponseWriter))
	MessagingUnsetHandler(name string)
	// spread
	SpreadPost(x, y, r float64, name string, val []byte, setters ...SpreadOptionSetter) error
	SpreadSetHandler(name string, handler func(*SpreadRequest))
	SpreadUnsetHandler(name string)
}

// ObservationHandler is an interface for observing the internal status of colonio.
// These interfaces are for debugging and simulation and are not intended for normal use.
// They are not guaranteed to work or be compatible.
type ObservationHandlers observation.Handlers

type Config struct {
	Logger              *slog.Logger
	ObservationHandlers *ObservationHandlers
	HttpClient          *http.Client
	SeedURL             string
	ICEServers          []*config.ICEServer
	CoordinateSystem    geometry.CoordinateSystem

	// RoutingExchangeInterval is interval at which packets exchanging routing information are sent.
	// However, if necessary, packets may be sent at intervals shorter than the setting.
	RoutingExchangeInterval time.Duration

	// PacketHopLimit is the maximum number of hops that a packet can be relayed.
	// If you set 0, the default value of 64 will be set.
	PacketHopLimit uint

	// CacheLifetime is the lifetime of the cache. The spread algorithm is
	// so simple that the same packet may be received multiple times;
	// if the same packet is received within the cache lifetime, it can be suppressed
	// for reprocessing. This costs more memory.
	SpreadCacheLifetime time.Duration

	// Before sending a large payload, you can check whether a cache exists
	// at the other node using knock packet. For small packets, it is more efficient
	// to send the packet without knocking.
	// If you set 0, disable the use of knock packets.
	SpreadSizeToUseKnock uint
}
type ConfigSetter func(*Config)

func WithLogger(logger *slog.Logger) ConfigSetter {
	return func(c *Config) {
		c.Logger = logger
	}
}

func WithObservation(handlers *ObservationHandlers) ConfigSetter {
	return func(c *Config) {
		c.ObservationHandlers = handlers
	}
}

func WithHttpClient(client *http.Client) ConfigSetter {
	return func(c *Config) {
		c.HttpClient = client
	}
}

func WithSeedURL(url string) ConfigSetter {
	return func(c *Config) {
		c.SeedURL = url
	}
}

// IceServers is a list of ICE servers.
// In Colonio, multiple ICE servers can be used to establish a WebRTC connection.
// Each entry contains the URLs of the ICE server, the username, and the credential.
func WithICEServers(servers []*config.ICEServer) ConfigSetter {
	return func(c *Config) {
		c.ICEServers = servers
	}
}

// Plane is a configuration for the plane geometry.
// If this configuration is set, the 2D position based network will be setup as a plane space.
func WithPlaneGeometry(xMin, xMax, yMin, yMax float64) ConfigSetter {
	return func(c *Config) {
		c.CoordinateSystem = geometry.NewPlaneCoordinateSystem(xMin, xMax, yMin, yMax)
	}
}

// Sphere is a configuration for the sphere geometry.
// If this configuration is set, the 2D position based network will be setup as a sphere space.
func WithSphereGeometry(radius float64) ConfigSetter {
	return func(c *Config) {
		c.CoordinateSystem = geometry.NewSphereCoordinateSystem(radius)
	}
}

type colonioImpl struct {
	logger      *slog.Logger
	ctx         context.Context
	cancel      context.CancelFunc
	localNodeID *shared.NodeID
	network     *network.Network
	messaging   *messaging.Messaging
	spread      *spread.Spread
}

func NewColonio(setters ...ConfigSetter) (Colonio, error) {
	config := &Config{
		Logger:                  slog.Default(),
		ObservationHandlers:     &ObservationHandlers{},
		RoutingExchangeInterval: 1 * time.Minute,
		PacketHopLimit:          64,

		SpreadCacheLifetime:  1 * time.Minute,
		SpreadSizeToUseKnock: 4096,
	}
	for _, setter := range setters {
		setter(config)
	}

	if len(config.SeedURL) == 0 {
		return nil, fmt.Errorf("seed url should be set")
	}

	if len(config.ICEServers) == 0 {
		return nil, fmt.Errorf("ICE servers should be set")
	}

	if config.CoordinateSystem == nil {
		return nil, fmt.Errorf("coordinate system should be set")
	}

	if config.HttpClient == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, err
		}

		config.HttpClient = &http.Client{
			Jar: jar,
		}
	}

	impl := &colonioImpl{
		logger: config.Logger,
	}

	// create network module
	net, err := network.NewNetwork(&network.Config{
		Logger:           config.Logger,
		Handler:          impl,
		Observation:      (*observation.Handlers)(config.ObservationHandlers),
		CoordinateSystem: config.CoordinateSystem,
		HttpClient:       config.HttpClient,
		SeedURL:          config.SeedURL,
		NLC: &node_accessor.NodeLinkConfig{
			ICEServers: config.ICEServers,

			// SessionTimeout is used to determine the timeout of the WebRTC session between nodes.
			SessionTimeout: 5 * time.Minute,

			// KeepaliveInterval is the interval to send a ping packet to tell living the node for each nodes.
			// Keepalive packet is be tried to send  when no packet with content has been sent.
			// The value should be less than `sessionTimeout`.
			KeepaliveInterval: 1 * time.Minute,

			// BufferInterval is maximum interval for buffering packets between nodes.
			// If packets exceeding WebRTCPacketBaseBytes are stored in the buffer even if it is less than interval,
			// the packets will be flush.
			// If you set 0, disables packet buffering and tries to transport the packet as immediately as possible.
			BufferInterval: 10 * time.Millisecond,

			// PacketBaseBytes is a reference value for the packet size to be sent in WebRTC communication,
			// since WebRTC data channel may fail to send too large packets.
			// If you set 0, 512KiB will be set as the default value.
			// For simplification of the internal implementation, the packet size actually sent may be
			// larger than this value. Therefore, please set this value with a margin.
			// This value is provided as a fallback, although it may not be necessary depending
			// on the WebRTC library implementation. In such a case, you can disable this value
			// the pseudo setting by setting a very large value.
			PacketBaseBytes: 4096,
		},
		RoutingExchangeInterval: config.RoutingExchangeInterval,
		PacketHopLimit:          config.PacketHopLimit,
	})
	if err != nil {
		return nil, err
	}
	impl.network = net

	impl.messaging = messaging.NewMessaging(&messaging.Config{
		Logger:     config.Logger,
		Transferer: net.GetTransferer(),
	})

	impl.spread = spread.NewSpread(&spread.Config{
		Logger:           config.Logger,
		Handler:          impl,
		Transferer:       net.GetTransferer(),
		CoordinateSystem: config.CoordinateSystem,
		CacheLifetime:    config.SpreadCacheLifetime,
		SizeToUseKnock:   config.SpreadSizeToUseKnock,
	})

	return impl, nil
}

func (c *colonioImpl) Start(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	var err error
	c.localNodeID, err = c.network.Start(c.ctx)
	if err != nil {
		return err
	}

	c.spread.Start(c.ctx, c.localNodeID)

	return nil
}

func (c *colonioImpl) Stop() {
	c.cancel()
}

func (c *colonioImpl) IsOnline() bool {
	return c.network.IsOnline()
}

func (c *colonioImpl) GetLocalNodeID() string {
	return c.localNodeID.String()
}

func (c *colonioImpl) UpdateLocalPosition(x, y float64) error {
	position := geometry.NewCoordinate(x, y)
	c.spread.UpdateLocalPosition(position)
	return c.network.UpdateLocalPosition(position)
}

// messaging

func MessagingWithAcceptNearby() MessagingOptionSetter {
	return func(o *MessagingOptions) {
		o.AcceptNearby = true
	}
}

func MessagingWithIgnoreResponse() MessagingOptionSetter {
	return func(o *MessagingOptions) {
		o.IgnoreResponse = true
	}
}

func (c *colonioImpl) MessagingPost(dst, name string, val []byte, setters ...MessagingOptionSetter) ([]byte, error) {
	dstNodeID, err := shared.NewNodeIDFromString(dst)
	if err != nil {
		return nil, fmt.Errorf("invalid node id")
	}

	opt := &MessagingOptions{}
	for _, setter := range setters {
		setter(opt)
	}

	return c.messaging.Post(dstNodeID, name, val, (*messaging.Options)(opt))
}

func (c *colonioImpl) MessagingSetHandler(name string, handler func(*MessagingRequest, MessagingResponseWriter)) {
	c.messaging.SetHandler(name, func(req *messaging.Request, res messaging.ResponseWriter) {
		handler(&MessagingRequest{
			SourceNodeID: req.SourceNodeID.String(),
			Message:      req.Message,
			Options:      (*MessagingOptions)(req.Options),
		}, MessagingResponseWriter(res))
	})
}

func (c *colonioImpl) MessagingUnsetHandler(name string) {
	c.messaging.UnsetHandler(name)
}

// spread

func SpreadWithSomeoneMustExists() SpreadOptionSetter {
	return func(o *SpreadOptions) {
		o.SomeoneMustExists = true
	}
}

func (c *colonioImpl) SpreadPost(x, y, r float64, name string, val []byte, setters ...SpreadOptionSetter) error {
	opt := &SpreadOptions{}
	for _, setter := range setters {
		setter(opt)
	}

	return c.spread.Post(geometry.NewCoordinate(x, y), r, name, val, (*spread.Options)(opt))
}

func (c *colonioImpl) SpreadSetHandler(name string, handler func(*SpreadRequest)) {
	c.spread.SetHandler(name, func(r *spread.Request) {
		handler(&SpreadRequest{
			SourceNodeID: r.SourceNodeID.String(),
			Message:      r.Message,
			Options:      (*SpreadOptions)(r.Options),
		})
	})
}

func (c *colonioImpl) SpreadUnsetHandler(name string) {
	c.spread.UnsetHandler(name)
}

func (c *colonioImpl) NetworkUpdateNextNodePosition(positions map[shared.NodeID]*geometry.Coordinate) {
	c.spread.UpdateNextNodePosition(positions)
}

func (c *colonioImpl) SpreadGetRelayNodeID(position *geometry.Coordinate) *shared.NodeID {
	return c.network.GetNextStep2D(position)
}
