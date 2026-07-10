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
	"fmt"
	"log/slog"
	"sync"
	"time"

	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	"github.com/llamerada-jp/colonio/node/internal/kvs/activation"
	"github.com/llamerada-jp/colonio/node/internal/kvs/hosting"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector"
	"github.com/llamerada-jp/colonio/node/internal/kvs/sector/consensus"
	"github.com/llamerada-jp/colonio/node/observation"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	"go.etcd.io/raft/v3"
)

type Handler interface {
	KvsGetStability() (bool, []*types.NodeID, []*types.NodeID)
}

type Config struct {
	Logger             *slog.Logger
	EnableRaftLogging  bool
	Handler            Handler
	Outbound           OutboundPort
	ConsensusOutbound  consensus.OutboundPort
	ActivationResolver *activation.Resolver
	HostingManager     *hosting.Manager
	Observation        observation.Caller
	Store              kvsTypes.Store
}

type KVS struct {
	logger             *slog.Logger
	raftLogger         raft.Logger
	ctx                context.Context
	handler            Handler
	outbound           OutboundPort
	consensusOutbound  consensus.OutboundPort
	activationResolver *activation.Resolver
	hostingManager     *hosting.Manager
	observation        observation.Caller
	store              kvsTypes.Store
	localNodeID        *types.NodeID
	// mtx protects sectors, sectorUpdated.
	// Lock order: hosting.Manager's lock may be held when acquiring k.mtx
	// (Manager calls back into KVS), so never call into hostingManager
	// while holding k.mtx.
	mtx     sync.RWMutex
	sectors map[kvsTypes.SectorKey]*sector.Sector
	// sectorTombstones records sector keys whose local replica was terminated
	// or removed. A raft member ID must never be reused with an empty log: the
	// group remembers the ID's acked entries and votes, and re-creating the
	// instance from scratch corrupts the raft state (etcd raft panics inside
	// nextCommittedEnts; シミュレーション run5, 2026-07-04 で観測). Re-delivered
	// SectorManageMember messages for these keys are rejected; the membership
	// manager re-adds the node under a fresh sectorNo instead.
	sectorTombstones          map[kvsTypes.SectorKey]time.Time
	mtxOperateSectors         sync.Mutex
	proposedSplittingNodeID   *types.NodeID
	proposedSplittingSectorID *kvsTypes.SectorID
	observationSectorInfo     map[kvsTypes.SectorKey]*observation.SectorInfo
}

func NewKVS(conf *Config) *KVS {
	k := &KVS{
		logger:             conf.Logger,
		handler:            conf.Handler,
		outbound:           conf.Outbound,
		consensusOutbound:  conf.ConsensusOutbound,
		activationResolver: conf.ActivationResolver,
		hostingManager:     conf.HostingManager,
		observation:        conf.Observation,
		store:              conf.Store,
		sectors:            make(map[kvsTypes.SectorKey]*sector.Sector),
		sectorTombstones:   make(map[kvsTypes.SectorKey]time.Time),
	}

	if conf.EnableRaftLogging {
		k.raftLogger = newSlogWrapper(conf.Logger)
	} else {
		k.raftLogger = newEmptyLogger()
	}

	return k
}

func (k *KVS) Start(ctx context.Context, localNodeID *types.NodeID) {
	k.localNodeID = localNodeID
	k.ctx = ctx

	k.hostingManager.Start(k, localNodeID)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				k.subRoutine()
				if k.observation != nil {
					k.takeObservation()
				}
			}
		}
	}()
}

func (k *KVS) Get(key string) chan *kvsTypes.GetResult {
	c := make(chan *kvsTypes.GetResult, 1)
	k.outbound.sendKvsOperation(&operationParam{
		command: proto.KvsOperation_COMMAND_GET,
		key:     key,
		value:   nil,
		receiver: func(res *proto.KvsOperationResponse, err error) {
			defer close(c)

			if err != nil {
				c <- &kvsTypes.GetResult{
					Data: nil,
					Err:  err,
				}
				return
			}

			switch res.Error {
			case proto.KvsOperationResponse_ERROR_NONE:
				// ok
				c <- &kvsTypes.GetResult{
					Data: res.Value,
					Err:  nil,
				}

			case proto.KvsOperationResponse_ERROR_NOT_FOUND:
				c <- &kvsTypes.GetResult{
					Data: nil,
					Err:  fmt.Errorf("key not found: %s", key),
				}

			default:
				c <- &kvsTypes.GetResult{
					Data: nil,
					Err:  fmt.Errorf("unknown error: %d", res.Error),
				}
			}
		},
	})

	return c
}

func (k *KVS) Set(key string, value []byte) chan error {
	c := make(chan error, 1)
	k.outbound.sendKvsOperation(&operationParam{
		command: proto.KvsOperation_COMMAND_SET,
		key:     key,
		value:   value,
		receiver: func(res *proto.KvsOperationResponse, err error) {
			defer close(c)

			if err != nil {
				c <- err
				return
			}

			if res.Error != proto.KvsOperationResponse_ERROR_NONE {
				c <- fmt.Errorf("kvs set error: %d", res.Error)
				return
			}

			c <- nil
		},
	})

	return c
}

func (k *KVS) Patch(key string, value []byte) chan error {
	c := make(chan error, 1)
	k.outbound.sendKvsOperation(&operationParam{
		command: proto.KvsOperation_COMMAND_PATCH,
		key:     key,
		value:   value,
		receiver: func(res *proto.KvsOperationResponse, err error) {
			defer close(c)

			if err != nil {
				c <- err
				return
			}

			if res.Error != proto.KvsOperationResponse_ERROR_NONE {
				c <- fmt.Errorf("kvs patch error: %d", res.Error)
				return
			}

			c <- nil
		},
	})

	return c
}

