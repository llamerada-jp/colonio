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
	"time"

	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/geometry"
	"github.com/llamerada-jp/colonio/internal/messaging"
	"github.com/llamerada-jp/colonio/internal/network"
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
	Connect(url string, token []byte) error
	Disconnect()
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

type Config struct {
	Ctx    context.Context
	Logger *slog.Logger

	// Allow insecure server connections. You must not use this option in production. Only for testing.
	Insecure bool
	// Interval between retries when a network error occurs.
	SeedTripInterval time.Duration
}
type ConfigSetter func(*Config)

func WithContext(ctx context.Context) ConfigSetter {
	return func(c *Config) {
		c.Ctx = ctx
	}
}

func WithLogger(logger *slog.Logger) ConfigSetter {
	return func(c *Config) {
		c.Logger = logger
	}
}

func WithInsecure() ConfigSetter {
	return func(c *Config) {
		c.Insecure = true
	}
}

func WithSeedTripInterval(interval time.Duration) ConfigSetter {
	return func(c *Config) {
		c.SeedTripInterval = interval
	}
}

type colonioImpl struct {
	config      *Config
	ctx         context.Context
	cancel      context.CancelFunc
	localNodeID *shared.NodeID
	network     *network.Network
	messaging   *messaging.Messaging
	spread      *spread.Spread
}

func NewColonio(setters ...ConfigSetter) Colonio {
	config := &Config{
		Ctx:              context.Background(),
		Logger:           slog.Default(),
		SeedTripInterval: 5 * time.Minute,
	}
	for _, setter := range setters {
		setter(config)
	}

	ctx, cancel := context.WithCancel(config.Ctx)

	impl := &colonioImpl{
		config:      config,
		ctx:         ctx,
		cancel:      cancel,
		localNodeID: shared.NewRandomNodeID(),
		messaging:   messaging.NewMessaging(config.Logger),
	}

	networkConfig := &network.Config{
		Ctx:              ctx,
		Logger:           config.Logger,
		LocalNodeID:      impl.localNodeID,
		Handler:          impl,
		Insecure:         config.Insecure,
		SeedTripInterval: config.SeedTripInterval,
	}

	impl.network = network.NewNetwork(networkConfig)
	impl.spread = spread.NewSpread(ctx, config.Logger, impl)

	return impl
}

func (c *colonioImpl) Connect(url string, token []byte) error {
	return c.network.Connect(url, token)
}

func (c *colonioImpl) Disconnect() {
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
	dstNodeID := shared.NewNodeIDFromString(dst)
	if dstNodeID == nil {
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

func (c *colonioImpl) NetworkRecvConfig(clusterConfig *config.Cluster) {
	// call GetTransferer asynchronously to avoid deadlock on Network module.
	go func() {
		c.messaging.ApplyConfig(c.network.GetTransferer())

		if clusterConfig.Geometry != nil && clusterConfig.Spread != nil {
			c.spread.ApplyConfig(c.network.GetTransferer(), clusterConfig.Spread, c.localNodeID, c.network.GetCoordinateSystem())
		}
	}()
}

func (c *colonioImpl) NetworkUpdateNextNodePosition(positions map[shared.NodeID]*geometry.Coordinate) {
	c.spread.UpdateNextNodePosition(positions)
}

func (c *colonioImpl) SpreadGetRelayNodeID(position *geometry.Coordinate) *shared.NodeID {
	return c.network.GetNextStep2D(position)
}
