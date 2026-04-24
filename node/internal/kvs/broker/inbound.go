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
package broker

import (
	"log/slog"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/network/transferer"
	"github.com/llamerada-jp/colonio/types"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
)

type inboundPort interface {
	processSectorInformation(param *sectorInformationParam)
}

var _ inboundPort = &Broker{}

type inboundAdapter struct {
	logger     *slog.Logger
	transferer *transferer.Transferer
	core       inboundPort
}

func NewInbound(logger *slog.Logger, t *transferer.Transferer, core inboundPort) *inboundAdapter {
	i := &inboundAdapter{
		logger:     logger,
		transferer: t,
		core:       core,
	}

	transferer.SetRequestHandler[proto.PacketContent_SectorInformation](t, i.recvSectorInformation)

	return i
}

func (i *inboundAdapter) recvSectorInformation(packet *networkTypes.Packet) {
	content := packet.Content.GetSectorInformation()
	var tailAddress *types.NodeID
	if content.TailAddress != nil {
		var err error
		tailAddress, err = types.NewNodeIDFromProto(content.TailAddress)
		if err != nil {
			i.logger.Warn("Failed to create NodeID from proto", "error", err, "nodeID", content.TailAddress)
			return
		}
	}

	i.core.processSectorInformation(&sectorInformationParam{
		srcNodeID:   packet.SrcNodeID,
		tailAddress: tailAddress,
	})
}
