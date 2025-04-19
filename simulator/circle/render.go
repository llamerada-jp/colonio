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
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/simulator/base"
	"github.com/llamerada-jp/colonio/simulator/canvas"
	"github.com/llamerada-jp/colonio/simulator/datastore"
)

const (
	scale = 0.95
)

type edge struct {
	node1 string
	node2 string
}

func RunRender(ctx context.Context, reader *datastore.Reader, canvas *canvas.Canvas, filer *canvas.Filer) error {
	var timestamp time.Time
	sec := 0
	nodes := make(map[string]time.Time)
	connects := make(map[edge]time.Time)
	required := make(map[edge]time.Time)
	canvas.EnableFont("", 16)

	for {
		var record base.Record
		recordTime, nodeID, err := reader.Read(&record)
		if err != nil {
			return err
		}
		if recordTime == nil {
			return nil
		}
		if timestamp.Equal(time.Time{}) {
			timestamp = recordTime.Round(time.Second)
		}

		switch record.State {
		case base.StateStart:
			nodes[nodeID] = timestamp

		case base.StateNormal:
			// skip record if node is not alive
			if _, ok := nodes[nodeID]; !ok {
				continue
			}
			nodes[nodeID] = timestamp

		case base.StateStop:
			delete(nodes, nodeID)
		}

		appendEdges(timestamp, connects, nodeID, record.ConnectedNodeIDs)
		appendEdges(timestamp, required, nodeID, record.RequiredNodeIDs1D)
		appendEdges(timestamp, required, nodeID, record.RequiredNodeIDs2D)

		for timestamp.Before(*recordTime) {
			sec += 1
			draw(canvas, sec, timestamp, nodes, connects, required)
			if filer != nil {
				filer.Save()
			}
			if canvas.HasQuitSignal() {
				return nil
			}
			cleanupEdges(timestamp, connects)
			cleanupEdges(timestamp, required)
			timestamp = timestamp.Add(time.Second)
			// time.Sleep(20 * time.Millisecond)
		}
	}
}

func draw(canvas *canvas.Canvas, sec int, current time.Time, nodes map[string]time.Time, connects map[edge]time.Time, required map[edge]time.Time) {
	silentNodeCount := 0

	for n, t := range nodes {
		if current.After(t.Add(3 * time.Second)) {
			silentNodeCount++
			canvas.SetColor(63, 63, 63)
		} else {
			canvas.SetColor(0, 0, 255)
		}
		drawNode(canvas, n)
	}

	canvas.SetColor(0, 0, 0)
	canvas.DrawText(
		canvas.XFromPixel(8),
		canvas.YFromPixel(canvas.GetCanvasHeighPixel()-96),
		fmt.Sprintf(
			"time: %d s\nnodes: %d\nlost nodes: %d\nconnections: %d, avg:%.1f",
			sec,
			len(nodes), silentNodeCount, len(connects),
			// one connection is connected two nodes
			float64(len(connects))/float64(len(nodes))*2.0,
		))

	for e := range connects {
		canvas.SetColor(0, 0, 255)
		if t, ok := nodes[e.node1]; ok {
			if current.After(t.Add(3 * time.Second)) {
				canvas.SetColor(63, 63, 63)
			}
		} else {
			continue
		}
		if t, ok := nodes[e.node2]; ok {
			if current.After(t.Add(3 * time.Second)) {
				canvas.SetColor(63, 63, 63)
			}
		} else {
			continue
		}
		drawEdge(canvas, e.node1, e.node2)
	}

	canvas.SetColor(191, 255, 191)
	for e := range required {
		if _, ok := connects[e]; ok {
			continue
		}

		if t, ok := nodes[e.node1]; ok {
			if current.After(t.Add(3 * time.Second)) {
				canvas.SetColor(63, 63, 63)
			}
		} else {
			continue
		}
		if t, ok := nodes[e.node2]; ok {
			if current.After(t.Add(3 * time.Second)) {
				canvas.SetColor(63, 63, 63)
			}
		} else {
			continue
		}
		drawEdge(canvas, e.node1, e.node2)
	}

	canvas.Render()
}

func drawNode(canvas *canvas.Canvas, n string) {
	x, y := convertCoordinate(n)
	canvas.DrawBox2(x, y, 5.0)
}

func drawEdge(canvas *canvas.Canvas, n1, n2 string) {
	x1, y1 := convertCoordinate(n1)
	x2, y2 := convertCoordinate(n2)
	canvas.DrawLine2(x1, y1, x2, y2)
}

func convertCoordinate(n string) (xo, yo float64) {
	nodeID := shared.NewNodeIDFromString(n)
	if nodeID == nil {
		panic(fmt.Sprintf("invalid nodeID: %s", n))
	}
	i, _ := nodeID.Raw()
	rad := 2 * math.Pi * float64(i) / math.MaxUint64
	xo = math.Cos(rad) * scale
	yo = math.Sin(rad) * scale
	return
}

func appendEdges(timestamp time.Time, edges map[edge]time.Time, nodeID string, nodes []string) {
	for _, node := range nodes {
		if strings.Compare(nodeID, node) < 0 {
			edges[edge{node1: nodeID, node2: node}] = timestamp
		} else {
			edges[edge{node1: node, node2: nodeID}] = timestamp
		}
	}
}

func cleanupEdges(timestamp time.Time, edges map[edge]time.Time) {
	for e, t := range edges {
		if timestamp.After(t.Add(3 * time.Second)) {
			delete(edges, e)
		}
	}
}
