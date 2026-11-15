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
	"fmt"
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

	defaultSnapshotCount          = 1000
	defaultSnapshotCatchUpEntries = 100
)

// ErrSnapshotUnavailable is returned by Handler.ConsensusGetSnapshot when the
// handler cannot provide a snapshot now. No snapshot is taken then.
var ErrSnapshotUnavailable = errors.New("snapshot is unavailable")

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

	// SnapshotCount is the number of applied entries after which a snapshot
	// is taken and the log is compacted. SnapshotCatchUpEntries is the number
	// of entries kept in the log at a compaction, for followers that are
	// slightly behind. Zero means the default.
	SnapshotCount          uint64
	SnapshotCatchUpEntries uint64
}

// snapshotGuardStorage keeps raft from panicking when a leader has to send a
// snapshot before it has created the first one. Raft falls back to sending a
// snapshot whenever a follower needs entries that are not in the leader's
// log, and panics on an empty snapshot. Returning
// ErrSnapshotTemporarilyUnavailable, the error the raft Storage interface
// defines for this case, makes raft skip the send instead. A snapshot that
// exists passes through unchanged.
type snapshotGuardStorage struct {
	*raft.MemoryStorage
}

func (s *snapshotGuardStorage) Snapshot() (raftpb.Snapshot, error) {
	snapshot, err := s.MemoryStorage.Snapshot()
	if err != nil {
		return snapshot, err
	}
	if raft.IsEmptySnap(snapshot) {
		return raftpb.Snapshot{}, raft.ErrSnapshotTemporarilyUnavailable
	}
	return snapshot, nil
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

		snapshotCatchUpEntriesN: defaultSnapshotCatchUpEntries,
		snapCount:               defaultSnapshotCount,

		members: maps.Clone(config.Members),

		stopCh:   make(chan struct{}),
		loopDone: make(chan struct{}),
	}

	if config.SnapshotCount != 0 {
		n.snapCount = config.SnapshotCount
	}
	if config.SnapshotCatchUpEntries != 0 {
		n.snapshotCatchUpEntriesN = config.SnapshotCatchUpEntries
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
		Storage:                   &snapshotGuardStorage{n.raftStorage},
		MaxSizePerMsg:             1024 * 1024,
		MaxInflightMsgs:           256,
		MaxUncommittedEntriesSize: 1 << 30,
	}

	if !config.Join {
		// StartNode writes one conf change per peer at the head of the log.
		// Sort the peers so that every initial member writes the same entries
		// in the same order. Attach the node IDs so that a member that joins
		// later and replays the log can resolve every initial member, even
		// one that has since been removed and is not in its member table. A
		// member that joins after these entries were compacted gets the
		// member table from the snapshot instead.
		peers := []raft.Peer{}
		for _, sectorNo := range slices.Sorted(maps.Keys(config.Members)) {
			peers = append(peers, raft.Peer{
				ID:      uint64(sectorNo),
				Context: []byte(config.Members[sectorNo].String()),
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
					// Do not return on an error: raft has already moved to the
					// snapshot, so the loop has to go on from it.
					if err := n.applySnapshot(rd.Snapshot); err != nil {
						n.logger.Error("Failed to apply snapshot", "sectorKey", n.sectorKey.String(), "error", err)
					}
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
		// Every AddNode must carry the node ID: a member that replays the
		// log later may not know it.
		if _, ok := contexts[id]; !ok {
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
	if len(entries) == 0 {
		return nil
	}

	// Skip the entries that are already applied, such as those covered by a
	// snapshot applied in the same batch. The entries must continue from the
	// applied index: applying over a gap would skip committed entries.
	firstIndex := entries[0].Index
	if firstIndex > n.appliedIndex+1 {
		return fmt.Errorf("committed entries start at %d, but the applied index is %d", firstIndex, n.appliedIndex)
	}
	skip := n.appliedIndex + 1 - firstIndex
	if skip >= uint64(len(entries)) {
		return nil
	}
	entries = entries[skip:]

	proposals := make([]*proto.ConsensusProposal, 0)

	for _, entry := range entries {
		// Do not return on an unmarshal error below: the applied index has to
		// reach the end of the batch, or every later batch would be refused
		// as leaving a gap.
		switch entry.Type {
		case raftpb.EntryNormal:
			if len(entry.Data) > 0 {
				p := &proto.ConsensusProposal{}
				if err := proto3.Unmarshal(entry.Data, p); err != nil {
					n.logger.Error("Failed to unmarshal committed proposal", "sectorKey", n.sectorKey.String(), "index", entry.Index, "error", err)
					continue
				}
				proposals = append(proposals, p)
			}

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(entry.Data); err != nil {
				n.logger.Error("Failed to unmarshal committed conf change", "sectorKey", n.sectorKey.String(), "index", entry.Index, "error", err)
				continue
			}
			n.mtx.Lock()
			n.confState = *n.raftNode.ApplyConfChange(cc)
			n.mtx.Unlock()

			// Do not return on an error: raft has already applied the change,
			// so returning would only skip the rest of the batch. An error here
			// only leaves this member unable to send messages to that node.
			if err := n.applyConfChangeSingle(cc.Type, cc.NodeID, cc.Context); err != nil {
				n.logger.Error("Failed to apply committed conf change", "sectorKey", n.sectorKey.String(), "error", err)
			}

		case raftpb.EntryConfChangeV2:
			var cc2 raftpb.ConfChangeV2
			if err := cc2.Unmarshal(entry.Data); err != nil {
				n.logger.Error("Failed to unmarshal committed conf change", "sectorKey", n.sectorKey.String(), "index", entry.Index, "error", err)
				continue
			}
			n.mtx.Lock()
			n.confState = *n.raftNode.ApplyConfChange(cc2)
			n.mtx.Unlock()

			for _, change := range cc2.Changes {
				// TODO: Context is used for both AddNode and RemoveNode, but it is only needed for AddNode. We should separate the context for AddNode and RemoveNode in ConfChangeV2.
				// Do not return on an error; see the EntryConfChange case.
				if err := n.applyConfChangeSingle(change.Type, change.NodeID, cc2.Context); err != nil {
					n.logger.Error("Failed to apply committed conf change", "sectorKey", n.sectorKey.String(), "error", err)
				}
			}
		}
	}

	for _, proposal := range proposals {
		// Do not return on an error: the entry is committed and Advance() runs
		// anyway, so returning would skip the rest of the batch for good and
		// leave this member diverged from the group.
		if err := n.handler.ConsensusApplyProposal(proposal); err != nil {
			n.logger.Error("Failed to apply committed proposal", "sectorKey", n.sectorKey.String(), "error", err)
		}
	}

	// Advance the applied index only after the whole batch: a snapshot is
	// taken at this index, so it must not run ahead of the state of the sector.
	n.appliedIndex = entries[len(entries)-1].Index

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

// applySnapshot installs a snapshot received from the leader: the raft
// storage, the member table and the state of the sector. The member table is
// replaced, not merged: the snapshot has the table of the group at the
// snapshot index, and the conf changes before it are no longer replayed.
func (n *Consensus) applySnapshot(snapshot raftpb.Snapshot) error {
	// Raft expects the storage to hold the snapshot from here on, so apply it
	// before anything that can fail on the content.
	if err := n.raftStorage.ApplySnapshot(snapshot); err != nil {
		return err
	}
	n.mtx.Lock()
	n.confState = snapshot.Metadata.ConfState
	n.mtx.Unlock()
	n.snapshotIndex = snapshot.Metadata.Index
	n.appliedIndex = snapshot.Metadata.Index

	wrapper := &proto.ConsensusSnapshot{}
	if err := proto3.Unmarshal(snapshot.Data, wrapper); err != nil {
		return err
	}
	members := make(map[kvsTypes.SectorNo]*types.NodeID, len(wrapper.Members))
	for id, nodeIDProto := range wrapper.Members {
		nodeID, err := types.NewNodeIDFromProto(nodeIDProto)
		if err != nil {
			return err
		}
		members[kvsTypes.SectorNo(id)] = nodeID
	}
	n.mtx.Lock()
	n.members = members
	n.mtx.Unlock()

	if err := n.handler.ConsensusApplySnapshot(wrapper.SectorState); err != nil {
		return err
	}
	n.logger.Debug("Applied snapshot", "sectorKey", n.sectorKey.String(), "index", snapshot.Metadata.Index)

	return nil
}

// buildSnapshotData returns the data of a raft snapshot: the state of the
// sector from the handler, wrapped with the member table.
func (n *Consensus) buildSnapshotData() ([]byte, error) {
	sectorState, err := n.handler.ConsensusGetSnapshot()
	if err != nil {
		return nil, err
	}

	n.mtx.RLock()
	members := make(map[uint64]*proto.NodeID, len(n.members))
	for sectorNo, nodeID := range n.members {
		members[uint64(sectorNo)] = nodeID.Proto()
	}
	n.mtx.RUnlock()

	return proto3.Marshal(&proto.ConsensusSnapshot{
		SectorState: sectorState,
		Members:     members,
	})
}

func (n *Consensus) maybeTriggerSnapshot() error {
	// Trigger a snapshot if the number of applied entries exceeds the threshold
	if n.appliedIndex-n.snapshotIndex <= n.snapCount {
		return nil
	}

	data, err := n.buildSnapshotData()
	if errors.Is(err, ErrSnapshotUnavailable) {
		return nil
	}
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

	n.logger.Debug("Created snapshot", "sectorKey", n.sectorKey.String(), "index", n.appliedIndex, "compactIndex", compactIndex)

	n.snapshotIndex = n.appliedIndex
	return nil
}
