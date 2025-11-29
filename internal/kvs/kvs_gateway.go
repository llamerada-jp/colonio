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
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type KvsUsecase interface {
	kvsOperate(command proto.KvsOperation_Command, key string, value []byte) (proto.KvsOperationResponse_Error, []byte)
	raftConfigure(param *raftConfigureParam) error
	raftConfigureResponse(srcNodeID *shared.NodeID, sectorID config.SectorID, sectorNo config.SectorNo)
	processRaftMessage(key config.KvsSectorKey, content *proto.RaftMessage)
}

var _ KvsUsecase = &KVS{}

type KvsGateway struct {
	logger     *slog.Logger
	transferer *transferer.Transferer
	usecase    KvsUsecase
}

func NewKvsGateway(l *slog.Logger, t *transferer.Transferer, u KvsUsecase) *KvsGateway {
	g := &KvsGateway{
		logger:     l,
		transferer: t,
		usecase:    u,
	}

	transferer.SetRequestHandler[proto.PacketContent_KvsOperation](t, g.recvKvsOperation)
	transferer.SetRequestHandler[proto.PacketContent_RaftConfig](t, g.recvRaftConfig)
	transferer.SetRequestHandler[proto.PacketContent_RaftConfigResponse](t, g.recvRaftConfigResponse)
	transferer.SetRequestHandler[proto.PacketContent_RaftMessage](t, g.recvRaftMessage)

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

func (g *KvsGateway) recvRaftConfig(packet *shared.Packet) {
	content := packet.Content.GetRaftConfig()
	sectorID, err := UnmarshalSectorID(content.SectorId)
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

	if err := g.usecase.raftConfigure(&raftConfigureParam{
		command: command,
		sectorKey: config.KvsSectorKey{
			SectorID: sectorID,
			SectorNo: sectorNo,
		},
		head:    packet.SrcNodeID,
		members: members,
	}); err != nil {
		g.logger.Warn("Failed to configure raft", "error", err)
		return
	}

	// send response when the node is created or appended
	g.transferer.RequestOneWay(
		packet.SrcNodeID,
		shared.PacketModeExplicit|shared.PacketModeNoRetry,
		&proto.PacketContent{
			Content: &proto.PacketContent_RaftConfigResponse{
				RaftConfigResponse: &proto.RaftConfigResponse{
					SectorId: content.GetSectorId(),
					SectorNo: content.GetSectorNo(),
				},
			},
		},
	)
}

func (g *KvsGateway) recvRaftConfigResponse(packet *shared.Packet) {
	content := packet.Content.GetRaftConfigResponse()
	sectorID, err := UnmarshalSectorID(content.SectorId)
	if err != nil {
		g.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}
	sectorNo := config.SectorNo(content.SectorNo)

	g.usecase.raftConfigureResponse(packet.SrcNodeID, sectorID, sectorNo)
}

func (g *KvsGateway) recvRaftMessage(packet *shared.Packet) {
	content := packet.Content.GetRaftMessage()
	sectorID, err := UnmarshalSectorID(content.SectorId)
	if err != nil {
		g.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}

	sectorKey := config.KvsSectorKey{
		SectorID: sectorID,
		SectorNo: config.SectorNo(content.SectorNo),
	}

	g.usecase.processRaftMessage(sectorKey, content)
}
