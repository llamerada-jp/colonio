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
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/llamerada-jp/colonio"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/llamerada-jp/colonio/simulator/utils"
	"github.com/llamerada-jp/colonio/test/util"
)

const (
	messageKey = "simulator"
)

type Handler struct {
	OnEachTime func(node *Node) error
	OnWrite    func(node *Node) error
}

type Node struct {
	Logger   *slog.Logger
	Col      colonio.Colonio
	Position *utils.Position
	Record   RecordInterface

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
		writer:   writer,
		handler:  handler,
	}

	col, err := colonio.NewColonio(
		colonio.WithLogger(logger),
		colonio.WithHttpClient(util.NewInsecureHttpClient()),
		colonio.WithSeedURL(seedURL),
		colonio.WithICEServers([]*config.ICEServer{
			{
				URLs: []string{},
			},
		}),
		colonio.WithObservation(&colonio.ObservationHandlers{
			OnChangeConnectedNodes: func(nodeIDs map[string]struct{}) {
				n.mtx.Lock()
				defer n.mtx.Unlock()
				r.ConnectedNodeIDs = convertMapToSlice(nodeIDs)
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
		colonio.WithSphereGeometry(6378137.0),
	)
	if err != nil {
		return nil, err
	}

	col.MessagingSetHandler(messageKey, func(mr *colonio.MessagingRequest, _ colonio.MessagingResponseWriter) {
		id := string(mr.Message)
		n.mtx.Lock()
		defer n.mtx.Unlock()
		r := n.Record.GetRecord()
		r.Receive[id] = time.Now().Format(time.RFC3339Nano)
	})

	n.Col = col
	return n, nil
}

func (n *Node) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			// Randomly generate a duration between 30 seconds and 10 minutes
			durationSec := rand.Intn(9*60+30) + 30
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
			time.Sleep(5 * time.Second)
			mtx.Lock()
			defer mtx.Unlock()
			if !check && ctx != nil {
				n.Logger.Warn("might be stacked")
			}
		}()
		timer := time.NewTimer(1 * time.Second)

		// send message randomly
		dstNodeID := shared.NewRandomNodeID().String()
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
			colonio.MessagingWithAcceptNearby(),
			colonio.MessagingWithIgnoreResponse(),
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
