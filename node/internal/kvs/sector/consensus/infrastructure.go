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
package consensus

import (
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
)

type Infrastructure interface {
	sendConsensusMessage(dstNodeID *types.NodeID, message *proto.ConsensusMessage)
}

type infrastructureImpl struct {
	transferer *transferer.Transferer
}

var _ Infrastructure = &infrastructureImpl{}

func NewInfrastructure(transferer *transferer.Transferer) Infrastructure {
	return &infrastructureImpl{
		transferer: transferer,
	}
}

func (i *infrastructureImpl) sendConsensusMessage(dstNodeID *types.NodeID, message *proto.ConsensusMessage) {
	i.transferer.RequestOneWay(
		dstNodeID,
		networkTypes.PacketModeExplicit|networkTypes.PacketModeNoRetry,
		&proto.PacketContent{
			Content: &proto.PacketContent_ConsensusMessage{
				ConsensusMessage: message,
			},
		},
	)
}