func (k *KVS) Delete(key string) chan error {
	c := make(chan error, 1)
	k.outbound.sendKvsOperation(&operationParam{
		command: proto.KvsOperation_COMMAND_DELETE,
		key:     key,
		value:   nil,
		receiver: func(res *proto.KvsOperationResponse, err error) {
			defer close(c)

			if err != nil {
				c <- err
				return
			}

			if res.Error != proto.KvsOperationResponse_ERROR_NONE {
				c <- fmt.Errorf("kvs delete error: %d", res.Error)
				return
			}

			c <- nil
		},
	})

	return c
}

func (k *KVS) kvsOperate(command proto.KvsOperation_Command, key string, value []byte) (proto.KvsOperationResponse_Error, []byte) {
	hostingSectorKey := k.hostingManager.GetHostingSectorKey()
	if hostingSectorKey == nil {
		k.logger.Debug("preparing hosting sector key")
		return proto.KvsOperationResponse_ERROR_PREPARING, nil
	}

	k.mtx.RLock()
	hostingSector := k.sectors[*hostingSectorKey]
	k.mtx.RUnlock()
	if hostingSector == nil {
		k.logger.Debug("preparing hosting sector")
		return proto.KvsOperationResponse_ERROR_PREPARING, nil
	}
	operator := hostingSector.GetOperator()

	switch command {
	case proto.KvsOperation_COMMAND_GET:
		data, err := operator.Get(key)

		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			if err == kvsTypes.ErrorStoreKeyNotFound {
				e = proto.KvsOperationResponse_ERROR_NOT_FOUND
			} else {
				e = proto.KvsOperationResponse_ERROR_UNKNOWN
			}
		}
		return e, data

	case proto.KvsOperation_COMMAND_SET:
		err := operator.Set(key, value)
		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			e = proto.KvsOperationResponse_ERROR_UNKNOWN
		}
		return e, nil

	case proto.KvsOperation_COMMAND_PATCH:
		err := operator.Patch(key, value)
		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			e = proto.KvsOperationResponse_ERROR_UNKNOWN
		}
		return e, nil

	case proto.KvsOperation_COMMAND_DELETE:
		err := operator.Delete(key)
		e := proto.KvsOperationResponse_ERROR_NONE
		if err != nil {
			e = proto.KvsOperationResponse_ERROR_UNKNOWN
		}
		return e, nil

	default:
		return proto.KvsOperationResponse_ERROR_UNKNOWN, nil
	}
}

func (k *KVS) subRoutine() {
	nodeIsStable, backwardNextNodeIDs, frontwardNextNodeIDs := k.handler.KvsGetStability()
	if !nodeIsStable {
		fmt.Println(time.Now(), k.localNodeID.String(), "== skip subRoutine: node is not stable")
		return
	}
	// The first element of frontwardNextNodeIDs is the next node in the frontward direction.
	nextNodeIDs := append(frontwardNextNodeIDs, backwardNextNodeIDs...)
	var frontwardNextNodeID *types.NodeID
	if len(nextNodeIDs) > 0 {
		frontwardNextNodeID = nextNodeIDs[0]
	}

	sectorIsStable := k.hostingManager.ManageMember(nextNodeIDs)

	// Try to operate sectors by one thread to avoid race.
	locked := k.mtxOperateSectors.TryLock()
	if !locked {
		fmt.Println(time.Now(), k.localNodeID.String(), "== skip subRoutine: previous operateSectors is still running")
		return
	}
	go func() {
		defer k.mtxOperateSectors.Unlock()

		hostingSectorKey := k.hostingManager.GetHostingSectorKey()
		if hostingSectorKey == nil {
			fmt.Println(time.Now(), k.localNodeID.String(), "== skip operateSectors: hosting sector key is not ready")
			return
		}
		k.mtx.RLock()
		hostingSector := k.sectors[*hostingSectorKey]
		k.mtx.RUnlock()
		if hostingSector == nil {
			fmt.Println(time.Now(), k.localNodeID.String(), "== skip operateSectors: hosting sector is not ready")
			return
		}

		k.operateSectors(hostingSector, nextNodeIDs, frontwardNextNodeID, sectorIsStable)
	}()
}

