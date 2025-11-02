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
	"context"
	"errors"
	"log/slog"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/shared"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	proto3 "google.golang.org/protobuf/proto"
)

const (
	raftTickDuration = 100 * time.Millisecond
)

type raftNodeManager interface {
	raftNodeError(sectorKey *config.KvsSectorKey, err error)
	raftNodeSendMessage(dstNodeID *shared.NodeID, data *proto.RaftMessage)
	raftNodeApplyProposal(sectorKey *config.KvsSectorKey, proposal *proto.RaftProposalManagement)
	raftNodeAppendNode(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo, nodeID *shared.NodeID)
	raftNodeRemoveNode(sectorKey *config.KvsSectorKey, sectorNo config.SectorNo)
}

type raftNodeStore interface {
	raftNodeApplyProposal(command *proto.RaftProposalStore)
	raftNodeGetSnapshot() ([]byte, error)
	raftNodeApplySnapshot(snapshot []byte) error
}

type raftNodeConfig struct {
	logger     *slog.Logger
	raftLogger raft.Logger
	manager    raftNodeManager
	store      raftNodeStore
	sectorKey  *config.KvsSectorKey
	join       bool
	member     map[config.SectorNo]*shared.NodeID
}

type raftNode struct {
	logger       *slog.Logger
	manager      raftNodeManager
	store        raftNodeStore
	sectorKey    config.KvsSectorKey
	ctx          context.Context
	etcdRaftNode raft.Node
	raftStorage  *raft.MemoryStorage

	snapshotCatchUpEntriesN uint64
	snapCount               uint64

	confState     raftpb.ConfState
	snapshotIndex uint64
	appliedIndex  uint64

	member map[config.SectorNo]*shared.NodeID
}

func newRaftNode(config *raftNodeConfig) *raftNode {
	n := &raftNode{
		logger:      config.logger,
		manager:     config.manager,
		store:       config.store,
		sectorKey:   *config.sectorKey,
		raftStorage: raft.NewMemoryStorage(),

		snapshotCatchUpEntriesN: 100,
		snapCount:               1000,

		member: config.member,
	}

	raftConfig := &raft.Config{
		Logger:                    config.raftLogger,
		ID:                        uint64(config.sectorKey.SectorNo),
		ElectionTick:              10,
		HeartbeatTick:             1,
		Storage:                   n.raftStorage,
		MaxSizePerMsg:             1024 * 1024,
		MaxInflightMsgs:           256,
		MaxUncommittedEntriesSize: 1 << 30,
	}

	if !config.join {
		peers := make([]raft.Peer, len(config.member))
		i := 0
		for sectorNo := range config.member {
			peers[i] = raft.Peer{
				ID: uint64(sectorNo),
			}
			i++
		}
		n.etcdRaftNode = raft.StartNode(raftConfig, peers)
	} else {
		n.etcdRaftNode = raft.RestartNode(raftConfig)
	}

	return n
}

func (n *raftNode) start(ctx context.Context) {
	n.ctx = ctx

	go func() {
		ticker := time.NewTicker(raftTickDuration)
		defer ticker.Stop()

		for {
			select {
			case <-n.ctx.Done():
				n.etcdRaftNode.Stop()
				return

			case <-ticker.C:
				n.etcdRaftNode.Tick()

			case rd := <-n.etcdRaftNode.Ready():
				if !raft.IsEmptySnap(rd.Snapshot) {
					n.raftStorage.ApplySnapshot(rd.Snapshot)
					n.confState = rd.Snapshot.Metadata.ConfState
					n.snapshotIndex = rd.Snapshot.Metadata.Index
					n.appliedIndex = rd.Snapshot.Metadata.Index
					n.store.raftNodeApplySnapshot(rd.Snapshot.Data)
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

				n.etcdRaftNode.Advance()
			}
		}
	}()
}

func (n *raftNode) stop() {
	n.etcdRaftNode.Stop()
}

func (n *raftNode) appendNode(sectorNo config.SectorNo, nodeID *shared.NodeID) error {
	return n.etcdRaftNode.ProposeConfChange(n.ctx, raftpb.ConfChange{
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  uint64(sectorNo),
		Context: []byte(nodeID.String()),
	})
}

func (n *raftNode) removeNode(sectorNo config.SectorNo) error {
	return n.etcdRaftNode.ProposeConfChange(n.ctx, raftpb.ConfChange{
		Type:   raftpb.ConfChangeRemoveNode,
		NodeID: uint64(sectorNo),
	})
}

func (n *raftNode) propose(p *proto.RaftProposal) {
	data, err := proto3.Marshal(p)
	if err != nil {
		panic("Failed to marshal Raft proposal: " + err.Error())
	}

	if err := n.etcdRaftNode.Propose(n.ctx, data); err != nil {
		n.manager.raftNodeError(&n.sectorKey, err)
	}
}

func (n *raftNode) processMessage(p *proto.RaftMessage) error {
	var msg raftpb.Message
	if err := msg.Unmarshal(p.Message); err != nil {
		return err
	}

	return n.etcdRaftNode.Step(n.ctx, msg)
}

func (n *raftNode) sendMessages(messages []raftpb.Message) error {
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

		sectorNoTo := config.SectorNo(msg.To)
		data, err := msg.Marshal()
		if err != nil {
			return err
		}
		dstNodeID, ok := n.member[sectorNoTo]
		if !ok {
			panic("Unknown node sectorNo for sending Raft message")
		}

		n.manager.raftNodeSendMessage(dstNodeID, &proto.RaftMessage{
			SectorId: MustMarshalSectorID(n.sectorKey.SectorID),
			SectorNo: uint64(sectorNoTo),
			Message:  data,
		})
	}
	return nil
}

func (n *raftNode) publishEntries(entries []raftpb.Entry) error {
	proposals := make([]*proto.RaftProposal, 0)

	for _, entry := range entries {
		switch entry.Type {
		case raftpb.EntryNormal:
			if len(entry.Data) > 0 {
				p := &proto.RaftProposal{}
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
			n.confState = *n.etcdRaftNode.ApplyConfChange(cc)

			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				var nodeID *shared.NodeID
				if len(cc.Context) != 0 {
					var err error
					nodeID, err = shared.NewNodeIDFromString(string(cc.Context))
					if err != nil {
						return err
					}
					n.member[config.SectorNo(cc.NodeID)] = nodeID
				} else {
					var ok bool
					nodeID, ok = n.member[config.SectorNo(cc.NodeID)]
					if !ok {
						return errors.New("missing node ID in conf change")
					}
				}
				n.manager.raftNodeAppendNode(&n.sectorKey, config.SectorNo(cc.NodeID), nodeID)

			case raftpb.ConfChangeRemoveNode:
				delete(n.member, config.SectorNo(cc.NodeID))
				n.manager.raftNodeRemoveNode(&n.sectorKey, config.SectorNo(cc.NodeID))

			default:
				n.logger.Warn("Unknown conf change type", "type", cc.Type)
			}
		}
	}

	for _, proposal := range proposals {
		if p := proposal.GetManagement(); p != nil {
			n.manager.raftNodeApplyProposal(&n.sectorKey, p)
		} else if p := proposal.GetStore(); p != nil {
			n.store.raftNodeApplyProposal(p)
		}
	}

	return nil
}

func (n *raftNode) maybeTriggerSnapshot() error {
	// Trigger a snapshot if the number of applied entries exceeds the threshold
	if n.appliedIndex-n.snapshotIndex <= n.snapCount {
		return nil
	}

	data, err := n.store.raftNodeGetSnapshot()
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
