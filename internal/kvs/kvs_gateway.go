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
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/kvs/sector"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type kvsUsecase interface {
	kvsOperate(command proto.KvsOperation_Command, key string, value []byte) (proto.KvsOperationResponse_Error, []byte)
	sectorManageMember(param *sectorManageMemberParam) error
	sectorManageMemberResponse(srcNodeID *shared.NodeID, sectorID config.SectorID, sectorNo config.SectorNo)
	processConsensusMessage(key config.KvsSectorKey, content *proto.ConsensusMessage)
}

var _ kvsUsecase = &KVS{}

type KvsGateway struct {
	logger     *slog.Logger
	transferer *transferer.Transferer
	usecase    kvsUsecase
}

func NewKvsGateway(l *slog.Logger, t *transferer.Transferer, u kvsUsecase) *KvsGateway {
	g := &KvsGateway{
		logger:     l,
		transferer: t,
		usecase:    u,
	}

	transferer.SetRequestHandler[proto.PacketContent_KvsOperation](t, g.recvKvsOperation)
	transferer.SetRequestHandler[proto.PacketContent_SectorManageMember](t, g.recvSectorManageMember)
	transferer.SetRequestHandler[proto.PacketContent_SectorManageMemberResponse](t, g.recvSectorManageMemberResponse)
	transferer.SetRequestHandler[proto.PacketContent_ConsensusMessage](t, g.recvConsensusMessage)

	return g
}

func (g *KvsGateway) recvKvsOperation(packet *shared.Packet) {
	content := packet.Content.GetKvsOperation()
	command := content.Command
	key := content.Key
	value := content.Value

	errCode, resValue := g.usecase.kvsOperate(command, key, value)

	g.transferer.Response(packet, &proto.PacketContent{
		Content: &proto.PacketContent_KvsOperationResponse{
			KvsOperationResponse: &proto.KvsOperationResponse{
				Error: errCode,
				Value: resValue,
			},
		},
	})
}

func (g *KvsGateway) recvSectorManageMember(packet *shared.Packet) {
	content := packet.Content.GetSectorManageMember()
	sectorID, err := sector.UnmarshalSectorID(content.SectorId)
	if err != nil {
		g.logger.Warn("Failed to parse promoter sectorID", "error", err)
		return
	}
	sectorNo := config.SectorNo(content.SectorNo)
	command := content.Command
	members := make(map[config.SectorNo]*shared.NodeID)
	for sectorNo, nodeID := range content.Members {
		var err error
		members[config.SectorNo(sectorNo)], err = shared.NewNodeIDFromProto(nodeID)
		if err != nil {
			g.logger.Warn("Failed to create NodeID from proto", "error", err, "nodeID", nodeID)
			return
		}
	}

	if err := g.usecase.sectorManageMember(&sectorManageMemberParam{
		command: command,
		sectorKey: config.KvsSectorKey{
			SectorID: sectorID,
			SectorNo: sectorNo,
		},
		head:    packet.SrcNodeID,
		members: members,
	}); err != nil {
		g.logger.Warn("Failed to configure sector", "error", err)
		return
	}

	// send response when the node is created or appended
	g.transferer.RequestOneWay(
		packet.SrcNodeID,
		shared.PacketModeExplicit|shared.PacketModeNoRetry,
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

func (g *KvsGateway) recvSectorManageMemberResponse(packet *shared.Packet) {
	content := packet.Content.GetSectorManageMemberResponse()
	sectorID, err := sector.UnmarshalSectorID(content.SectorId)
	if err != nil {
		g.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}
	sectorNo := config.SectorNo(content.SectorNo)

	g.usecase.sectorManageMemberResponse(packet.SrcNodeID, sectorID, sectorNo)
}

func (g *KvsGateway) recvConsensusMessage(packet *shared.Packet) {
	content := packet.Content.GetConsensusMessage()
	sectorID, err := sector.UnmarshalSectorID(content.SectorId)
	if err != nil {
		g.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}

	sectorKey := config.KvsSectorKey{
		SectorID: sectorID,
		SectorNo: config.SectorNo(content.SectorNo),
	}

	g.usecase.processConsensusMessage(sectorKey, content)
}