func (k *KVS) operateSectors(hostingSector *sector.Sector, nextNodeIDs []*types.NodeID, frontwardNextNodeID *types.NodeID, sectorIsStable bool) {
	if !k.proposedSplitRoutine(hostingSector, nextNodeIDs) {
		fmt.Println(time.Now(), k.localNodeID.String(), "== skip operateSectors: proposed splitting is not resolved")
		return
	}

	hostingSectorIsActive := false
	if hostingSector.GetTailAddress() != nil {
		hostingSectorIsActive = true
	}
	hostingSectorState := kvsTypes.SectorStateInactive
	if hostingSectorIsActive {
		hostingSectorState = kvsTypes.SectorStateActive
	}
	if err := k.activationResolver.SetSectorState(k.ctx, hostingSectorState); err != nil {
		k.logger.Warn("Failed to set sector state to active", "error", err)
	}

	// If there is any management proposal, skip operating sectors until the proposal is resolved
	// because the proposal may change the condition of sectors.
	// NOTE: TLA+ AbortProposal により、提案対象が離脱した場合のキャンセル処理が必要。
	// splitSector: proposedSplitRoutine がターゲット離脱を検知して Terminate で解消。
	// activateFrontwardSector: 呼び出し側は proposal をセットしないため stuck しない。
	// mergeSector: TODO — ターゲット離脱時の abort 処理を確認・追加する。
	// NOTE: シミュレーション (100ノード・ランダム停止, 2026-07-04) で、離脱検知後の
	// Terminate も raft コミット必須のため quorum 喪失グループでは完了せず、
	// 「離脱を検知しても解消できない」ケースを観測。abort 処理は raft を経由しない
	// ローカル破棄とセットで設計する必要がある。
	// → sector.checkQuorumLoss (リーダー不在の継続または pending proposal の
	// commit 停滞で raft を経由せずローカル破棄) を実装 (2026-07-04)。破棄されると
	// SectorTerminated 経由でセクターが再作成され、pending proposal ごと解消される。
	if hostingSector.HasManagementProposal() {
		fmt.Println(time.Now(), k.localNodeID.String(), "== skip operateSectors: hosting sector has management proposal")
		return
	}

	if !hostingSectorIsActive {
		if sectorIsStable {
			// Haven't activated sector yet, try to activate if the sector is stable.
			k.activateHostingSector(hostingSector, nextNodeIDs, true)
		} else {
			fmt.Println(time.Now(), k.localNodeID.String(), "== skip activation: sector is not stable")
		}
		return
	}

	frontwardNextSector, frontwardNodeMatch := k.getFrontwardCondition(frontwardNextNodeID)
	// Skip until the frontward sector will be created.
	if frontwardNextSector == nil {
		frontwardNextNodeIDStr := "<nil>"
		if frontwardNextNodeID != nil {
			frontwardNextNodeIDStr = frontwardNextNodeID.String()
		}
		fmt.Println(time.Now(), k.localNodeID.String(), "== skip operateSectors: frontward next sector is not created yet, frontwardNextNodeID:", frontwardNextNodeIDStr)
		return
	}

	hostingSectorTail := hostingSector.GetTailAddress()

	// The sector is inactive.
	if frontwardNextSector.GetTailAddress() == nil {
		if !frontwardNodeMatch {
			// Terminate the frontward sector.
			// NOTE: シミュレーションで、frontward レプリカのグループが quorum を失って
			// いると Terminate がコミットされず、ここを毎秒通り続けることを観測 (2026-07-04)。
			// → quorum 喪失時は sector.checkQuorumLoss の強制破棄が脱出経路になる。
			fmt.Println(time.Now(), k.localNodeID.String(), "@@ Terminate frontward next sector")
			frontwardNextSector.Terminate()
			return
		}

		switch {
		case frontwardNextSector.GetHeadAddress().Equal(hostingSectorTail):
			// frontwardNextSector.GetHeadAddress() == hostingSectorTail
			// Activate frontward next sector.
			fmt.Println(time.Now(), k.localNodeID.String(), "@@ Activate frontward next sector", frontwardNextSector.GetHeadAddress().String())
			k.activateFrontwardSector(frontwardNextSector)
		case frontwardNextSector.GetHeadAddress().IsBetween(k.localNodeID, hostingSectorTail):
			// frontwardNextSector.GetHeadAddress() is inside the hosting sector range [local, hostingSectorTail)
			// Split hosting sector and make frontward next sector active.
			fmt.Println(time.Now(), k.localNodeID.String(), "@@ Split hosting sector and make frontward next sector active")
			k.splitSector(hostingSector, frontwardNextSector)
		default:
			// frontwardNextSector.GetHeadAddress() is outside the hosting sector range (past hostingSectorTail on the ring)
			// Extend hosing sector to frontward next sector.
			if k.hasActiveSectorHeadInRange(hostingSectorTail, frontwardNextSector.GetHeadAddress()) {
				fmt.Println(time.Now(), k.localNodeID.String(), "== skip Extend 1: active sector head exists in range")
				return
			}
			fmt.Println(time.Now(), k.localNodeID.String(), "@@ Extend 1")
			if err := hostingSector.Extend(*frontwardNextSector.GetHeadAddress()); err != nil {
				fmt.Println(time.Now(), k.localNodeID.String(), "== Extend 1 failed:", err)
				k.logger.Warn("Failed to extend sector", "error", err)
			}
		}
		return
	}

	// The sector is active & frontward node is not match.
	if !frontwardNodeMatch {
		switch {
		case frontwardNextSector.GetHeadAddress().IsBetween(k.localNodeID, hostingSectorTail):
			// frontwardNextSector.GetHeadAddress() is inside the hosting sector range [local, hostingSectorTail)
			// Terminate both of hosting sector and frontward next sector.
			fmt.Println(time.Now(), k.localNodeID.String(), "@@ Terminate hosting sector 1")
			hostingSector.Terminate()
			fmt.Println(time.Now(), k.localNodeID.String(), "@@ Terminate frontward next sector 1")
			frontwardNextSector.Terminate()
		default:
			// frontwardNextSector.GetHeadAddress() is at or past hostingSectorTail on the ring
			// Merge the hosting sector with frontward sector and delete the frontward sector.
			fmt.Println(time.Now(), k.localNodeID.String(), "@@ Merge the hosting sector with frontward sector and delete the frontward sector")
			k.mergeSector(hostingSector, frontwardNextSector)
		}
		return
	}

	// NOTE: TLA+ Raft モデルでは TerminateB（このパス: frontwardNodeMatch=true）が
	// 実際に発火する (N=3: 8回, N=4: 14回)。Propose→Commit 間のインターリービングで
	// CommitActivate/CommitSplit が古い tail を適用し、セクター重複が一時的に発生する。
	// このパスが重複解消の主要な防御ラインであることが形式検証で確認された。
	// NOTE: TerminateB で inactive にされるノードが proposing (提案中) の場合、
	// そのノードの proposing 状態をクリアする必要がある。さもないと CommitMerge 等の
	// 後続 commit が不正に発火し、anyActive フラグの不整合を引き起こす
	// (TLA+ MaxChurn=3 で検出: ActiveFlagConsistent 違反)。
	switch {
	case frontwardNextSector.GetHeadAddress().Equal(hostingSectorTail):
		// frontwardNextSector.GetHeadAddress() == hostingSectorTail
		// Just a normal case, nothing to do.
	case frontwardNextSector.GetHeadAddress().IsBetween(k.localNodeID, hostingSectorTail):
		// frontwardNextSector.GetHeadAddress() is inside the hosting sector range [local, hostingSectorTail)
		// Terminate both of hosting sector and frontward next sector.
		fmt.Println(time.Now(), k.localNodeID.String(), "@@ Terminate hosting sector 2")
		hostingSector.Terminate()
		fmt.Println(time.Now(), k.localNodeID.String(), "@@ Terminate frontward next sector 1")
		frontwardNextSector.Terminate()
	default:
		// frontwardNextSector.GetHeadAddress() is past hostingSectorTail on the ring
		// Extend hosing sector to frontward next sector.
		if k.hasActiveSectorHeadInRange(hostingSectorTail, frontwardNextSector.GetHeadAddress()) {
			fmt.Println(time.Now(), k.localNodeID.String(), "== skip Extend 2: active sector head exists in range")
			return
		}
		fmt.Println(time.Now(), k.localNodeID.String(), "@@ Extend 2")
		if err := hostingSector.Extend(*frontwardNextSector.GetHeadAddress()); err != nil {
			fmt.Println(time.Now(), k.localNodeID.String(), "== Extend 2 failed:", err)
			k.logger.Warn("Failed to extend sector", "error", err)
		}
	}
}

