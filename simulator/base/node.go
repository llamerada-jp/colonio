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
package base

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	colonioNode "github.com/llamerada-jp/colonio/node"
	"github.com/llamerada-jp/colonio/node/observation"
	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/llamerada-jp/colonio/simulator/utils"
	"github.com/llamerada-jp/colonio/test/util"
	"github.com/llamerada-jp/colonio/types"
	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
	networkTypes "github.com/llamerada-jp/colonio/types/network"
)

const (
	messageKey = "simulator"
	// stackWarnDuration is how long one runNode iteration may take before the
	// watchdog warns; stackStopDuration is how long before the node is treated
	// as a "network zombie" and forcibly stopped (シミュレーション 2026-07-06:
	// ノード内部のロック滞留でループが MessagingPost 内に永久ブロックし、
	// リンク keepalive だけが生き残る半死ノードが 12 件発生。既存の全
	// 故障検知をすり抜けて undercount 赤 / is_stable 凍結の原因になった)。
	stackWarnDuration = 5 * time.Second
	stackStopDuration = 60 * time.Second
)

// dumpGoroutinesOnce writes an aggregated goroutine dump to stdout on the
// first watchdog escalation, to pin down the exact frame the stacked loop is
// blocked on. Once per process: one dump is enough to diagnose, and a full
// dump of a 100-node process is large. Each line is prefixed with "==" so the
// simulator's log collection keeps it (it drops lines without "=="/"@@").
var dumpGoroutinesOnce sync.Once

func dumpGoroutines() {
	dumpGoroutinesOnce.Do(func() {
		var buf bytes.Buffer
		if err := pprof.Lookup("goroutine").WriteTo(&buf, 1); err != nil {
			fmt.Println("== gdump: failed to dump goroutines:", err)
			return
		}
		for _, line := range strings.Split(buf.String(), "\n") {
			fmt.Println("== gdump:", line)
		}
	})
}

type Handler struct {
	OnEachTime func(node *Node) error
	OnWrite    func(node *Node) error
}

type Node struct {
	Logger   *slog.Logger
	Col      colonioNode.Node
	Position *utils.Position
	Record   RecordInterface

	seedURL string
	writer  *datastore.Writer
	handler *Handler
	mtx     sync.Mutex
}

func NewNode(logger *slog.Logger, seedURL string, writer *datastore.Writer, handler *Handler, record RecordInterface, region *utils.Region) (*Node, error) {
	r := record.GetRecord()
	r.ConnectedNodeIDs = make([]string, 0)
	r.RequiredNodeIDs1D = make([]string, 0)
	r.RequiredNodeIDs2D = make([]string, 0)
	r.Post = make(map[string]string)
	r.Receive = make(map[string]string)

	n := &Node{
		Logger:   logger,
		Position: utils.NewPosition(region),
		Record:   record,
		seedURL:  seedURL,
		writer:   writer,
		handler:  handler,
	}

	return n, nil
}

func (n *Node) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			if err := n.renewColonio(); err != nil {
				n.Logger.Error("failed to renew colonio", "error", err)
				return err
			}

			// Randomly generate a duration between 1~10 minutes
			durationSec := rand.Intn(19*60) + 60
			timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(durationSec)*time.Second)
			if err := n.runNode(timeoutCtx); err != nil {
				n.Logger.Error("node error", "error", err)
				cancel()
				return err
			}
			time.Sleep(time.Duration(rand.Intn(30)) * time.Second)
			// do cancel to avoid context leak
			cancel()
		}
	}
}

func (n *Node) Write(f func() error) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if err := f(); err != nil {
		n.Logger.Error("error on cb", "error", err)
		return
	}

	if n.handler != nil && n.handler.OnWrite != nil {
		if err := n.handler.OnWrite(n); err != nil {
			n.Logger.Error("error on OnWrite", "error", err)
			return
		}
	}

	if err := n.writer.Write(time.Now(), n.Col.GetLocalNodeID(), n.Record); err != nil {
		n.Logger.Error("failed to write record", "error", err)
	}

	r := n.Record.GetRecord()
	r.Post = make(map[string]string)
	r.Receive = make(map[string]string)
}

