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
package consensus

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	proto3 "google.golang.org/protobuf/proto"
)

const (
	raftTickDuration = 100 * time.Millisecond
)

type Handler interface {
	ConsensusError(err error)
	ConsensusAppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID)
	ConsensusRemoveNode(sectorNo kvsTypes.SectorNo)
	ConsensusApplyProposal(proposal *proto.ConsensusProposal)
	ConsensusGetSnapshot() ([]byte, error)
	ConsensusApplySnapshot(snapshot []byte) error
}

type Config struct {
	Logger         *slog.Logger
	RaftLogger     raft.Logger
	Handler        Handler
	Infrastructure Infrastructure
	SectorKey      *kvsTypes.SectorKey
	Join           bool
	Members        map[kvsTypes.SectorNo]*types.NodeID
}

type Consensus struct {
	logger         *slog.Logger
	handler        Handler
	infrastructure Infrastructure
	sectorKey      kvsTypes.SectorKey
	ctx            context.Context
	raftNode       raft.Node
	raftStorage    *raft.MemoryStorage

	snapshotCatchUpEntriesN uint64
	snapCount               uint64

	confState     raftpb.ConfState
	snapshotIndex uint64
	appliedIndex  uint64

	members map[kvsTypes.SectorNo]*types.NodeID
}

func NewConsensus(config *Config) *Consensus {
	n := &Consensus{
		logger:         config.Logger,
		handler:        config.Handler,
		infrastructure: config.Infrastructure,
		sectorKey:      *config.SectorKey,
		raftStorage:    raft.NewMemoryStorage(),

		snapshotCatchUpEntriesN: 100,
		snapCount:               1000,

		members: maps.Clone(config.Members),
	}

	raftConfig := &raft.Config{
		Logger:                    config.RaftLogger,
		ID:                        uint64(config.SectorKey.SectorNo),
		ElectionTick:              10,
		HeartbeatTick:             1,
		Storage:                   n.raftStorage,
		MaxSizePerMsg:             1024 * 1024,
		MaxInflightMsgs:           256,
		MaxUncommittedEntriesSize: 1 << 30,
	}

	if !config.Join {
		peers := []raft.Peer{}
		for sectorNo := range config.Members {
			peers = append(peers, raft.Peer{
				ID: uint64(sectorNo),
			})
		}
		n.raftNode = raft.StartNode(raftConfig, peers)
	} else {
		n.raftNode = raft.RestartNode(raftConfig)
	}

	return n
}

func (n *Consensus) Start(ctx context.Context) {
	n.ctx = ctx

	go func() {
		ticker := time.NewTicker(raftTickDuration)
		defer ticker.Stop()

		for {
			select {
			case <-n.ctx.Done():
				n.raftNode.Stop()
				return

			case <-ticker.C:
				n.raftNode.Tick()

			case rd := <-n.raftNode.Ready():
				if !raft.IsEmptySnap(rd.Snapshot) {
					n.raftStorage.ApplySnapshot(rd.Snapshot)
					n.confState = rd.Snapshot.Metadata.ConfState
					n.snapshotIndex = rd.Snapshot.Metadata.Index
					n.appliedIndex = rd.Snapshot.Metadata.Index
					n.handler.ConsensusApplySnapshot(rd.Snapshot.Data)
				}

				if err := n.raftStorage.Append(rd.Entries); err != nil {
					n.logger.Error("Failed to append entries to storage", "error", err)
				}

				if err := n.sendMessages(rd.Messages); err != nil {
					n.logger.Error("Failed to send raft messages", "error", err)
				}

				entries := rd.CommittedEntries
				if err := n.publishEntries(entries); err != nil {
					n.logger.Error("Failed to publish committed entries", "error", err)
				}

				if err := n.maybeTriggerSnapshot(); err != nil {
					n.logger.Error("Failed to trigger snapshot", "error", err)
				}

				n.raftNode.Advance()
			}
		}
	}()
}

func (n *Consensus) Stop() {
	n.raftNode.Stop()
}

// TODO: do append and remove in batch using ConfChangeV2
func (n *Consensus) AppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	go func() {
		if err := n.raftNode.ProposeConfChange(n.ctx, raftpb.ConfChangeV2{
			Transition: raftpb.ConfChangeTransitionAuto,
			Changes: []raftpb.ConfChangeSingle{{
				Type:   raftpb.ConfChangeAddNode,
				NodeID: uint64(sectorNo),
			}},
			Context: []byte(nodeID.String()),
		}); err != nil {
			// TODO: handle error
			n.logger.Error("Failed to propose conf change for adding node", "error", err)
		}
	}()
}

func (n *Consensus) RemoveNode(sectorNo kvsTypes.SectorNo) {
	go func() {
		if err := n.raftNode.ProposeConfChange(n.ctx, raftpb.ConfChangeV2{
			Transition: raftpb.ConfChangeTransitionAuto,
			Changes: []raftpb.ConfChangeSingle{{
				Type:   raftpb.ConfChangeRemoveNode,
				NodeID: uint64(sectorNo),
			}},
		}); err != nil {
			// TODO: handle error
			n.logger.Error("Failed to propose conf change for removing node", "error", err)
		}
	}()
}

func (n *Consensus) Propose(p *proto.ConsensusProposal) {
	data, err := proto3.Marshal(p)
	if err != nil {
		panic("Failed to marshal Raft proposal: " + err.Error())
	}

	if err := n.raftNode.Propose(n.ctx, data); err != nil {
		n.handler.ConsensusError(err)
	}
}