// getFrontwardCondition returns the frontward next sector and whether the frontward next node is match or not.
func (k *KVS) getFrontwardCondition(frontwardNextNodeID *types.NodeID) (*sector.Sector, bool) {
	var frontwardNextSector *sector.Sector
	var minimumSector *sector.Sector
	k.mtx.RLock()
	for _, sector := range k.sectors {
		if sector.GetHeadAddress().Equal(k.localNodeID) {
			continue
		}
		// find the frontward sector if exists, otherwise find the minimum sector.
		if k.localNodeID.Smaller(sector.GetHeadAddress()) {
			if frontwardNextSector == nil || sector.GetHeadAddress().Smaller(frontwardNextSector.GetHeadAddress()) {
				frontwardNextSector = sector
			}
		} else {
			if minimumSector == nil || sector.GetHeadAddress().Smaller(minimumSector.GetHeadAddress()) {
				minimumSector = sector
			}
		}
	}
	k.mtx.RUnlock()
	if frontwardNextSector == nil {
		frontwardNextSector = minimumSector
	}

	// local node is stable and other nodes are not exist
	if frontwardNextNodeID == nil {
		// if there is any sector, not match
		return frontwardNextSector, frontwardNextSector == nil
	}

	if frontwardNextSector == nil {
		return nil, false
	}

	if frontwardNextSector.GetHeadAddress().IsBetween(k.localNodeID, frontwardNextNodeID) {
		return frontwardNextSector, false
	} else if frontwardNextSector.GetHeadAddress().Equal(frontwardNextNodeID) {
		return frontwardNextSector, true
	} else {
		return nil, false
	}
}

// hasActiveSectorHeadInRange checks whether any active sector's head exists in the range (from, to) on the ring.
// This guard prevents Extend from creating an overlap with an already-active sector when the local
// sector-store view (k.sectors) is stale relative to routing. Without this, TerminateB would
// eventually resolve the overlap, but this proactive check avoids the transient inconsistency.
// (Derived from TLA+ KvsSectorSepView: Extend precondition \A other \in Actives : ~IsBetween(other, n, newTail))
func (k *KVS) hasActiveSectorHeadInRange(from, to *types.NodeID) bool {
	k.mtx.RLock()
	defer k.mtx.RUnlock()
	for _, s := range k.sectors {
		head := s.GetHeadAddress()
		if head.Equal(k.localNodeID) || head.Equal(to) {
			continue
		}
		if s.GetTailAddress() != nil && head.IsBetween(from, to) {
			return true
		}
	}
	return false
}

func (k *KVS) allocateSector(
	sectorKey *kvsTypes.SectorKey,
	head *types.NodeID,
	isHosting bool,
	append bool,
	members map[kvsTypes.SectorNo]*types.NodeID,
) {
	s := sector.NewSector(&sector.SectorConfig{
		Logger:     k.logger,
		RaftLogger: k.raftLogger,
		Handler:    k,
		Outbound:   k.consensusOutbound,
		SectorKey:  sectorKey,
		IsHosting:  isHosting,
		Join:       append,
		Members:    members,
		Store:      k.store,
		Head:       head,
	})

	k.sectors[*sectorKey] = s

	s.Start(k.ctx)
}

type sectorManageMemberParam struct {
	command   proto.SectorManageMember_Command
	sectorKey kvsTypes.SectorKey
	head      *types.NodeID
	members   map[kvsTypes.SectorNo]*types.NodeID
}

func (k *KVS) sectorManageMember(param *sectorManageMemberParam) error {
	// Fetch before taking k.mtx: the established lock order is the hosting
	// manager's mutex first, then k.mtx (ManageMember → HostingAllocateSector);
	// taking them in the opposite order here could deadlock.
	hostingSectorKey := k.hostingManager.GetHostingSectorKey()

	k.mtx.Lock()
	defer k.mtx.Unlock()

	switch param.command {
	case proto.SectorManageMember_COMMAND_CREATE, proto.SectorManageMember_COMMAND_APPEND:
		// Never resurrect a terminated/removed member with an empty log: the
		// group still remembers this raft ID (progress, votes), and a fresh
		// instance under the same ID corrupts the raft state. The setting
		// message is re-sent every second while the member is not Normal, so
		// a re-delivery after a local force-terminate is common. Rejecting it
		// leaves the member non-Normal on the manager side, which re-adds the
		// node under a fresh sectorNo after memberSetupTimeout.
		if _, ok := k.sectorTombstones[param.sectorKey]; ok {
			return fmt.Errorf("sector key is tombstoned: %s", param.sectorKey.String())
		}
		if _, ok := k.sectors[param.sectorKey]; !ok {
			append := true
			if param.command == proto.SectorManageMember_COMMAND_CREATE {
				append = false
			}
			k.allocateSector(&param.sectorKey, param.head, false, append, param.members)
		}

	case proto.SectorManageMember_COMMAND_REMOVE:
		// Out-of-band removal notification from the group's host: this member
		// was removed by a committed conf change, but a removed member no
		// longer receives anything from the group, so the notification is the
		// only way to learn it promptly (the fallback is the 30s leaderless
		// force terminate). Destroy the replica locally — never via raft, the
		// group would ignore proposals from a removed member.
		//
		// A node never removes its own host slot from its hosting sector
		// (getNodesToBeChanged skips HostNodeSectorNo), so a REMOVE targeting
		// the local hosting sector is bogus (stale or misdirected): terminating
		// it here would let a single unauthenticated packet kill an active
		// sector.
		if hostingSectorKey != nil && *hostingSectorKey == param.sectorKey {
			return fmt.Errorf("reject removing the hosting sector: %s", param.sectorKey.String())
		}

		// Tombstone even when the replica does not exist locally (already
		// destroyed, or the notification raced ahead of the setting message):
		// the group has committed the removal, so this member ID must never be
		// (re)created on this node.
		k.addSectorTombstoneLocked(param.sectorKey)

		sector, ok := k.sectors[param.sectorKey]
		if !ok {
			return nil
		}
		// Same flow as the quorum-loss force terminate: TerminateLocally
		// releases the store sector and fires SectorTerminated, which stops the
		// raft loop and deletes the map entry. The entry must stay in k.sectors
		// until then — SectorTerminated returns early (skipping Stop) when the
		// key is already gone. Run it off this goroutine: it takes the sector
		// lock and k.mtx is held here.
		go sector.TerminateLocally()

	default:
		return fmt.Errorf("unknown SectorManageMember command: %d", param.command)
	}

	return nil
}