func (n *Node) renewColonio() error {
	r := n.Record.GetRecord()

	var err error
	n.Col, err = colonioNode.NewNode(
		colonioNode.WithLogger(n.Logger),
		colonioNode.WithHttpClient(util.NewInsecureHttpClient()),
		colonioNode.WithSeedURL(n.seedURL),
		colonioNode.WithICEServers([]*networkTypes.ICEServer{
			{
				URLs: []string{},
			},
		}),
		colonioNode.WithObservation(&observation.Handler{
			OnChangeConnectedNodes: func(nodeIDs map[string]struct{}) {
				n.mtx.Lock()
				defer n.mtx.Unlock()
				r.ConnectedNodeIDs = convertMapToSlice(nodeIDs)
			},
			OnChangeKvsSectors: func(s map[kvsTypes.SectorKey]*observation.SectorInfo) {
				n.mtx.Lock()
				defer n.mtx.Unlock()
				sectorInfos := make([]SectorInfo, 0, len(s))
				for sectorKey, info := range s {
					sectorInfos = append(sectorInfos, SectorInfo{
						SectorID: sectorKey.SectorID.String(),
						SectorNo: uint64(sectorKey.SectorNo),
						Head:     info.Head,
						Tail:     info.Tail,
					})
				}
				r.SectorInfos = sectorInfos
			},
			OnUpdateRequiredNodeIDs1D: func(nodeIDs map[string]struct{}) {
				n.mtx.Lock()
				defer n.mtx.Unlock()
				r.RequiredNodeIDs1D = convertMapToSlice(nodeIDs)
			},
			OnUpdateRequiredNodeIDs2D: func(nodeIDs map[string]struct{}) {
				n.mtx.Lock()
				defer n.mtx.Unlock()
				r.RequiredNodeIDs2D = convertMapToSlice(nodeIDs)
			},
		}),
		colonioNode.WithSphereGeometry(6378137.0),
	)
	if err != nil {
		return err
	}

	n.Col.MessagingSetHandler(messageKey, func(mr *colonioNode.MessagingRequest, _ colonioNode.MessagingResponseWriter) {
		id := string(mr.Message)
		n.mtx.Lock()
		defer n.mtx.Unlock()
		r := n.Record.GetRecord()
		r.Receive[id] = time.Now().Format(time.RFC3339Nano)
	})

	return nil
}

func (n *Node) runNode(ctx context.Context) error {
	mtx := sync.Mutex{}

	if err := n.Col.Start(ctx); err != nil {
		return err
	}

	localNodeID := n.Col.GetLocalNodeID()
	n.Logger.Info("start", "nodeID", localNodeID)

	defer func() {
		n.Write(func() error {
			r := n.Record.GetRecord()
			r.State = StateStop
			return nil
		})
		n.Logger.Info("end", "nodeID", localNodeID)
		n.Col.Stop()

		// set ctx to nil to tell the stuck checking goroutine that this node is stopped
		mtx.Lock()
		defer mtx.Unlock()
		ctx = nil
	}()

	n.Position.MoveRandom()
	if err := n.Col.UpdateLocalPosition(n.Position.X, n.Position.Y); err != nil {
		n.Logger.Warn("failed to update position", "error", err)
	}

	// KVS write load for the snapshot Stage 6 verification (no-op unless
	// COLONIO_SIM_KVS_INTERVAL_MS is set; see kvsload.go)
	n.startKvsLoad(ctx)

	n.Write(func() error {
		r := n.Record.GetRecord()
		r.State = StateStart
		r.IsOnline = n.Col.IsOnline()
		r.IsStable = n.Col.IsStable()
		return nil
	})

	for {
		check := false
		go func() {
			isStuck := func() bool {
				mtx.Lock()
				defer mtx.Unlock()
				return !check && ctx != nil
			}

			time.Sleep(stackWarnDuration)
			if !isStuck() {
				return
			}
			n.Logger.Warn("might be stacked")

			// The loop never recovers once it blocks inside the node's network
			// stack (na.mtx freeze); the node keeps its links and seed
			// registration alive while being unable to send or receive, which
			// poisons every raft group and neighbor around it. Dump the
			// goroutines to identify the blocked frame, then stop the node so
			// it becomes fully dead and the existing failure detectors
			// (link timeout, force terminate, member reap) can take over.
			time.Sleep(stackStopDuration - stackWarnDuration)
			if !isStuck() {
				return
			}
			dumpGoroutines()
			n.Logger.Error("loop is stacked, stopping the node to avoid a network zombie",
				"nodeID", localNodeID)
			n.Col.Stop()
		}()
		timer := time.NewTimer(1 * time.Second)

		// send message randomly
		dstNodeID := types.NewRandomNodeID().String()
		id7, err := uuid.NewV7()
		if err != nil {
			panic(err)
		}
		id := id7.String()
		r := n.Record.GetRecord()
		n.mtx.Lock()
		r.Post[id] = time.Now().Format(time.RFC3339Nano)
		n.mtx.Unlock()
		n.Col.MessagingPost(dstNodeID, messageKey, []byte(id),
			colonioNode.MessagingWithAcceptNearby(),
			colonioNode.MessagingWithIgnoreResponse(),
		)

		select {
		case <-ctx.Done():
			return nil

		case <-timer.C:
			if n.handler != nil && n.handler.OnEachTime != nil {
				if err := n.handler.OnEachTime(n); err != nil {
					n.Logger.Error("error on OnEachTime", "error", err)
					return err
				}
			}

			n.Write(func() error {
				r := n.Record.GetRecord()
				r.State = StateNormal
				r.IsOnline = n.Col.IsOnline()
				r.IsStable = n.Col.IsStable()
				return nil
			})
		}
		mtx.Lock()
		check = true
		mtx.Unlock()
	}
}

func convertMapToSlice(m map[string]struct{}) []string {
	s := make([]string, 0, len(m))
	for k := range m {
		s = append(s, k)
	}
	return s
}
