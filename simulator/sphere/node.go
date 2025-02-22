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
	"time"

	"github.com/llamerada-jp/colonio"
	"github.com/llamerada-jp/colonio/config"
	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/llamerada-jp/colonio/test/util"
)

func RunNode(ctx context.Context, logger *slog.Logger, seedURL string, writer *datastore.Writer) error {
	position := newPosition()

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
		colonio.WithICEServers([]config.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		}),
		colonio.WithObservation(&colonio.ObservationHandlers{
			OnChangeConnectedNodes: func(nodeIDs map[string]struct{}) {
				record.ConnectedNodeIDs = convertMapToSlice(nodeIDs)
			},
			OnUpdateRequiredNodeIDs1D: func(nodeIDs map[string]struct{}) {
				record.RequiredNodeIDs1D = convertMapToSlice(nodeIDs)
			},
			OnUpdateRequiredNodeIDs2D: func(nodeIDs map[string]struct{}) {
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
	defer col.Stop()

	logger.Info("connected", "nodeID", col.GetLocalNodeID())

	for {
		timer := time.NewTimer(1 * time.Second)
		select {
		case <-ctx.Done():
			return nil

		case <-timer.C:
			position.moveRandom()
			logger.Info("move", "x", position.x, "y", position.y)
			if err := col.UpdateLocalPosition(position.x, position.y); err != nil {
				logger.Warn("failed to update position", "error", err)
				continue
			}

			record.X = position.x
			record.Y = position.y
			writer.Write(time.Now(), col.GetLocalNodeID(), record)
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
