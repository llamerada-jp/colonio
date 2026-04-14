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
	"log/slog"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
)

type inboundPort interface {
	kvsOperate(command proto.KvsOperation_Command, key string, value []byte) (proto.KvsOperationResponse_Error, []byte)
	sectorManageMember(param *sectorManageMemberParam) error
	sectorManageMemberResponse(srcNodeID *types.NodeID, sectorID kvsTypes.SectorID, sectorNo kvsTypes.SectorNo)
	processConsensusMessage(key kvsTypes.SectorKey, content *proto.ConsensusMessage)
}

var _ inboundPort = &KVS{}

type inboundAdapter struct {
	logger     *slog.Logger
	transferer *transferer.Transferer
	core       inboundPort
}

func NewInbound(l *slog.Logger, t *transferer.Transferer, c inboundPort) {
	i := &inboundAdapter{
		logger:     l,
		transferer: t,
		core:       c,
	}

	transferer.SetRequestHandler[proto.PacketContent_KvsOperation](t, i.recvKvsOperation)
	transferer.SetRequestHandler[proto.PacketContent_SectorManageMember](t, i.recvSectorManageMember)
	transferer.SetRequestHandler[proto.PacketContent_SectorManageMemberResponse](t, i.recvSectorManageMemberResponse)
	transferer.SetRequestHandler[proto.PacketContent_ConsensusMessage](t, i.recvConsensusMessage)
}

func (i *inboundAdapter) recvKvsOperation(packet *networkTypes.Packet) {
	content := packet.Content.GetKvsOperation()
	command := content.Command
	key := content.Key
	value := content.Value

	errCode, resValue := i.core.kvsOperate(command, key, value)

	i.transferer.Response(packet, &proto.PacketContent{
		Content: &proto.PacketContent_KvsOperationResponse{
			KvsOperationResponse: &proto.KvsOperationResponse{
				Error: errCode,
				Value: resValue,
			},
		},
	})
}

func (i *inboundAdapter) recvSectorManageMember(packet *networkTypes.Packet) {
	content := packet.Content.GetSectorManageMember()
	sectorID, err := kvsTypes.UnmarshalSectorID(content.SectorId)
	if err != nil {
		i.logger.Warn("Failed to parse promoter sectorID", "error", err)
		return
	}
	sectorNo := kvsTypes.SectorNo(content.SectorNo)
	command := content.Command
	members := make(map[kvsTypes.SectorNo]*types.NodeID)
	for sectorNo, nodeID := range content.Members {
		var err error
		members[kvsTypes.SectorNo(sectorNo)], err = types.NewNodeIDFromProto(nodeID)
		if err != nil {
			i.logger.Warn("Failed to create NodeID from proto", "error", err, "nodeID", nodeID)
			return
		}
	}

	if err := i.core.sectorManageMember(&sectorManageMemberParam{
		command: command,
		sectorKey: kvsTypes.SectorKey{
			SectorID: sectorID,
			SectorNo: sectorNo,
		},
		head:    packet.SrcNodeID,
		members: members,
	}); err != nil {
		i.logger.Warn("Failed to configure sector", "error", err)
		return
	}

	// send response when the node is created or appended
	i.transferer.RequestOneWay(
		packet.SrcNodeID,
		networkTypes.PacketModeExplicit|networkTypes.PacketModeNoRetry,
		&proto.PacketContent{
			Content: &proto.PacketContent_SectorManageMemberResponse{
				SectorManageMemberResponse: &proto.SectorManageMemberResponse{
					SectorId: content.GetSectorId(),
					SectorNo: content.GetSectorNo(),
				},
			},
		},
	)
}

func (i *inboundAdapter) recvSectorManageMemberResponse(packet *networkTypes.Packet) {
	content := packet.Content.GetSectorManageMemberResponse()
	sectorID, err := kvsTypes.UnmarshalSectorID(content.SectorId)
	if err != nil {
		i.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}
	sectorNo := kvsTypes.SectorNo(content.SectorNo)

	i.core.sectorManageMemberResponse(packet.SrcNodeID, sectorID, sectorNo)
}

func (i *inboundAdapter) recvConsensusMessage(packet *networkTypes.Packet) {
	content := packet.Content.GetConsensusMessage()
	sectorID, err := kvsTypes.UnmarshalSectorID(content.SectorId)
	if err != nil {
		i.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}

	sectorKey := kvsTypes.SectorKey{
		SectorID: sectorID,
		SectorNo: kvsTypes.SectorNo(content.SectorNo),
	}

	i.core.processConsensusMessage(sectorKey, content)
}
