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
package hosting

import (
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
)

type OutboundAdapter struct {
	transferer *transferer.Transferer
}

func NewOutbound(transferer *transferer.Transferer) *OutboundAdapter {
	return &OutboundAdapter{
		transferer: transferer,
	}
}

func (o *OutboundAdapter) sendSectorManageMember(param *SectorManageMemberParam) {
	members := make(map[uint64]*proto.NodeID)
	for no, nid := range param.Members {
		members[uint64(no)] = nid.Proto()
	}

	o.transferer.RequestOneWay(
		param.DstNodeID,
		networkTypes.PacketModeExplicit|networkTypes.PacketModeNoRetry,
		&proto.PacketContent{
			Content: &proto.PacketContent_SectorManageMember{
				SectorManageMember: &proto.SectorManageMember{
					SectorId: kvsTypes.MustMarshalSectorID(param.SectorID),
					SectorNo: uint64(param.SectorNo),
					Command:  param.Command,
					Members:  members,
				},
			},
		},
	)
}
