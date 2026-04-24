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
	"fmt"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/constants"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
)

type operationParam struct {
	command  proto.KvsOperation_Command
	key      string
	value    []byte
	receiver func(res *proto.KvsOperationResponse, err error)
}

type SectorManageMemberParam struct {
	dstNodeID *types.NodeID
	sectorID  kvsTypes.SectorID
	sectorNo  kvsTypes.SectorNo
	command   proto.SectorManageMember_Command
	members   map[kvsTypes.SectorNo]*types.NodeID
}

type OutboundPort interface {
	sendKvsOperation(param *operationParam)
	sendSectorManageMember(param *SectorManageMemberParam)
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

type operationHandler struct {
	receiver func(res *proto.KvsOperationResponse, err error)
}

func (h *operationHandler) OnResponse(packet *networkTypes.Packet) {
	h.receiver(packet.Content.GetKvsOperationResponse(), nil)
}

func (h *operationHandler) OnError(code constants.PacketErrorCode, message string) {
	h.receiver(nil, fmt.Errorf("packet error %d: %s", code, message))
}

func (o *outboundAdapter) sendKvsOperation(param *operationParam) {
	dst := types.NewHashedNodeID([]byte(param.key))

	content := &proto.PacketContent{
		Content: &proto.PacketContent_KvsOperation{
			KvsOperation: &proto.KvsOperation{
				Command: param.command,
				Key:     param.key,
				Value:   param.value,
			},
		},
	}

	o.transferer.Request(dst, networkTypes.PacketModeNone, content, &operationHandler{
		receiver: param.receiver,
	})
}

func (o *outboundAdapter) sendSectorManageMember(param *SectorManageMemberParam) {
	members := make(map[uint64]*proto.NodeID)
	for no, nid := range param.members {
		members[uint64(no)] = nid.Proto()
	}

	o.transferer.RequestOneWay(
		param.dstNodeID,
		networkTypes.PacketModeExplicit|networkTypes.PacketModeNoRetry,
		&proto.PacketContent{
			Content: &proto.PacketContent_SectorManageMember{
				SectorManageMember: &proto.SectorManageMember{
					SectorId: kvsTypes.MustMarshalSectorID(param.sectorID),
					SectorNo: uint64(param.sectorNo),
					Command:  param.command,
					Members:  members,
				},
			},
		},
	)
}
