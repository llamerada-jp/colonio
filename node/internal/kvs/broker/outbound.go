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
package broker

import (
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
)

type OutboundPort interface {
	sendSectorInformation(dst *types.NodeID, tailAddress *types.NodeID)
}

type outboundAdapter struct {
	transferer *transferer.Transferer
}

var _ OutboundPort = &outboundAdapter{}

func NewOutbound(transferer *transferer.Transferer) OutboundPort {
	return &outboundAdapter{
		transferer: transferer,
	}
}

func (o *outboundAdapter) sendSectorInformation(dst *types.NodeID, tailAddress *types.NodeID) {
	var protoTailAddress *proto.NodeID
	if tailAddress != nil {
		protoTailAddress = tailAddress.Proto()
	}
	o.transferer.RequestOneWay(
		dst,
		networkTypes.PacketModeExplicit|networkTypes.PacketModeNoRetry,
		&proto.PacketContent{
			Content: &proto.PacketContent_SectorInformation{
				SectorInformation: &proto.SectorInformation{
					TailAddress: protoTailAddress,
				},
			},
		},
	)
}
