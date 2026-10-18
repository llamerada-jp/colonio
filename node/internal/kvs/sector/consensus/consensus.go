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
	"slices"
	"sync"
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
	// proposeTimeout bounds raftNode.Propose, which otherwise blocks until a
	// leader exists. A leaderless (quorum-lost) group must not block the
	// caller forever; the sector's retry loop re-proposes pending proposals.
	proposeTimeout = 2 * time.Second
	// promoteCheckTicks is how often, in raft ticks, the leader checks
	// whether learners have caught up and can be promoted to voters.
	promoteCheckTicks = 10
)

type Handler interface {
	ConsensusError(err error)
	ConsensusAppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID)
	ConsensusRemoveNode(sectorNo kvsTypes.SectorNo)
	ConsensusApplyProposal(proposal *proto.ConsensusProposal) error
	ConsensusGetSnapshot() ([]byte, error)
	ConsensusApplySnapshot(snapshot []byte) error
}

type Config struct {
	Logger     *slog.Logger
	RaftLogger raft.Logger
	Handler    Handler
	Outbound   OutboundPort
	SectorKey  *kvsTypes.SectorKey
	Join       bool
	Members    map[kvsTypes.SectorNo]*types.NodeID
}

type Consensus struct {
	logger      *slog.Logger
	handler     Handler
	outbound    OutboundPort
	sectorKey   kvsTypes.SectorKey
	ctx         context.Context
	raftNode    raft.Node
	raftStorage *raft.MemoryStorage

	snapshotCatchUpEntriesN uint64
	snapCount               uint64

	// mtx protects confState and members. The event loop writes them, and
	// AppendNode reads confState from the sector's retry loop.
	mtx           sync.RWMutex
	confState     raftpb.ConfState
	snapshotIndex uint64
	appliedIndex  uint64

	members map[kvsTypes.SectorNo]*types.NodeID

	// stopCh ends the event loop started by Start; loopDone is closed when it
	// has returned.
	stopCh   chan struct{}
	stopOnce sync.Once
	loopDone chan struct{}
}