func (k *KVS) sectorActivate(srcNodeID *types.NodeID, sectorID kvsTypes.SectorID) bool {
	fmt.Println(time.Now(), k.localNodeID.String(), "== receive sectorActivate from", srcNodeID.String(), "sectorID:", sectorID.String())
	hostingSectorKey := k.hostingManager.GetHostingSectorKey()

	// ignore if it does not match the current node
	if hostingSectorKey == nil || hostingSectorKey.SectorID != sectorID {
		fmt.Println(time.Now(), k.localNodeID.String(), "== reject sectorActivate: hosting sector key is nil or sectorID mismatch")
		return false
	}

	// skip if there isn't hosing sector
	k.mtx.RLock()
	hostingSector, ok := k.sectors[*hostingSectorKey]
	k.mtx.RUnlock()
	if !ok {
		fmt.Println(time.Now(), k.localNodeID.String(), "== reject sectorActivate: hosting sector is not ready")
		return false
	}

	// already activated
	if hostingSector.GetTailAddress() != nil {
		fmt.Println(time.Now(), k.localNodeID.String(), "== sectorActivate: already activated")
		return true
	}

	nodeIsStable, _, frontwardNextNodeIDs := k.handler.KvsGetStability()
	if !nodeIsStable {
		fmt.Println(time.Now(), k.localNodeID.String(), "== reject sectorActivate: node is not stable")
		return false
	}
	k.activateHostingSector(hostingSector, frontwardNextNodeIDs, false)

	return true
}

func (k *KVS) sectorPrepareSplit(srcNodeID *types.NodeID, sectorID kvsTypes.SectorID) bool {
	locked := k.mtxOperateSectors.TryLock()
	if !locked {
		fmt.Println(time.Now(), k.localNodeID.String(), "== reject sectorPrepareSplit from", srcNodeID.String(), ": operateSectors is running")
		return false
	}
	defer k.mtxOperateSectors.Unlock()

	if k.proposedSplittingNodeID.Equal(srcNodeID) &&
		k.proposedSplittingSectorID != nil && *k.proposedSplittingSectorID == sectorID {
		fmt.Println(time.Now(), k.localNodeID.String(), "== accept sectorPrepareSplit from", srcNodeID.String(), "(already accepted)")
		return true
	}

	// The hosting sector key is nil between a (force) termination of the
	// hosting sector and its re-creation by ManageMember; an inbound
	// prepare-split in that window must simply be rejected
	// (シミュレーション run7, 2026-07-04: この窓での nil 参照で panic).
	hostingSectorKey := k.hostingManager.GetHostingSectorKey()
	if k.proposedSplittingNodeID != nil ||
		hostingSectorKey == nil || hostingSectorKey.SectorID != sectorID {
		fmt.Println(time.Now(), k.localNodeID.String(), "== reject sectorPrepareSplit from", srcNodeID.String(), ": another proposal or sectorID mismatch")
		return false
	}

	k.proposedSplittingNodeID = srcNodeID
	k.proposedSplittingSectorID = &sectorID

	fmt.Println(time.Now(), k.localNodeID.String(), "== accept sectorPrepareSplit from", srcNodeID.String())
	return true
}

// proposedSplitRoutine is called periodically to check whether the proposed splitting can be finished or not.
// it returns true if the proposed splitting is finished or there is no proposed splitting, otherwise false.
func (k *KVS) proposedSplitRoutine(hostingSector *sector.Sector, nextNodeIDs []*types.NodeID) bool {
	if k.proposedSplittingNodeID == nil {
		return true
	}

	// split is proposed but the hosting sector is changed, cancel the proposed splitting.
	if hostingSector.GetKey().SectorID != *k.proposedSplittingSectorID {
		k.proposedSplittingNodeID = nil
		k.proposedSplittingSectorID = nil
		return true
	}

	proposedNodeExists := false
	for _, n := range nextNodeIDs {
		if n.Equal(k.proposedSplittingNodeID) {
			proposedNodeExists = true
			break
		}
	}
	// If the proposed node is not exist, terminate the hosting sector
	// because the proposed splitting cannot be finished.
	// NOTE: シミュレーションで、proposer 離脱後にこの Terminate が hosting sector の
	// quorum 喪失によりコミットされず、毎秒ここを通り続けるケースを観測 (2026-07-04)。
	// 離脱検知だけでは不十分で、raft を経由しないローカル破棄の脱出経路が必要。
	// → sector.checkQuorumLoss として実装 (2026-07-04)。破棄で hosting sector が
	// 再作成されると sectorID が変わり、上の分岐で proposedSplitting も解消される。
	if !proposedNodeExists {
		fmt.Println(time.Now(), k.localNodeID.String(), "== proposedSplitRoutine: proposer", k.proposedSplittingNodeID.String(), "left, terminate hosting sector")
		hostingSector.Terminate()
	}

	// Finish splitting normally.
	if hostingSector.GetTailAddress() != nil {
		k.proposedSplittingNodeID = nil
		k.proposedSplittingSectorID = nil
		return true
	}

	return false
}

