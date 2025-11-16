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
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/constants"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type operationParam struct {
	command  proto.KvsOperation_Command
	key      string
	value    []byte
	receiver func(res *proto.KvsOperationResponse, err error)
}

type raftConfigParam struct {
	dstNodeID *shared.NodeID
	sectorID  config.SectorID
	sectorNo  config.SectorNo
	command   proto.RaftConfig_Command
	members   map[config.SectorNo]*shared.NodeID
}

type KvsInfrastructure interface {
	sendOperation(param *operationParam)
	sendRaftConfig(param *raftConfigParam)
}

type kvsInfrastructureImpl struct {
	transferer *transferer.Transferer
}

var _ KvsInfrastructure = &kvsInfrastructureImpl{}

func NewKvsInfrastructure(transferer *transferer.Transferer) KvsInfrastructure {
	return &kvsInfrastructureImpl{
		transferer: transferer,
	}
}

type operationHandler struct {
	receiver func(res *proto.KvsOperationResponse, err error)
}

func (h *operationHandler) OnResponse(packet *shared.Packet) {
	h.receiver(packet.Content.GetKvsOperationResponse(), nil)
}

func (h *operationHandler) OnError(code constants.PacketErrorCode, message string) {
	h.receiver(nil, fmt.Errorf("packet error %d: %s", code, message))
}

func (i *kvsInfrastructureImpl) sendOperation(param *operationParam) {
	dst := shared.NewHashedNodeID([]byte(param.key))

	content := &proto.PacketContent{
		Content: &proto.PacketContent_KvsOperation{
			KvsOperation: &proto.KvsOperation{
				Command: param.command,
				Key:     param.key,
				Value:   param.value,
			},
		},
	}

	i.transferer.Request(dst, shared.PacketModeNone, content, &operationHandler{
		receiver: param.receiver,
	})
}

func (i *kvsInfrastructureImpl) sendRaftConfig(param *raftConfigParam) {
	members := make(map[uint64]*proto.NodeID)
	for no, nid := range param.members {
		members[uint64(no)] = nid.Proto()
	}

	i.transferer.RequestOneWay(
		param.dstNodeID,
		shared.PacketModeExplicit|shared.PacketModeNoRetry,
		&proto.PacketContent{
			Content: &proto.PacketContent_RaftConfig{
				RaftConfig: &proto.RaftConfig{
					SectorId: MustMarshalSectorID(param.sectorID),
					SectorNo: uint64(param.sectorNo),
					Command:  param.command,
					Members:  members,
				},
			},
		},
	)
}
