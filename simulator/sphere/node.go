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
package sphere

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/llamerada-jp/colonio"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/llamerada-jp/colonio/simulator/utils"
	"github.com/llamerada-jp/colonio/test/util"
)

func RunNode(ctx context.Context, logger *slog.Logger, seedURL string, writer *datastore.Writer) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			// Randomly generate a duration between 30 seconds and 10 minutes
			durationSec := rand.Intn(9*60) + 60
			timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(durationSec)*time.Second)
			if err := runNode(timeoutCtx, logger, seedURL, writer); err != nil {
				logger.Error("node error", "error", err)
				cancel()
				return err
			}
			time.Sleep(time.Duration(rand.Intn(30)) * time.Second)
			// do cancel to avoid context leak
			cancel()
		}
	}
}

func runNode(ctx context.Context, logger *slog.Logger, seedURL string, writer *datastore.Writer) error {
	position := utils.NewPosition()

	mtx := sync.Mutex{}
	record := &Record{
		ConnectedNodeIDs:  make([]string, 0),
		RequiredNodeIDs1D: make([]string, 0),
		RequiredNodeIDs2D: make([]string, 0),
	}

	var err error
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
				mtx.Lock()
				defer mtx.Unlock()
				record.ConnectedNodeIDs = convertMapToSlice(nodeIDs)
			},
			OnUpdateRequiredNodeIDs1D: func(nodeIDs map[string]struct{}) {
				mtx.Lock()
				defer mtx.Unlock()
				record.RequiredNodeIDs1D = convertMapToSlice(nodeIDs)
			},
			OnUpdateRequiredNodeIDs2D: func(nodeIDs map[string]struct{}) {
				mtx.Lock()
				defer mtx.Unlock()
				record.RequiredNodeIDs2D = convertMapToSlice(nodeIDs)
			},
		}),
		colonio.WithSphereGeometry(6378137.0),
	)
	if err != nil {
		return err
	}
	if err := col.Start(ctx); err != nil {
		return err
	}

	localNodeID := col.GetLocalNodeID()
	logger.Info("connected", "nodeID", localNodeID)

	mtx.Lock()
	record.State = state_start
	writer.Write(time.Now(), localNodeID, record)
	mtx.Unlock()

	defer func() {
		mtx.Lock()
		defer mtx.Unlock()
		record.State = state_stop
		writer.Write(time.Now(), localNodeID, record)
		col.Stop()
	}()

	for {
		timer := time.NewTimer(1 * time.Second)
		select {
		case <-ctx.Done():
			return nil

		case <-timer.C:
			position.MoveRandom()
			logger.Info("move", "x", position.X, "y", position.Y)
			if err := col.UpdateLocalPosition(position.X, position.Y); err != nil {
				logger.Warn("failed to update position", "error", err)
				continue
			}

			mtx.Lock()
			record.State = state_normal
			record.X = position.X
			record.Y = position.Y
			writer.Write(time.Now(), localNodeID, record)
			mtx.Unlock()
		}
	}
}

func convertMapToSlice(m map[string]struct{}) []string {
	s := make([]string, 0, len(m))
	for k := range m {
		s = append(s, k)
	}
	return s
}