func (k *KVS) processConsensusMessage(key kvsTypes.SectorKey, message *proto.ConsensusMessage) {
	k.mtx.RLock()
	sector, ok := k.sectors[key]
	k.mtx.RUnlock()
	if !ok {
		return
	}

	if err := sector.ProcessConsensusMessage(message); err != nil {
		k.logger.Error("Failed to process Raft message", "error", err)
	}
}

func (k *KVS) SectorError(sectorKey *kvsTypes.SectorKey, err error) {
	// TODO: It may be necessary to identify the cause of the error and stop the node.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return // shutting down
	}
	k.logger.Error("sector error",
		"sectorKey", sectorKey.String(),
		"error", err)
}

func (k *KVS) SectorAppendNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	k.hostingManager.OnSectorAppendNode(sectorKey, sectorNo, nodeID)
}

func (k *KVS) SectorRemoveNode(sectorKey *kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo) {
	k.hostingManager.OnSectorRemoveNode(sectorKey, sectorNo)

	k.mtx.Lock()
	defer k.mtx.Unlock()

	targetKey := kvsTypes.SectorKey{
		SectorID: sectorKey.SectorID,
		SectorNo: sectorNo,
	}
	// The removal is committed by the group: this member ID must never be
	// re-created on this node, even if it never held the replica locally.
	k.addSectorTombstoneLocked(targetKey)

	sector, ok := k.sectors[targetKey]
	if !ok {
		return
	}

	delete(k.sectors, targetKey)

	go sector.Stop()
}

func (k *KVS) SectorTerminated(sectorKey *kvsTypes.SectorKey) {
	k.hostingManager.OnSectorTerminated(sectorKey)

	k.mtx.Lock()
	defer k.mtx.Unlock()

	k.addSectorTombstoneLocked(*sectorKey)

	sector, ok := k.sectors[*sectorKey]
	if !ok {
		return
	}

	go sector.Stop()
	delete(k.sectors, *sectorKey)
}

// addSectorTombstoneLocked marks a sector key as never-recreatable and prunes
// old entries. A tombstone is only needed while the group can still address
// the old member ID; expired entries are dropped to bound the map size.
// Call with k.mtx write-locked.
func (k *KVS) addSectorTombstoneLocked(sectorKey kvsTypes.SectorKey) {
	const tombstoneRetention = 30 * time.Minute

	now := time.Now()
	for key, at := range k.sectorTombstones {
		if now.Sub(at) > tombstoneRetention {
			delete(k.sectorTombstones, key)
		}
	}
	k.sectorTombstones[sectorKey] = now
}

// HostingAllocateSector implements hosting.SectorHandler.
func (k *KVS) HostingAllocateSector(sectorKey *kvsTypes.SectorKey, head *types.NodeID, isHosting bool, join bool, members map[kvsTypes.SectorNo]*types.NodeID) {
	k.mtx.Lock()
	defer k.mtx.Unlock()
	k.allocateSector(sectorKey, head, isHosting, join, members)
}

// HostingApplyAppendNode implements hosting.SectorHandler.
func (k *KVS) HostingApplyAppendNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo, nodeID *types.NodeID) {
	k.mtx.RLock()
	s, ok := k.sectors[sectorKey]
	k.mtx.RUnlock()
	if ok {
		s.AppendNode(sectorNo, nodeID)
	}
}

// HostingApplyRemoveNode implements hosting.SectorHandler.
func (k *KVS) HostingApplyRemoveNode(sectorKey kvsTypes.SectorKey, sectorNo kvsTypes.SectorNo) {
	k.mtx.RLock()
	s, ok := k.sectors[sectorKey]
	k.mtx.RUnlock()
	if ok {
		s.RemoveNode(sectorNo)
	}
}

// SendSectorManageMember implements hosting.OutboundPort.
func (k *KVS) SendSectorManageMember(param *hosting.SectorManageMemberParam) {
	k.outbound.sendSectorManageMember(&SectorManageMemberParam{
		dstNodeID: param.DstNodeID,
		sectorID:  param.SectorID,
		sectorNo:  param.SectorNo,
		command:   param.Command,
		members:   param.Members,
	})
}