func (n *Consensus) ProcessMessage(p *proto.ConsensusMessage) error {
	var msg raftpb.Message
	if err := msg.Unmarshal(p.Message); err != nil {
		return err
	}

	return n.raftNode.Step(n.ctx, msg)
}

func (n *Consensus) sendMessages(messages []raftpb.Message) error {
	for _, msg := range messages {
		if msg.To == 0 {
			continue // skip messages without a target
		}

		// When there is a `raftpb.EntryConfChange` after creating the snapshot,
		// then the confState included in the snapshot is out of date. so We need
		// to update the confState before sending a snapshot to a follower.
		if msg.Type == raftpb.MsgSnap {
			msg.Snapshot.Metadata.ConfState = n.confState
		}

		sectorNoTo := kvsTypes.SectorNo(msg.To)
		data, err := msg.Marshal()
		if err != nil {
			return err
		}
		dstNodeID, ok := n.members[sectorNoTo]
		if !ok {
			n.logger.Warn("Unknown node sectorNo for sending Raft message")
			continue
		}

		n.infrastructure.sendConsensusMessage(
			dstNodeID,
			&proto.ConsensusMessage{
				SectorId: kvsTypes.MustMarshalSectorID(n.sectorKey.SectorID),
				SectorNo: uint64(sectorNoTo),
				Message:  data,
			},
		)
	}
	return nil
}

func (n *Consensus) publishEntries(entries []raftpb.Entry) error {
	proposals := make([]*proto.ConsensusProposal, 0)

	for _, entry := range entries {
		switch entry.Type {
		case raftpb.EntryNormal:
			if len(entry.Data) > 0 {
				p := &proto.ConsensusProposal{}
				if err := proto3.Unmarshal(entry.Data, p); err != nil {
					return err
				}
				proposals = append(proposals, p)
			}

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(entry.Data); err != nil {
				return err
			}
			n.confState = *n.raftNode.ApplyConfChange(cc)

			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				var nodeID *types.NodeID
				if len(cc.Context) != 0 {
					var err error
					nodeID, err = types.NewNodeIDFromString(string(cc.Context))
					if err != nil {
						return err
					}
					n.members[kvsTypes.SectorNo(cc.NodeID)] = nodeID
				} else {
					var ok bool
					nodeID, ok = n.members[kvsTypes.SectorNo(cc.NodeID)]
					if !ok {
						return errors.New("missing node ID in conf change")
					}
				}
				n.handler.ConsensusAppendNode(kvsTypes.SectorNo(cc.NodeID), nodeID)

			case raftpb.ConfChangeRemoveNode:
				delete(n.members, kvsTypes.SectorNo(cc.NodeID))
				n.handler.ConsensusRemoveNode(kvsTypes.SectorNo(cc.NodeID))

			default:
				n.logger.Warn("Unknown conf change type", "type", cc.Type)
			}

		case raftpb.EntryConfChangeV2:
			var cc2 raftpb.ConfChangeV2
			if err := cc2.Unmarshal(entry.Data); err != nil {
				return err
			}
			n.confState = *n.raftNode.ApplyConfChange(cc2)

			for _, change := range cc2.Changes {
				switch change.Type {
				case raftpb.ConfChangeAddNode:
					var nodeID *types.NodeID
					// TODO: Context is used for both AddNode and RemoveNode, but it is only needed for AddNode. We should separate the context for AddNode and RemoveNode in ConfChangeV2.
					if len(cc2.Context) != 0 {
						var err error
						nodeID, err = types.NewNodeIDFromString(string(cc2.Context))
						if err != nil {
							return err
						}
						n.members[kvsTypes.SectorNo(change.NodeID)] = nodeID
					} else {
						var ok bool
						nodeID, ok = n.members[kvsTypes.SectorNo(change.NodeID)]
						if !ok {
							return errors.New("missing node ID in conf change")
						}
					}
					n.handler.ConsensusAppendNode(kvsTypes.SectorNo(change.NodeID), nodeID)

				case raftpb.ConfChangeRemoveNode:
					delete(n.members, kvsTypes.SectorNo(change.NodeID))
					n.handler.ConsensusRemoveNode(kvsTypes.SectorNo(change.NodeID))

				default:
					n.logger.Warn("Unknown conf change type", "type", change.Type)
				}
			}
		}
	}

	for _, proposal := range proposals {
		n.handler.ConsensusApplyProposal(proposal)
	}

	return nil
}

func (n *Consensus) maybeTriggerSnapshot() error {
	// Trigger a snapshot if the number of applied entries exceeds the threshold
	if n.appliedIndex-n.snapshotIndex <= n.snapCount {
		return nil
	}

	data, err := n.handler.ConsensusGetSnapshot()
	if err != nil {
		return err
	}

	_, err = n.raftStorage.CreateSnapshot(n.appliedIndex, &n.confState, data)
	if err != nil {
		return err
	}

	compactIndex := uint64(1)
	if n.appliedIndex > n.snapshotCatchUpEntriesN {
		compactIndex = n.appliedIndex - n.snapshotCatchUpEntriesN
	}
	if err := n.raftStorage.Compact(compactIndex); err != nil {
		if !errors.Is(err, raft.ErrCompacted) {
			return err
		}
	}

	n.snapshotIndex = n.appliedIndex
	return nil
}
