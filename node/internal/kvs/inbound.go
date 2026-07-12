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

type kvsOperateResult struct {
	value    []byte
	revision uint64
	// lock fields of an applied LOCK_ACQUIRE
	lockGeneration uint64
	lockDeadlineMS int64
}

type inboundPort interface {
	// srcNodeID is the requesting node: the implicit lock owner (there is no
	// spoofable owner field on the wire).
	kvsOperate(operation *proto.KvsOperation, srcNodeID *types.NodeID) (proto.KvsOperationResponse_Error, *kvsOperateResult)
	processConsensusMessage(key kvsTypes.SectorKey, content *proto.ConsensusMessage)
	sectorManageMember(param *sectorManageMemberParam) error
	sectorActivate(srcNodeID *types.NodeID, sectorID kvsTypes.SectorID) bool
	sectorPrepareSplit(srcNodeID *types.NodeID, sectorID kvsTypes.SectorID) bool
}

var _ inboundPort = &KVS{}

type inboundAdapter struct {
	logger     *slog.Logger
	transferer *transferer.Transferer
	core       inboundPort
}

func SetupInbound(l *slog.Logger, t *transferer.Transferer, c inboundPort) {
	i := &inboundAdapter{
		logger:     l,
		transferer: t,
		core:       c,
	}

	transferer.SetRequestHandler[proto.PacketContent_KvsOperation](t, i.recvKvsOperation)
	transferer.SetRequestHandler[proto.PacketContent_ConsensusMessage](t, i.recvConsensusMessage)
	transferer.SetRequestHandler[proto.PacketContent_SectorManageMember](t, i.recvSectorManageMember)
	transferer.SetRequestHandler[proto.PacketContent_SectorActivate](t, i.recvSectorActivate)
	transferer.SetRequestHandler[proto.PacketContent_SectorPrepareSplit](t, i.recvSectorPrepareSplit)
}

func (i *inboundAdapter) recvKvsOperation(packet *networkTypes.Packet) {
	content := packet.Content.GetKvsOperation()

	errCode, result := i.core.kvsOperate(content, packet.SrcNodeID)

	response := &proto.KvsOperationResponse{Error: errCode}
	if result != nil {
		response.Value = result.value
		response.Revision = result.revision
		response.LockGeneration = result.lockGeneration
		response.LockDeadlineMs = result.lockDeadlineMS
	}
	i.transferer.Response(packet, &proto.PacketContent{
		Content: &proto.PacketContent_KvsOperationResponse{
			KvsOperationResponse: response,
		},
	})
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

	// REMOVE is a one-way notification: the sender has already dropped the
	// member entry, so there is no state machine waiting for a response.
	if command == proto.SectorManageMember_COMMAND_REMOVE {
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

func (i *inboundAdapter) recvSectorActivate(packet *networkTypes.Packet) {
	content := packet.Content.GetSectorActivate()
	sectorID, err := kvsTypes.UnmarshalSectorID(content.SectorId)
	if err != nil {
		i.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}

	success := i.core.sectorActivate(packet.SrcNodeID, sectorID)

	i.transferer.Response(packet, &proto.PacketContent{
		Content: &proto.PacketContent_SectorActivateResponse{
			SectorActivateResponse: &proto.SectorActivateResponse{
				Success: success,
			},
		},
	})
}

func (i *inboundAdapter) recvSectorPrepareSplit(packet *networkTypes.Packet) {
	content := packet.Content.GetSectorPrepareSplit()
	sectorID, err := kvsTypes.UnmarshalSectorID(content.SectorId)
	if err != nil {
		i.logger.Warn("Failed to parse promoter NodeID", "error", err)
		return
	}

	success := i.core.sectorPrepareSplit(packet.SrcNodeID, sectorID)

	i.transferer.Response(packet, &proto.PacketContent{
		Content: &proto.PacketContent_SectorPrepareSplitResponse{
			SectorPrepareSplitResponse: &proto.SectorPrepareSplitResponse{
				Success: success,
			},
		},
	})
}