func NewConsensus(config *Config) *Consensus {
	n := &Consensus{
		logger:      config.Logger,
		handler:     config.Handler,
		outbound:    config.Outbound,
		sectorKey:   *config.SectorKey,
		raftStorage: raft.NewMemoryStorage(),

		snapshotCatchUpEntriesN: 100,
		snapCount:               1000,

		members: maps.Clone(config.Members),

		stopCh:   make(chan struct{}),
		loopDone: make(chan struct{}),
	}

	raftConfig := &raft.Config{
		Logger:        config.RaftLogger,
		ID:            uint64(config.SectorKey.SectorNo),
		ElectionTick:  10,
		HeartbeatTick: 1,
		// Without CheckQuorum, a leader whose followers all died stays leader
		// forever, so the sector never sees the group as leaderless. With it,
		// such a leader steps down after an election timeout.
		CheckQuorum: true,
		// Without PreVote, a member cut off from the quorum keeps raising its
		// term by campaigning, and disrupts the leader when it reconnects.
		PreVote:                   true,
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
		defer close(n.loopDone)
		ticker := time.NewTicker(raftTickDuration)
		defer ticker.Stop()
		tickCount := 0

		for {
			select {
			case <-n.ctx.Done():
				n.raftNode.Stop()
				return

			case <-n.stopCh:
				return

			case <-ticker.C:
				n.raftNode.Tick()
				tickCount++
				if tickCount >= promoteCheckTicks {
					tickCount = 0
					n.maybePromoteLearners()
				}

			case rd := <-n.raftNode.Ready():
				if !raft.IsEmptySnap(rd.Snapshot) {
					n.raftStorage.ApplySnapshot(rd.Snapshot)
					n.mtx.Lock()
					n.confState = rd.Snapshot.Metadata.ConfState
					n.mtx.Unlock()
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

// Stop stops the raft node and makes the event loop started by Start return.
// It does not wait for the event loop, so it is safe to call before Start.
func (n *Consensus) Stop() {
	n.stopOnce.Do(func() { close(n.stopCh) })
	n.raftNode.Stop()
}

// Status returns the current raft status. The sector also uses it to detect
// a group that has stayed leaderless too long.
func (n *Consensus) Status() raft.Status {
	return n.raftNode.Status()
}

// AppendNode adds the node as a learner. A learner does not count toward the
// quorum, so appending a node that is dead or still catching up cannot break
// the quorum. The leader promotes it to a voter once it has caught up
// (maybePromoteLearners), and the handler is notified only then.
//
// The sector's retry loop calls this until the promotion is applied, so it
// does nothing when the node is already in the config: adding a voter as a
// learner again would demote it.
//
// TODO: do append and remove in batch using ConfChangeV2
func (n *Consensus) AppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	id := uint64(sectorNo)

	n.mtx.RLock()
	inConfig := slices.Contains(n.confState.Voters, id) ||
		slices.Contains(n.confState.Learners, id)
	n.mtx.RUnlock()
	if inConfig {
		return
	}

	go func() {
		if err := n.raftNode.ProposeConfChange(n.ctx, raftpb.ConfChangeV2{
			Transition: raftpb.ConfChangeTransitionAuto,
			Changes: []raftpb.ConfChangeSingle{{
				Type:   raftpb.ConfChangeAddLearnerNode,
				NodeID: id,
			}},
			Context: []byte(nodeID.String()),
		}); err != nil {
			// TODO: handle error
			n.logger.Error("Failed to propose conf change for adding learner", "error", err)
		}
	}()
}

// maybePromoteLearners proposes promoting the learners that have caught up
// with the commit index to voters. Only the leader knows the progress of the
// others, so it does nothing on the other members. A dead learner never
// catches up and stays a learner until the membership manager removes it.
// Runs on the event loop.
func (n *Consensus) maybePromoteLearners() {
	status := n.raftNode.Status()
	if status.RaftState != raft.StateLeader {
		return
	}

	n.mtx.RLock()
	learners := slices.Clone(n.confState.Learners)
	contexts := make(map[uint64][]byte, len(learners))
	for _, id := range learners {
		if nodeID, ok := n.members[kvsTypes.SectorNo(id)]; ok {
			contexts[id] = []byte(nodeID.String())
		}
	}
	n.mtx.RUnlock()

	for _, id := range learners {
		pr, ok := status.Progress[id]
		if !ok || status.Commit == 0 || pr.Match < status.Commit {
			continue
		}

		// This is proposed again on the next check until the promotion is
		// applied, so the wait is bounded like Propose.
		go func(id uint64, nodeIDContext []byte) {
			ctx, cancel := context.WithTimeout(n.ctx, proposeTimeout)
			defer cancel()
			if err := n.raftNode.ProposeConfChange(ctx, raftpb.ConfChangeV2{
				Transition: raftpb.ConfChangeTransitionAuto,
				Changes: []raftpb.ConfChangeSingle{{
					Type:   raftpb.ConfChangeAddNode,
					NodeID: id,
				}},
				Context: nodeIDContext,
			}); err != nil {
				n.logger.Error("Failed to propose conf change for promoting learner", "error", err)
			}
		}(id, contexts[id])
	}
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

	ctx, cancel := context.WithTimeout(n.ctx, proposeTimeout)
	defer cancel()
	if err := n.raftNode.Propose(ctx, data); err != nil {
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

		n.outbound.sendConsensusMessage(
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
			n.mtx.Lock()
			n.confState = *n.raftNode.ApplyConfChange(cc)
			n.mtx.Unlock()

			if err := n.applyConfChangeSingle(cc.Type, cc.NodeID, cc.Context); err != nil {
				return err
			}

		case raftpb.EntryConfChangeV2:
			var cc2 raftpb.ConfChangeV2
			if err := cc2.Unmarshal(entry.Data); err != nil {
				return err
			}
			n.mtx.Lock()
			n.confState = *n.raftNode.ApplyConfChange(cc2)
			n.mtx.Unlock()

			for _, change := range cc2.Changes {
				// TODO: Context is used for both AddNode and RemoveNode, but it is only needed for AddNode. We should separate the context for AddNode and RemoveNode in ConfChangeV2.
				if err := n.applyConfChangeSingle(change.Type, change.NodeID, cc2.Context); err != nil {
					return err
				}
			}
		}
	}

	for _, proposal := range proposals {
		// These entries are already committed, and Advance() runs after this
		// anyway, so returning here would skip the rest of the batch for good
		// and leave this member diverged from the group.
		if err := n.handler.ConsensusApplyProposal(proposal); err != nil {
			n.logger.Error("Failed to apply committed proposal", "sectorKey", n.sectorKey.String(), "error", err)
		}
	}

	return nil
}

// applyConfChangeSingle updates the member table for one applied conf change
// and notifies the handler. Adding a learner only records its node ID so that
// messages can be sent to it; the handler is notified when the member becomes
// a voter, since only then does it take part in the quorum.
func (n *Consensus) applyConfChangeSingle(ccType raftpb.ConfChangeType, id uint64, context []byte) error {
	sectorNo := kvsTypes.SectorNo(id)

	switch ccType {
	case raftpb.ConfChangeAddLearnerNode:
		if len(context) != 0 {
			nodeID, err := types.NewNodeIDFromString(string(context))
			if err != nil {
				return err
			}
			n.mtx.Lock()
			n.members[sectorNo] = nodeID
			n.mtx.Unlock()
		}

	case raftpb.ConfChangeAddNode:
		var nodeID *types.NodeID
		if len(context) != 0 {
			var err error
			nodeID, err = types.NewNodeIDFromString(string(context))
			if err != nil {
				return err
			}
			n.mtx.Lock()
			n.members[sectorNo] = nodeID
			n.mtx.Unlock()
		} else {
			var ok bool
			n.mtx.RLock()
			nodeID, ok = n.members[sectorNo]
			n.mtx.RUnlock()
			if !ok {
				return errors.New("missing node ID in conf change")
			}
		}
		n.handler.ConsensusAppendNode(sectorNo, nodeID)

	case raftpb.ConfChangeRemoveNode:
		n.mtx.Lock()
		delete(n.members, sectorNo)
		n.mtx.Unlock()
		n.handler.ConsensusRemoveNode(sectorNo)

	default:
		n.logger.Warn("Unknown conf change type", "type", ccType)
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
