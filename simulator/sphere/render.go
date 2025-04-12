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
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/llamerada-jp/colonio/simulator/canvas"
	"github.com/llamerada-jp/colonio/simulator/datastore"
)

const (
	scale = 0.95
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
	sec := 0
	aliveNodes := make(map[string]*node)
	silentNodes := make(map[string]*node)
	connects := make(map[edge]time.Time)
	required := make(map[edge]time.Time)
	canvas.EnableFont("", 16)

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

		if record.State == state_start {
			silentNodes[nodeID] = &node{
				x:         record.X,
				y:         record.Y,
				timestamp: timestamp,
			}
			continue
		}
		if record.State == state_stop {
			delete(aliveNodes, nodeID)
			delete(silentNodes, nodeID)
		}
		// skip record if node is not alive
		_, alive := aliveNodes[nodeID]
		_, silent := silentNodes[nodeID]
		if !alive && !silent {
			continue
		}

		for timestamp.Before(*recordTime) {
			sec += 1
			draw(canvas, sec, aliveNodes, silentNodes, connects, required)
			if filer != nil {
				filer.Save()
			}
			if canvas.HasQuitSignal() {
				return nil
			}
			cleanupNodes(timestamp, aliveNodes, silentNodes)
			cleanupEdges(timestamp, connects)
			cleanupEdges(timestamp, required)
			timestamp = timestamp.Add(time.Second)
			// time.Sleep(20 * time.Millisecond)
		}

		aliveNodes[nodeID] = &node{
			x:         record.X,
			y:         record.Y,
			timestamp: timestamp,
		}
		appendEdges(timestamp, connects, nodeID, record.ConnectedNodeIDs)
		appendEdges(timestamp, required, nodeID, record.RequiredNodeIDs2D)
	}
}

func draw(canvas *canvas.Canvas, sec int, aliveNodes, silentNodes map[string]*node, connects map[edge]time.Time, required map[edge]time.Time) {
	canvas.SetColor(0, 0, 0)
	canvas.DrawText(
		canvas.XFromPixel(8),
		canvas.YFromPixel(canvas.GetCanvasHeighPixel()-96),
		fmt.Sprintf(
			"time: %d s\nnodes: %d\nlost nodes: %d\nconnections: %d, avg:%.1f",
			sec,
			len(aliveNodes), len(silentNodes), len(connects),
			// one connection is connected two nodes
			float64(len(connects))/float64(len(aliveNodes))*2.0,
		))

	canvas.SetColor(0, 0, 255)
	for _, n := range aliveNodes {
		drawNode(canvas, n)
	}

	canvas.SetColor(63, 63, 63)
	for _, n := range silentNodes {
		drawNode(canvas, n)
	}

	/*
		for e := range connects {
			canvas.SetColor(0, 0, 255)
			var n1, n2 *node
			if n, ok := aliveNodes[e.node1]; ok {
				n1 = n
			} else if n, ok := silentNodes[e.node1]; ok {
				canvas.SetColor(255, 0, 0)
				n1 = n
			} else {
				continue
			}
			if n, ok := aliveNodes[e.node2]; ok {
				n2 = n
			} else if n, ok := silentNodes[e.node2]; ok {
				canvas.SetColor(255, 0, 0)
				n2 = n
			} else {
				continue
			}
			drawEdge(canvas, n1, n2)
		}
	//*/

	for e := range required {
		canvas.SetColor(0, 255, 63)
		if _, ok := connects[e]; ok {
			canvas.SetColor(0, 0, 255)
		}
		n1 := aliveNodes[e.node1]
		if n1 == nil {
			n1 = silentNodes[e.node1]
		}
		n2 := aliveNodes[e.node2]
		if n2 == nil {
			n2 = silentNodes[e.node2]
		}
		if n1 == nil || n2 == nil {
			continue
		}
		drawEdge(canvas, n1, n2)
	}

	canvas.Render()
}

func drawNode(canvas *canvas.Canvas, n *node) {
	x, y, z := convertCoordinate(n)
	canvas.DrawBox3(x, y, z, 5.0)
}

func drawEdge(canvas *canvas.Canvas, n1, n2 *node) {
	x1, y1, z1 := convertCoordinate(n1)
	x2, y2, z2 := convertCoordinate(n2)
	canvas.DrawLine3(x1, y1, z1, x2, y2, z2)
}

func convertCoordinate(n *node) (xo, yo, zo float64) {
	xo = math.Cos(n.x) * math.Cos(n.y) * scale
	yo = math.Sin(n.y) * scale
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

func cleanupNodes(timestamp time.Time, aliveNodes, silentNodes map[string]*node) {
	for nodeID, n := range aliveNodes {
		if timestamp.After(n.timestamp.Add(3 * time.Second)) {
			delete(aliveNodes, nodeID)
			silentNodes[nodeID] = n
		} else {
			delete(silentNodes, nodeID)
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
