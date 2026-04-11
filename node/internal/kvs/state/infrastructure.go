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
package state

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	service "github.com/llamerada-jp/colonio/api/colonio/v1alpha/v1alphaconnect"
	"github.com/llamerada-jp/colonio/types"
)

type Infrastructure interface {
	send(ctx context.Context, state types.KvsState) (types.KvsState, error)
}

type infrastructureImpl struct {
	client service.SeedServiceClient
}

var _ Infrastructure = &infrastructureImpl{}

func NewInfrastructure(client service.SeedServiceClient) Infrastructure {
	return &infrastructureImpl{
		client: client,
	}
}

func (i *infrastructureImpl) send(ctx context.Context, state types.KvsState) (types.KvsState, error) {
	res, err := i.client.StateKvs(ctx, &connect.Request[proto.StateKvsRequest]{
		Msg: &proto.StateKvsRequest{
			State: proto.KvsState(state),
		},
	})

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return types.KvsStateUnknown, nil
		}
		return types.KvsStateUnknown, fmt.Errorf("failed to set/get state KVS: %w", err)
	}

	return types.KvsState(res.Msg.EntireState), nil
}