func (k *KVS) activateHostingSector(hostingSector *sector.Sector, frontwardNextNodeIDs []*types.NodeID, checkEntireState bool) {
	// NOTE: TLA+ では ActivateFirst アクションに相当する。
	// anyActive=FALSE (リング上に active セクターが一つもない) でのみ発火する
	// ため原子的にモデル化している。Go 実装も checkEntireState=true の値で
	// EntireStateInactive を確認してから Activate するため同じ保証がある。
	// checkEntireState=false (sectorActivate 経由) のパスは他ノードが既に
	// active である前提なので、ActivateFrontward (Propose+Commit) に相当する。
	if checkEntireState {
		entireState, err := k.activationResolver.ResolveEntireState(k.ctx)
		if err != nil {
			fmt.Println(time.Now(), k.localNodeID.String(), "== skip activation: failed to resolve entire state:", err)
			k.logger.Warn("Failed to get sector entire state", "error", err)
			return
		}

		// other node might have already activated the sector, check the state again
		if entireState != kvsTypes.EntireStateInactive {
			fmt.Println(time.Now(), k.localNodeID.String(), "@@ Entire not inactive", entireState)
			return
		}
	}

	fmt.Println(time.Now(), k.localNodeID.String(), "== Activating hosting sector", checkEntireState)

	// Decide the tail under RLock, then activate after releasing it.
	// Activate/markSectorUpdated must not run while k.mtx is held.
	tail := func() *types.NodeID {
		k.mtx.RLock()
		defer k.mtx.RUnlock()

		// local node is stable and other nodes are not exist
		if len(frontwardNextNodeIDs) == 0 {
			for sectorKey := range k.sectors {
				if sectorKey != hostingSector.GetKey() {
					k.logger.Warn("No frontward node but other sectors exist")
					return nil
				}
			}
			return k.localNodeID
		}

		var candidate *sector.Sector
		frontwardNodeID := frontwardNextNodeIDs[0]
		fmt.Println(time.Now(), k.localNodeID.String(), "== frontwardNodeID", frontwardNodeID.String())
		// blocker is the nearest ACTIVE sector head inside (local, frontwardNodeID)
		// (TLA+ KvsSectorLeftover: CommitActivate / ActivateFirst の blockers)。
		// Active-only: inactive replicas of departed nodes can remain in
		// k.sectors for a while; if they are counted here, activation may be
		// blocked forever because inactive replicas do not self-heal.
		var blocker *types.NodeID
		for sectorKey, sector := range k.sectors {
			sectorHead := sector.GetHeadAddress()
			fmt.Println(time.Now(), k.localNodeID.String(), "== check sector", sectorKey.String(), "head", sectorHead.String())
			if sectorHead.Equal(k.localNodeID) {
				continue
			}
			if sectorHead.Equal(frontwardNodeID) {
				if candidate == nil || sectorKey.SectorNo > candidate.GetKey().SectorNo {
					candidate = sector
				}
			}

			if sectorHead.IsBetween(k.localNodeID, frontwardNodeID) && sector.GetTailAddress() != nil {
				if blocker == nil || sectorHead.IsBetween(k.localNodeID, blocker) {
					blocker = sectorHead
				}
			}
		}
		// An active sector between the local node and the frontward node is
		// typically the leftover of a dead host, kept alive by its healthy
		// replica group. Activation used to be rejected here ("skip 1") to
		// avoid overlap — but the only sector positioned to merge the leftover
		// away is this one, and merge requires it to be ACTIVE first: a
		// circular wait that froze the activation chain until the leftover's
		// group happened to lose quorum (シミュレーション run 11/13 で観測、
		// run 13 では 14 分停止。TLA+ KvsSectorLeftover Phase L1 の liveness
		// 違反として再現)。Instead, activate with the tail clipped to the
		// nearest blocker: [local, blocker) does not overlap anything, and
		// once this sector is active the standard merge path (with the
		// ReleaseMerge backstop) absorbs the leftover and extends the tail
		// (Phase L2 で liveness 回復、Phase L3 で誤検知 release との併発
		// safety を検証済み)。
		if blocker != nil {
			fmt.Println(time.Now(), k.localNodeID.String(), "@@ Activate hosting sector clipped to", blocker.String(), "instead of", frontwardNodeID.String())
			return blocker
		}
		if candidate == nil {
			fmt.Println(time.Now(), k.localNodeID.String(), "== skip 2")
			return nil
		}
		return candidate.GetHeadAddress()
	}()
	if tail == nil {
		return
	}

	fmt.Println(time.Now(), k.localNodeID.String(), "== Activate hosting sector with tail", tail.String())
	hostingSector.Activate(*tail)
}

// NOTE: TLA+ Raft モデルでは activateFrontwardSector を ProposeActivate と CommitActivate に分割。
// sendSectorActivate (Raft Propose) から応答を受け取るまでの間に対象ノードが Leave する場合、
// HasManagementProposal() が true のまま stuck する可能性がある。
// タイムアウトや対象離脱検知による abort 処理の追加を検討すべき。
func (k *KVS) activateFrontwardSector(frontwardNextSector *sector.Sector) {
	sectorID := frontwardNextSector.GetKey().SectorID
	dstNodeID := *frontwardNextSector.GetHeadAddress()

	fmt.Println(time.Now(), k.localNodeID.String(), "== send sectorActivate to", dstNodeID.String(), "sectorID:", sectorID.String())
	cErr := k.outbound.sendSectorActivate(&SectorActivateParam{
		dstNodeID: &dstNodeID,
		sectorID:  sectorID,
	})

	go func() {
		err := <-cErr
		fmt.Println(time.Now(), k.localNodeID.String(), "== sectorActivate response from", dstNodeID.String(), "err:", err)
		if err != nil {
			k.logger.Warn("Failed to activate frontward sector", "dstNodeID", dstNodeID.String(), "sectorID", sectorID.String(), "error", err)
		}
	}()
}

// NOTE: TLA+ Raft モデルでは splitSector を ProposeSplit (PreCommitSplit で tail 縮小) と
// CommitSplit (frontward の活性化) の 2 ステップに分割してモデル化。
// PreCommitSplit と CommitSplit の間に他ノードの操作がインターリーブすることで
// 一時的なセクター重複が発生しうる。これは TerminateB で解消される。
// NOTE: シミュレーション (100ノード・ランダム停止, 2026-07-04) で、Migrate 内の
// frontward 側グループへの Import 提案が quorum 喪失によりコミットされず、この関数が
// 無期限ブロックするハングを観測 (5/6 件、相手ノードは生存・prepareSplit 受諾済み)。
// この間 mtxOperateSectors を握り続けるため当該ノードのセクター管理が全停止し、
// frontward 側も proposedSplitting がセットされたまま待ち続ける。グループが治癒して
// 23 秒後に回復した例もあるが、過半数喪失時は治癒に必要な ConfChange 自体が
// コミットできず永久化する。timeout+abort と quorum 喪失検知が必要。
// → 実装 (2026-07-04): Import 等のブロッキング操作は proposalWaitTimeout で
// ErrProposalTimeout を返し、下の abort パス (Terminate) に入る。quorum 喪失で
// Terminate も commit できない場合は sector.checkQuorumLoss の強制破棄が後始末する。
// frontward 側の proposedSplitting はセクター再作成による sectorID 変化で解消される。
func (k *KVS) splitSector(hostingSector, frontwardNextSector *sector.Sector) {
	fmt.Println(time.Now(), k.localNodeID.String(), "== split: send sectorPrepareSplit to", frontwardNextSector.GetHeadAddress().String())
	cErr := k.outbound.sendSectorPrepareSplit(&SectorSplitParam{
		dstNodeID: frontwardNextSector.GetHeadAddress(),
		sectorID:  frontwardNextSector.GetKey().SectorID,
	})
	if err := <-cErr; err != nil {
		fmt.Println(time.Now(), k.localNodeID.String(), "== split: sectorPrepareSplit failed:", err)
		k.logger.Warn("Failed to split sector", "error", err)
		return
	}
	frontwardNewTail := hostingSector.GetTailAddress()

	fmt.Println(time.Now(), k.localNodeID.String(), "== split: migrating records to frontward sector")
	if err := hostingSector.Migrate(frontwardNextSector); err != nil {
		// NOTE: TLA+ AbortProposal に相当。Migrate 失敗時に frontward 側の
		// 提案 (sendSectorPrepareSplit で受諾済) をキャンセルし、
		// proposing 状態が永続しないようにする。
		// PreCommitSplit/CommitSplit 失敗パスも同様。
		frontwardNextSector.Terminate()
		fmt.Println(time.Now(), k.localNodeID.String(), "== split: migrate failed:", err)
		k.logger.Warn("Failed to migrate sector", "error", err)
		return
	}

	fmt.Println(time.Now(), k.localNodeID.String(), "== split: pre-committing split on hosting sector")
	if err := hostingSector.PreCommitSplit(frontwardNextSector.GetHeadAddress()); err != nil {
		frontwardNextSector.Terminate()
		fmt.Println(time.Now(), k.localNodeID.String(), "== split: pre commit failed:", err)
		k.logger.Warn("Failed to pre commit split", "error", err)
		return
	}

	fmt.Println(time.Now(), k.localNodeID.String(), "== split: committing split on frontward sector")
	if err := frontwardNextSector.CommitSplit(frontwardNewTail); err != nil {
		frontwardNextSector.Terminate()
		fmt.Println(time.Now(), k.localNodeID.String(), "== split: commit failed:", err)
		k.logger.Warn("Failed to commit split", "error", err)
		return
	}
	fmt.Println(time.Now(), k.localNodeID.String(), "== split: done")
}

