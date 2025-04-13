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
package circle

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

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

func RunNode(ctx context.Context, logger *slog.Logger, seedURL string, writer *datastore.Writer) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			// Randomly generate a duration between 30 seconds and 10 minutes
			durationSec := rand.Intn(9*60+30) + 30
			timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(durationSec)*time.Second)
			if err := node(timeoutCtx, logger, seedURL, writer); err != nil {
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

func node(ctx context.Context, logger *slog.Logger, seedURL string, writer *datastore.Writer) error {
	position := utils.NewPosition()
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

	col.MessagingSetHandler(messageKey, func(mr *colonio.MessagingRequest, mrw colonio.MessagingResponseWriter) {
		// Do nothing
	})

	if err := col.Start(ctx); err != nil {
		return err
	}

	localNodeID := col.GetLocalNodeID()
	logger.Info("connected", "nodeID", localNodeID)

	record.State = state_start
	writer.Write(time.Now(), localNodeID, record)
	defer func() {
		record := &Record{
			State: state_stop,
		}
		writer.Write(time.Now(), localNodeID, record)
		col.Stop()
	}()

	for {
		timer := time.NewTimer(1 * time.Second)

		// send message randomly
		dstNodeID := shared.NewRandomNodeID().String()
		col.MessagingPost(dstNodeID, messageKey, []byte("hello"),
			colonio.MessagingWithAcceptNearby(),
			colonio.MessagingWithIgnoreResponse(),
		)

		position.MoveRandom()
		if err := col.UpdateLocalPosition(position.X, position.Y); err != nil {
			logger.Warn("failed to update position", "error", err)
		}

		select {
		case <-ctx.Done():
			logger.Info("context done")
			return nil

		case <-timer.C:
			record.State = state_normal
			writer.Write(time.Now(), localNodeID, record)
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
