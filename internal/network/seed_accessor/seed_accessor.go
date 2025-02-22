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
package seed_accessor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"connectrpc.com/connect"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	service "github.com/llamerada-jp/colonio/api/colonio/v1alpha/v1alphaconnect"
	"github.com/llamerada-jp/colonio/internal/network/signal"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Handler interface {
	SeedRecvSignalOffer(*shared.NodeID, *signal.Offer)
	SeedRecvSignalAnswer(*shared.NodeID, *signal.Answer)
	SeedRecvSignalICE(*shared.NodeID, *signal.ICE)
}

type Config struct {
	Logger     *slog.Logger
	Handler    Handler
	URL        string
	HttpClient *http.Client // optional
}

type SeedAccessor struct {
	logger      *slog.Logger
	ctx         context.Context
	handler     Handler
	client      service.SeedServiceClient
	localNodeID *shared.NodeID

	// mtx is for waiting and isAlone
	mtx sync.RWMutex
	// isAlone is true if this node is the only online node of the seed
	isAlone bool
}

func NewSeedAccessor(config *Config) *SeedAccessor {
	if len(config.URL) == 0 {
		panic("URL should not be empty")
	}

	sa := &SeedAccessor{
		logger:  config.Logger,
		handler: config.Handler,
		isAlone: false,
	}

	httpClient := config.HttpClient
	if httpClient == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			panic(err)
		}

		httpClient = &http.Client{
			Transport: &http.Transport{},
			Jar:       jar,
		}
	}

	sa.client = service.NewSeedServiceClient(httpClient, config.URL)

	return sa
}

func (sa *SeedAccessor) Start(ctx context.Context) (*shared.NodeID, error) {
	sa.ctx = ctx

	res, err := sa.client.AssignNodeID(sa.ctx, &connect.Request[proto.AssignNodeIDRequest]{})
	if err != nil {
		return nil, fmt.Errorf("failed to assign node ID: %w", err)
	}

	sa.mtx.Lock()
	defer sa.mtx.Unlock()
	sa.localNodeID = shared.NewNodeIDFromProto(res.Msg.GetNodeId())
	sa.isAlone = res.Msg.GetIsAlone()

	go func() {
		// call round every second or finish when context is done

		for {
			select {
			case <-sa.ctx.Done():
				return
			default:
				err := sa.poll()
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					sa.logger.Warn("failed to poll", slog.String("error", err.Error()))
					time.Sleep(10 * time.Second)
				}
			}
		}
	}()

	return sa.localNodeID, nil
}

// IsAlone indicates whether the node is the only one online of the seed.
func (sa *SeedAccessor) IsAlone() bool {
	sa.mtx.RLock()
	defer sa.mtx.RUnlock()
	return sa.isAlone
}

func (sa *SeedAccessor) SendSignalOffer(dstNodeID *shared.NodeID, offer *signal.Offer) error {
	var offerType proto.SignalOfferType
	switch offer.OfferType {
	case signal.OfferTypeExplicit:
		offerType = proto.SignalOfferType_EXPLICIT
	case signal.OfferTypeNext:
		offerType = proto.SignalOfferType_NEXT
	default:
		return fmt.Errorf("unknown offer type: %d", offer.OfferType)
	}

	res, err := sa.client.SendSignal(sa.ctx, &connect.Request[proto.SendSignalRequest]{
		Msg: &proto.SendSignalRequest{
			Signal: &proto.Signal{
				DstNodeId: dstNodeID.Proto(),
				SrcNodeId: sa.localNodeID.Proto(),
				Content: &proto.Signal_Offer{
					Offer: &proto.SignalOffer{
						OfferId: offer.OfferID,
						Type:    offerType,
						Sdp:     offer.Sdp,
					},
				},
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to send signal offer: %w", err)
	}

	sa.mtx.Lock()
	sa.isAlone = res.Msg.GetIsAlone()
	sa.mtx.Unlock()
	return nil
}

func (sa *SeedAccessor) SendSignalAnswer(dstNodeID *shared.NodeID, answer *signal.Answer) error {
	res, err := sa.client.SendSignal(sa.ctx, &connect.Request[proto.SendSignalRequest]{
		Msg: &proto.SendSignalRequest{
			Signal: &proto.Signal{
				DstNodeId: dstNodeID.Proto(),
				SrcNodeId: sa.localNodeID.Proto(),
				Content: &proto.Signal_Answer{
					Answer: &proto.SignalAnswer{
						OfferId: answer.OfferID,
						Status:  uint32(answer.Status),
						Sdp:     answer.Sdp,
					},
				},
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to send signal answer: %w", err)
	}

	sa.mtx.Lock()
	sa.isAlone = res.Msg.GetIsAlone()
	sa.mtx.Unlock()
	return nil
}

func (sa *SeedAccessor) SendSignalICE(dstNodeID *shared.NodeID, ices *signal.ICE) error {
	res, err := sa.client.SendSignal(sa.ctx, &connect.Request[proto.SendSignalRequest]{
		Msg: &proto.SendSignalRequest{
			Signal: &proto.Signal{
				DstNodeId: dstNodeID.Proto(),
				SrcNodeId: sa.localNodeID.Proto(),
				Content: &proto.Signal_Ice{
					Ice: &proto.SignalICE{
						OfferId: ices.OfferID,
						Ices:    ices.Ices,
					},
				},
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to send signal ICE: %w", err)
	}

	sa.mtx.Lock()
	sa.isAlone = res.Msg.GetIsAlone()
	sa.mtx.Unlock()
	return nil
}

func (sa *SeedAccessor) poll() error {
	res, err := sa.client.PollSignal(sa.ctx, &connect.Request[proto.PollSignalRequest]{
		Msg: &proto.PollSignalRequest{},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("failed to poll signal: %w", err)
	}

	for res.Receive() {
		msg := res.Msg()

		for _, s := range msg.GetSignals() {
			from := shared.NewNodeIDFromProto(s.GetSrcNodeId())

			switch content := s.GetContent().(type) {
			case *proto.Signal_Offer:
				var offerType signal.OfferType
				switch content.Offer.Type {
				case proto.SignalOfferType_EXPLICIT:
					offerType = signal.OfferTypeExplicit
				case proto.SignalOfferType_NEXT:
					offerType = signal.OfferTypeNext
				default:
					sa.logger.Warn("unknown offer type")
					continue
				}
				sa.handler.SeedRecvSignalOffer(from, &signal.Offer{
					OfferID:   content.Offer.OfferId,
					OfferType: offerType,
					Sdp:       content.Offer.Sdp,
				})

			case *proto.Signal_Answer:
				sa.handler.SeedRecvSignalAnswer(from, &signal.Answer{
					OfferID: content.Answer.OfferId,
					Status:  signal.AnswerStatus(content.Answer.Status),
					Sdp:     content.Answer.Sdp,
				})

			case *proto.Signal_Ice:
				sa.handler.SeedRecvSignalICE(from, &signal.ICE{
					OfferID: content.Ice.OfferId,
					Ices:    content.Ice.Ices,
				})

			default:
				sa.logger.Warn("unknown signal type", slog.String("type", fmt.Sprintf("%T", content)))
			}
		}
	}

	return res.Err()
}
