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
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	proto3 "google.golang.org/protobuf/proto"
)

// Snapshot tuning knobs. The environment overrides exist for simulator
// verification runs (spec/kvs/snapshot.md Stage 5): lowering them makes
// management churn alone trigger snapshot/compaction/InstallSnapshot
// frequently, without waiting for a data-plane write load. Production uses
// the defaults.
var (
	defaultSnapCount               = envUint("COLONIO_KVS_SNAP_COUNT", 1000)
	defaultSnapshotCatchUpEntriesN = envUint("COLONIO_KVS_SNAP_CATCHUP", 100)
)

func envUint(name string, fallback uint64) uint64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return fallback
	}
	return parsed
}

const (
	raftTickDuration = 100 * time.Millisecond
	// proposeTimeout bounds raftNode.Propose, which otherwise blocks until a
	// leader exists. A leaderless (quorum-lost) group must not block the
	// caller forever; the sector's retry loop re-proposes pending proposals.
	proposeTimeout = 2 * time.Second
	// promoteCheckTicks controls how often the leader checks whether learners
	// have caught up and can be promoted to voters (in raft ticks).
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

// snapshotGuardStorage wraps MemoryStorage to keep raft from panicking when a
// leader is asked to send a snapshot before the first one has been created
// (maybeTriggerSnapshot fires only after snapCount applied entries). Raft
// reaches maybeSendSnapshot whenever a follower's Next falls outside the
// leader's log and panics on an empty snapshot ("need non-empty snapshot",
// シミュレーション run6, 2026-07-04 で観測). Returning
// ErrSnapshotTemporarilyUnavailable makes raft skip the send instead: before
// the first snapshot the log is still uncompacted, so the follower can catch
// up from entry 1 (or is re-added under a fresh sectorNo by the membership
// manager's reap). Once a snapshot exists it passes through unchanged and
// lagging followers are served via InstallSnapshot.
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

	// mtx protects confState and members: both are written by the consensus
	// loop goroutine (publishEntries / snapshot handling) and read from the
	// AppendNode/RemoveNode goroutines spawned by the sector's retry loop.
	mtx           sync.RWMutex
	confState     raftpb.ConfState
	snapshotIndex uint64
	appliedIndex  uint64

	members map[kvsTypes.SectorNo]*types.NodeID
}

