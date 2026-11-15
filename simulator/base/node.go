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
	"log/slog"
	"math/rand"
	"runtime/pprof"
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
	// loopWarnDuration is how long one iteration of the runNode loop may take
	// before the watchdog warns, and loopStopDuration is how long before the
	// watchdog stops the node.
	loopWarnDuration = 5 * time.Second
	loopStopDuration = 60 * time.Second
)

// dumpGoroutinesOnce limits the goroutine dump to once per process. The dump
// of a process running many nodes is large, and one is enough to find where a
// loop is blocked.
var dumpGoroutinesOnce sync.Once

func dumpGoroutines(logger *slog.Logger) {
	dumpGoroutinesOnce.Do(func() {
		var buf bytes.Buffer
		if err := pprof.Lookup("goroutine").WriteTo(&buf, 1); err != nil {
			logger.Error("failed to dump goroutines", "error", err)
			return
		}
		logger.Error("goroutine dump", "dump", buf.String())
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
	options []colonioNode.ConfigSetter
	writer  *datastore.Writer
	handler *Handler
	mtx     sync.Mutex
}

// NewNode creates a node of the simulation. options are passed to the colonio
// node in addition to those the simulation sets.
func NewNode(logger *slog.Logger, seedURL string, writer *datastore.Writer, handler *Handler, record RecordInterface, region *utils.Region, options ...colonioNode.ConfigSetter) (*Node, error) {
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
		options:  options,
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

			// Randomly generate a duration between 1 and 20 minutes
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

	options := []colonioNode.ConfigSetter{
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
	}
	options = append(options, n.options...)

	var err error
	n.Col, err = colonioNode.NewNode(options...)
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

	// The watchdog cancels ctx to end the loop of a node that it stopped.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := n.Col.Start(ctx); err != nil {
		return err
	}

	// n.Col is replaced when the node is renewed, so keep the one of this run.
	col := n.Col
	localNodeID := col.GetLocalNodeID()
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

			time.Sleep(loopWarnDuration)
			if !isStuck() {
				return
			}
			n.Logger.Warn("might be stacked")

			// A loop blocked for this long does not recover. The node keeps
			// its links and its seed registration while it cannot send or
			// receive, so stop it to let the other nodes detect the failure.
			time.Sleep(loopStopDuration - loopWarnDuration)
			if !isStuck() {
				return
			}
			dumpGoroutines(n.Logger)
			n.Logger.Error("loop is stuck, stopping the node", "nodeID", localNodeID)
			col.Stop()
			cancel()
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
