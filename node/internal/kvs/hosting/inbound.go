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
	"log/slog"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
)

type InboundPort interface {
	sectorManageMemberResponse(srcNodeID *types.NodeID, sectorID kvsTypes.SectorID, sectorNo kvsTypes.SectorNo)
}

var _ InboundPort = &Manager{}

type InboundAdapter struct {
	logger     *slog.Logger
	transferer *transferer.Transferer
	core       InboundPort
}

func SetupInbound(l *slog.Logger, t *transferer.Transferer, c InboundPort) {
	i := &InboundAdapter{
		logger:     l,
		transferer: t,
		core:       c,
	}

	transferer.SetRequestHandler[proto.PacketContent_SectorManageMemberResponse](i.transferer, i.recvSectorManageMemberResponse)
}

func (i *InboundAdapter) recvSectorManageMemberResponse(packet *networkTypes.Packet) {
	content := packet.Content.GetSectorManageMemberResponse()
	sectorID, err := kvsTypes.UnmarshalSectorID(content.SectorId)
	if err != nil {
		i.logger.Warn("Failed to parse sectorID", "error", err)
		return
	}
	sectorNo := kvsTypes.SectorNo(content.SectorNo)

	i.core.sectorManageMemberResponse(packet.SrcNodeID, sectorID, sectorNo)
}