func NewConsensus(config *Config) *Consensus {
	n := &Consensus{
		logger:      config.Logger,
		handler:     config.Handler,
		outbound:    config.Outbound,
		sectorKey:   *config.SectorKey,
		raftStorage: raft.NewMemoryStorage(),

		snapshotCatchUpEntriesN: defaultSnapshotCatchUpEntriesN,
		snapCount:               defaultSnapCount,

		members: maps.Clone(config.Members),
	}

	raftConfig := &raft.Config{
		Logger:        config.RaftLogger,
		ID:            uint64(config.SectorKey.SectorNo),
		ElectionTick:  10,
		HeartbeatTick: 1,
		// Without CheckQuorum a leader whose followers all died stays leader
		// forever (Status().Lead == self), so the sector's quorum-loss
		// detection never sees the group as leaderless. With CheckQuorum the
		// leader steps down after an election timeout without a quorum of
		// active followers, which also stops its heartbeats and lets the
		// surviving followers observe Lead == 0.
		// (シミュレーション 2026-07-04: leader-without-quorum がリーダー不在
		// 検知をすり抜けて活性化チェーンが停止する事例を観測)
		CheckQuorum:               true,
		Storage:                   &snapshotGuardStorage{n.raftStorage},
		MaxSizePerMsg:             1024 * 1024,
		MaxInflightMsgs:           256,
		MaxUncommittedEntriesSize: 1 << 30,
	}

	if !config.Join {
		// Attach the node ID as the peer context: StartNode synthesizes a
		// ConfChangeAddNode entry per peer, and these entries are replayed from
		// the head of the log by every member that joins before the first
		// compaction (after compaction the joiner instead receives the member
		// table inside the snapshot; see applySnapshot). Without the
		// context, a replaying member can resolve a bootstrap sectorNo only
		// through its own initial member table. That table is the membership
		// manager's CURRENT view at append time, so it no longer contains
		// bootstrap members that were replaced in the meantime, and replacement
		// starts within the first minute of a run (routing-view shifts during
		// ring formation, later churn).
		//
		// The failure chain this context breaks (シミュレーション 2026-07-06,
		// split 修正後の run で観測):
		//
		//  1. A member joins an existing group and catches up from entry 1.
		//     The bootstrap AddNode entry of an already-replaced member cannot
		//     be resolved → applyConfChangeSingle returns "missing node ID in
		//     conf change".
		//  2. publishEntries used to abort the whole batch on that error while
		//     Advance() still ran. For a catching-up member the first batch is
		//     essentially the entire history, so the member permanently lost
		//     every committed entry after the poison one: activate/import
		//     proposals (tail stays nil → replica-state mismatch, simulator
		//     red) and all later conf changes (members table stays near-empty).
		//     → now mitigated independently in publishEntries (log-and-continue).
		//  3. At the raft level the diverged member is healthy — it
		//     acknowledges appends and gets promoted to voter — but with a broken members
		//     table it cannot map sectorNo → nodeID for its peers, so once it
		//     campaigns or wins an election it cannot send a single message
		//     ("Unknown node sectorNo for sending Raft message", 5万件/10min).
		//  4. To every other member the group now looks leaderless; after
		//     forceTerminateDuration (30s) they destroy their replicas one by
		//     one (leaderless force terminate 18→95 件/分 と加速), the manager
		//     re-appends replacements, each replacement replays the same
		//     poisoned history → more diverged members. This positive feedback
		//     degraded the ring from 97/97 active to 72 active within minutes.
		peers := []raft.Peer{}
		for sectorNo, nodeID := range config.Members {
			peers = append(peers, raft.Peer{
				ID:      uint64(sectorNo),
				Context: []byte(nodeID.String()),
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
		tickCount := 0

		for {
			select {
			case <-n.ctx.Done():
				n.raftNode.Stop()
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
					if err := n.applySnapshot(rd.Snapshot); err != nil {
						// Log-and-continue like publishEntries: aborting the
						// loop would stop the replica entirely, while a failed
						// snapshot apply leaves it unsynced until the
						// membership manager reaps and re-adds it.
						n.logger.Error("Failed to apply snapshot", "error", err)
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

func (n *Consensus) Stop() {
	n.raftNode.Stop()
}

// Status returns the current raft status for debugging.
func (n *Consensus) Status() raft.Status {
	return n.raftNode.Status()
}

// TODO: do append and remove in batch using ConfChangeV2
//
// AppendNode adds the node as a LEARNER first (learner-first membership): a
// new member enters the quorum only after it has caught up with the log, so
// appending unsynced or dead nodes can no longer break the group's quorum
// (シミュレーション run4, 2026-07-04: join 波で未同期 voter が蓄積し quorum 喪失
// → 強制破棄ストームとなるのを観測). Promotion to voter is proposed by the
// current leader once the learner's Match reaches the commit index
// (maybePromoteLearners). This method is re-invoked by the sector's retry
// loop until the promotion is applied, so it must be idempotent: proposing
// AddLearnerNode for an id that is already a voter would DEMOTE it, hence the
// config check.
func (n *Consensus) AppendNode(sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	id := uint64(sectorNo)

	n.mtx.RLock()
	inConfig := slices.Contains(n.confState.Voters, id) ||
		slices.Contains(n.confState.Learners, id)
	n.mtx.RUnlock()
	if inConfig {
		// already a learner (waiting for promotion) or a voter
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

// maybePromoteLearners promotes caught-up learners to voters. Runs on the
// consensus loop; only the current leader can observe follower progress.
// Dead or lagging learners are simply never promoted (and are removed later
// by the membership manager when they drop out of the routing view), so they
// never affect the quorum.
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
		// Never propose a promotion without the member's node ID: an
		// empty-context AddNode entry stays in the log until compaction and can
		// only be resolved by replicas that already have the ID in their
		// members table. Every member that joins later replays it, fails, and
		// enters the divergence chain documented at the bootstrap peers in
		// NewConsensus — so a promotion that cannot carry its node ID must not
		// enter the log at all. (Skipping is safe: the promotion is re-proposed
		// by promoteCheckTicks as long as the learner stays caught up, and a
		// learner whose ID the leader does not know cannot be messaged anyway.)
		if _, ok := contexts[id]; !ok {
			continue
		}

		go func(id uint64, context []byte) {
			if err := n.raftNode.ProposeConfChange(n.ctx, raftpb.ConfChangeV2{
				Transition: raftpb.ConfChangeTransitionAuto,
				Changes: []raftpb.ConfChangeSingle{{
					Type:   raftpb.ConfChangeAddNode,
					NodeID: id,
				}},
				Context: context,
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

	// Skip entries already covered by the applied state — e.g. entries that
	// overlap a snapshot applied in the same Ready batch (mirrors etcd's
	// raftexample). A gap above appliedIndex+1 must never happen; applying
	// across it would silently skip committed entries, so refuse the batch.
	firstIndex := entries[0].Index
	if firstIndex > n.appliedIndex+1 {
		return fmt.Errorf("first index of committed entries (%d) leaves a gap above applied index (%d)", firstIndex, n.appliedIndex)
	}
	if n.appliedIndex-firstIndex+1 >= uint64(len(entries)) {
		return nil // every entry is already applied
	}
	entries = entries[n.appliedIndex-firstIndex+1:]

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

			// Log-and-continue, like the proposal loop below: the entry is
			// committed, and aborting here would silently skip the remaining
			// committed entries of the batch (Advance() still runs), leaving
			// this member permanently diverged from the group state. For a
			// catching-up member the first batch is essentially the whole log,
			// so a single unresolvable entry used to cost it every activate/
			// import proposal and every later conf change — step 2 of the
			// divergence chain documented at the bootstrap peers in
			// NewConsensus (シミュレーション 2026-07-06). Note ApplyConfChange
			// already ran above, so the raft-internal state stays consistent
			// regardless of this error.
			if err := n.applyConfChangeSingle(cc.Type, cc.NodeID, cc.Context); err != nil {
				n.logger.Error("Failed to apply committed conf change", "error", err)
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
				// Log-and-continue: same reasoning as the EntryConfChange case
				// above — an unresolvable change must not cost this member the
				// rest of the committed batch.
				if err := n.applyConfChangeSingle(change.Type, change.NodeID, cc2.Context); err != nil {
					n.logger.Error("Failed to apply committed conf change", "error", err)
				}
			}
		}
	}

	for _, proposal := range proposals {
		// Log-and-continue: these entries are already committed by the group,
		// so a failed apply must not abort the rest of the batch. Aborting
		// silently skipped the remaining committed entries (including conf
		// changes) because Advance() still ran, which left this member
		// permanently diverged from the group state.
		// (シミュレーション run3, 2026-07-04: 非冪等な apply が毒エントリー化し、
		// 同一バッチの ConfChange 適用を巻き添えにして新メンバーが永久に
		// 同期しない状態を観測)
		if err := n.handler.ConsensusApplyProposal(proposal); err != nil {
			n.logger.Error("Failed to apply committed proposal", "error", err)
		}
	}

	// Advance appliedIndex only after the whole batch is applied: it is what
	// maybeTriggerSnapshot snapshots at, so it must never run ahead of the
	// state machine (a snapshot taken beyond the applied state would drop the
	// unapplied suffix for every future snapshot-joining member).
	n.appliedIndex = entries[len(entries)-1].Index

	return nil
}

// applyConfChangeSingle updates the member table and notifies the handler for
// one applied conf change. Learner additions only register the nodeID for
// message delivery; the handler (membership manager) is notified when the
// member is promoted to voter, so a member counts as "appended" only once it
// participates in the quorum.
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

// applySnapshot installs a received snapshot: raft storage and metadata,
// the consensus-layer member table, and the sector-layer state (via the
// handler). The member table is REPLACED, not merged — the snapshot's members
// map is the group's authoritative sectorNo→nodeID view at the snapshot
// index, and a member joining via snapshot never replays the compacted
// conf-change entries that would otherwise build it (see spec/kvs/snapshot.md).
func (n *Consensus) applySnapshot(snapshot raftpb.Snapshot) error {
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

	if err := n.raftStorage.ApplySnapshot(snapshot); err != nil {
		return err
	}
	n.mtx.Lock()
	n.confState = snapshot.Metadata.ConfState
	n.members = members
	n.mtx.Unlock()
	n.snapshotIndex = snapshot.Metadata.Index
	n.appliedIndex = snapshot.Metadata.Index

	return n.handler.ConsensusApplySnapshot(wrapper.SectorState)
}

// buildSnapshotData wraps the handler's sector-layer payload with the
// consensus-layer member table into the bytes stored in raftpb.Snapshot.Data.
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

	// the "@@" marker is required: the simulator's log collection only keeps
	// lines containing "==" or "@@" (see the retry log in sector.go)
	fmt.Println(time.Now(), "@@ snapshot compact", n.sectorKey.String(),
		"applied", n.appliedIndex, "compact", compactIndex)

	n.snapshotIndex = n.appliedIndex
	return nil
}
