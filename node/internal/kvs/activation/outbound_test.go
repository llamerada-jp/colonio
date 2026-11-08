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
	"testing"

	"connectrpc.com/connect"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	service "github.com/llamerada-jp/colonio/api/colonio/v1alpha/v1alphaconnect"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSeedServiceClient struct {
	err      error
	response *proto.ResolveKvsActivationResponse
}

var _ service.SeedServiceClient = &mockSeedServiceClient{}

func (m *mockSeedServiceClient) AssignNode(context.Context, *connect.Request[proto.AssignNodeRequest]) (*connect.Response[proto.AssignNodeResponse], error) {
	panic("not implemented")
}

func (m *mockSeedServiceClient) UnassignNode(context.Context, *connect.Request[proto.UnassignNodeRequest]) (*connect.Response[proto.UnassignNodeResponse], error) {
	panic("not implemented")
}

func (m *mockSeedServiceClient) Keepalive(context.Context, *connect.Request[proto.KeepaliveRequest]) (*connect.Response[proto.KeepaliveResponse], error) {
	panic("not implemented")
}

func (m *mockSeedServiceClient) ReconcileNextNodes(context.Context, *connect.Request[proto.ReconcileNextNodesRequest]) (*connect.Response[proto.ReconcileNextNodesResponse], error) {
	panic("not implemented")
}

func (m *mockSeedServiceClient) SendSignal(context.Context, *connect.Request[proto.SendSignalRequest]) (*connect.Response[proto.SendSignalResponse], error) {
	panic("not implemented")
}

func (m *mockSeedServiceClient) PollSignal(context.Context, *connect.Request[proto.PollSignalRequest]) (*connect.ServerStreamForClient[proto.PollSignalResponse], error) {
	panic("not implemented")
}

func (m *mockSeedServiceClient) ResolveKvsActivation(_ context.Context, _ *connect.Request[proto.ResolveKvsActivationRequest]) (*connect.Response[proto.ResolveKvsActivationResponse], error) {
	if m.err != nil {
		return nil, m.err
	}
	return &connect.Response[proto.ResolveKvsActivationResponse]{
		Msg: m.response,
	}, nil
}

func TestOutbound_SuccessResponse(t *testing.T) {
	mockClient := &mockSeedServiceClient{
		response: &proto.ResolveKvsActivationResponse{
			EntireState: proto.ResolveKvsActivationResponse_ENTIRE_STATE_ACTIVE,
		},
	}
	outbound := NewOutbound(mockClient)

	ctx := context.Background()
	entireState, err := outbound.send(ctx, kvsTypes.SectorStateInactive)

	require.NoError(t, err)
	assert.Equal(t, kvsTypes.EntireStateActive, entireState)
}

func TestOutbound_ErrorHandling(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		expectEntireState kvsTypes.EntireState
		expectErrorNil    bool
	}{
		{
			name:              "ContextCanceled returns nil error",
			err:               context.Canceled,
			expectEntireState: kvsTypes.EntireStateUnknown,
			expectErrorNil:    true,
		},
		{
			name:              "ContextDeadlineExceeded returns nil error",
			err:               context.DeadlineExceeded,
			expectEntireState: kvsTypes.EntireStateUnknown,
			expectErrorNil:    true,
		},
		{
			name:              "OtherError returns wrapped error",
			err:               errors.New("connection refused"),
			expectEntireState: kvsTypes.EntireStateUnknown,
			expectErrorNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockSeedServiceClient{
				err: tt.err,
			}
			outbound := NewOutbound(mockClient)

			ctx := context.Background()
			entireState, err := outbound.send(ctx, kvsTypes.SectorStateInactive)

			assert.Equal(t, tt.expectEntireState, entireState)
			if tt.expectErrorNil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.ErrorContains(t, err, "failed to set/get state KVS")
			}
		})
	}
}

func TestOutbound_EnumConversion(t *testing.T) {
	tests := []struct {
		name          string
		requestState  kvsTypes.SectorState
		responseState proto.ResolveKvsActivationResponse_EntireState
		expect        kvsTypes.EntireState
	}{
		{
			name:          "Inactive request Active response",
			requestState:  kvsTypes.SectorStateInactive,
			responseState: proto.ResolveKvsActivationResponse_ENTIRE_STATE_ACTIVE,
			expect:        kvsTypes.EntireStateActive,
		},
		{
			name:          "Active request Inactive response",
			requestState:  kvsTypes.SectorStateActive,
			responseState: proto.ResolveKvsActivationResponse_ENTIRE_STATE_INACTIVE,
			expect:        kvsTypes.EntireStateInactive,
		},
		{
			name:          "Inactive request Inactive response",
			requestState:  kvsTypes.SectorStateInactive,
			responseState: proto.ResolveKvsActivationResponse_ENTIRE_STATE_INACTIVE,
			expect:        kvsTypes.EntireStateInactive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockSeedServiceClient{
				response: &proto.ResolveKvsActivationResponse{
					EntireState: tt.responseState,
				},
			}
			outbound := NewOutbound(mockClient)

			ctx := context.Background()
			entireState, err := outbound.send(ctx, tt.requestState)

			require.NoError(t, err)
			assert.Equal(t, tt.expect, entireState)
		})
	}
}
