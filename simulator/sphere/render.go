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
	"math"
	"strings"
	"time"

	"github.com/llamerada-jp/colonio/simulator/canvas"
	"github.com/llamerada-jp/colonio/simulator/datastore"
)

type node struct {
	x         float64
	y         float64
	timestamp time.Time
}

type edge struct {
	node1 string
	node2 string
}

func RunRender(ctx context.Context, reader *datastore.Reader, canvas *canvas.Canvas, filer *canvas.Filer) error {
	var timestamp time.Time
	nodes := make(map[string]node)
	connects := make(map[edge]time.Time)
	required := make(map[edge]time.Time)

	for {
		var record Record
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

		for timestamp.Before(*recordTime) {
			draw(canvas, nodes, connects, required)
			if filer != nil {
				filer.Save()
			}
			if canvas.HasQuitSignal() {
				return nil
			}
			cleanupNodes(timestamp, nodes)
			cleanupEdges(timestamp, connects)
			cleanupEdges(timestamp, required)
			timestamp = timestamp.Add(time.Second)
			time.Sleep(20 * time.Millisecond)
		}

		nodes[nodeID] = node{
			x:         record.X,
			y:         record.Y,
			timestamp: timestamp,
		}
		appendEdges(timestamp, connects, nodeID, record.ConnectedNodeIDs)
		appendEdges(timestamp, required, nodeID, record.RequiredNodeIDs2D)
	}
}

func draw(canvas *canvas.Canvas, nodes map[string]node, connects map[edge]time.Time, required map[edge]time.Time) {
	canvas.SetColor(0, 0, 255)
	for _, n := range nodes {
		drawNode(canvas, n)
	}

	for e := range connects {
		drawEdge(canvas, nodes[e.node1], nodes[e.node2])
	}

	canvas.SetColor(255, 0, 0)
	for e := range required {
		if _, ok := connects[e]; ok {
			continue
		}
		drawEdge(canvas, nodes[e.node1], nodes[e.node2])
	}

	canvas.Render()
}

func drawNode(canvas *canvas.Canvas, n node) {
	x, y, z := convertCoordinate(n)
	canvas.DrawBox3(x, y, z, 5.0)
}

func drawEdge(canvas *canvas.Canvas, n1, n2 node) {
	x1, y1, z1 := convertCoordinate(n1)
	x2, y2, z2 := convertCoordinate(n2)
	canvas.DrawLine3(x1, y1, z1, x2, y2, z2)
}

func convertCoordinate(n node) (xo, yo, zo float64) {
	xo = math.Cos(n.x) * math.Cos(n.y)
	yo = math.Sin(n.y)
	zo = math.Sin(n.x) * math.Cos(n.y)
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

func cleanupNodes(timestamp time.Time, nodes map[string]node) {
	for nodeID, n := range nodes {
		if n.timestamp.Add(3 * time.Second).After(timestamp) {
			delete(nodes, nodeID)
		}
	}
}

func cleanupEdges(timestamp time.Time, edges map[edge]time.Time) {
	for e, t := range edges {
		if t.Add(3 * time.Second).After(timestamp) {
			delete(edges, e)
		}
	}
}
