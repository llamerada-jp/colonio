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
package activation

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	service "github.com/llamerada-jp/colonio/api/colonio/v1alpha/v1alphaconnect"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

type OutboundPort interface {
	send(ctx context.Context, state kvsTypes.ActivationState) (kvsTypes.ActivationState, error)
}

type outboundAdapter struct {
	client service.SeedServiceClient
}

var _ OutboundPort = &outboundAdapter{}

func NewOutbound(client service.SeedServiceClient) OutboundPort {
	return &outboundAdapter{
		client: client,
	}
}

func (i *outboundAdapter) send(ctx context.Context, sectorState kvsTypes.ActivationState) (kvsTypes.ActivationState, error) {
	res, err := i.client.ResolveKvsActivation(ctx, &connect.Request[proto.ResolveKvsActivationRequest]{
		Msg: &proto.ResolveKvsActivationRequest{
			SectorState: proto.KvsActivationState(sectorState),
		},
	})

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return kvsTypes.ActivationStateUnknown, nil
		}
		return kvsTypes.ActivationStateUnknown, fmt.Errorf("failed to set/get state KVS: %w", err)
	}

	return kvsTypes.ActivationState(res.Msg.EntireState), nil
}
