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
package kvs

import (
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type raftMessageParam struct {
	dstNodeID *shared.NodeID
	message   *proto.RaftMessage
}

type RaftInfrastructure interface {
	sendRaftMessage(param *raftMessageParam)
}

type raftInfrastructureImpl struct {
	transferer *transferer.Transferer
}

var _ RaftInfrastructure = &raftInfrastructureImpl{}

func NewRaftInfrastructure(transferer *transferer.Transferer) RaftInfrastructure {
	return &raftInfrastructureImpl{
		transferer: transferer,
	}
}

func (i *raftInfrastructureImpl) sendRaftMessage(param *raftMessageParam) {
	i.transferer.RequestOneWay(
		param.dstNodeID,
		shared.PacketModeExplicit|shared.PacketModeNoRetry,
		&proto.PacketContent{
			Content: &proto.PacketContent_RaftMessage{
				RaftMessage: param.message,
			},
		},
	)
}