// NOTE: mergeSector は PrepareMerge → Merge → Terminate → CommitMerge の
// 3 つの Raft グループにまたがる操作。TLA+ では ProposeMerge/CommitMerge に非原子化済み。
// ProposeMerge→CommitMerge 間に TerminateB が割り込み、吸収対象 (fs) が既に
// inactive になるケースを CommitMerge が冪等に処理する設計。
// NOTE: TLA+ MaxChurn=2 検証で、Merge で吸収される側が提案中 (proposing) の場合に
// proposing 状態がクリアされないバグを発見。Go 実装でも Merge 時に吸収される側の
// 提案中操作を安全にキャンセルする処理が必要。
// NOTE: TLA+ MaxChurn=3 検証で、CommitMerge が fs を active→inactive にする際に
// anyActive フラグの更新漏れを発見。Go 実装でも merge commit 後に全セクターが
// inactive になった場合の状態遷移を正しく処理する必要がある。
// NOTE: PrepareMerge/CommitMerge も raft 適用まで cond.Wait で無期限ブロックするため、
// splitSector の Import と同じく quorum 喪失でハングする危険がある
// (シミュレーションでは merge は全件完了、未観測。2026-07-04)。
// → proposalWaitTimeout の導入で無期限ブロックは解消 (2026-07-04)。
func (k *KVS) mergeSector(hostingSector, frontwardNextSector *sector.Sector) {
	fmt.Println(time.Now(), k.localNodeID.String(), "== merge: preparing merge on frontward sector, head", frontwardNextSector.GetHeadAddress().String())
	if err := frontwardNextSector.PrepareMerge(k.localNodeID); err != nil {
		fmt.Println(time.Now(), k.localNodeID.String(), "== merge: prepare failed:", err)
		k.logger.Warn("Failed to prepare merge", "error", err)
		return
	}

	newTail := frontwardNextSector.GetTailAddress()

	fmt.Println(time.Now(), k.localNodeID.String(), "== merge: migrating records from frontward sector")
	if err := hostingSector.Merge(frontwardNextSector); err != nil {
		fmt.Println(time.Now(), k.localNodeID.String(), "== merge: migrate failed:", err)
		k.logger.Warn("Failed to merge sector", "error", err)
		return
	}

	fmt.Println(time.Now(), k.localNodeID.String(), "== merge: terminating frontward sector")
	frontwardNextSector.Terminate()

	fmt.Println(time.Now(), k.localNodeID.String(), "== merge: committing merge on hosting sector")
	if err := hostingSector.CommitMerge(newTail); err != nil {
		fmt.Println(time.Now(), k.localNodeID.String(), "== merge: commit failed:", err)
		k.logger.Warn("Failed to commit merge", "error", err)
		return
	}
	fmt.Println(time.Now(), k.localNodeID.String(), "== merge: done")
}

func (k *KVS) takeObservation() {
	k.mtx.Lock()
	sectors := make(map[kvsTypes.SectorKey]*sector.Sector, len(k.sectors))
	for sectorKey, s := range k.sectors {
		sectors[sectorKey] = s
	}
	k.mtx.Unlock()

	sectorInfos := make(map[kvsTypes.SectorKey]*observation.SectorInfo)
	for sectorKey, sector := range sectors {
		headAddress := sector.GetHeadAddress()
		tailAddress := sector.GetTailAddress()
		tail := ""
		if tailAddress != nil {
			tail = tailAddress.String()
		}
		sectorInfos[sectorKey] = &observation.SectorInfo{
			Head: headAddress.String(),
			Tail: tail,
		}
	}

	if k.updatedSectorInfo(k.observationSectorInfo, sectorInfos) {
		k.observationSectorInfo = sectorInfos
		k.observation.ChangeKvsSectors(sectorInfos)
	}
}

func (k *KVS) updatedSectorInfo(a, b map[kvsTypes.SectorKey]*observation.SectorInfo) bool {
	if len(a) != len(b) {
		return true
	}

	for key, infoA := range a {
		infoB, ok := b[key]
		if !ok {
			return true
		}
		if infoA.Head != infoB.Head || infoA.Tail != infoB.Tail {
			return true
		}
	}

	return false
}
