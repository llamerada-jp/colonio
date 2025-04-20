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
	"fmt"
	"math"
	"time"

	"github.com/llamerada-jp/colonio/internal/shared"
	"github.com/llamerada-jp/colonio/simulator/base"
	"github.com/llamerada-jp/colonio/simulator/canvas"
	"github.com/llamerada-jp/colonio/simulator/datastore"
)

const (
	scale = 0.95
)

func RunRender(reader *datastore.Reader, canvas *canvas.Canvas, filer *canvas.Filer) error {
	canvas.EnableFont("", 16)
	framework := base.NewRenderFramework(reader, canvas, filer, 1, recordReader, drawer)
	return framework.Start()
}

func recordReader(reader *datastore.Reader) (*time.Time, string, base.RecordInterface, error) {
	record := base.Record{}
	recordTime, nodeID, err := reader.Read(&record)
	return recordTime, nodeID, &record, err
}

func drawer(framework *base.RenderFramework, canvas *canvas.Canvas, sec int) {
	canvas.SetColor(0, 0, 0)
	canvas.DrawText(
		canvas.XFromPixel(8),
		canvas.YFromPixel(canvas.GetCanvasHeighPixel()-96),
		fmt.Sprintf(
			"time: %d s\nnodes: %d\nlost nodes: %d\nconnections: %d, avg:%.1f",
			sec,
			len(framework.AliveNodes), len(framework.SilentNodes), len(framework.ConnectEdges),
			// one connection is connected two nodes
			float64(len(framework.ConnectEdges))/float64(len(framework.AliveNodes))*2.0,
		))

	canvas.SetColor(0, 63, 255)
	for n := range framework.AliveNodes {
		drawNode(canvas, n)
	}

	canvas.SetColor(63, 63, 63)
	for n := range framework.SilentNodes {
		drawNode(canvas, n)
	}

	for e := range framework.ConnectEdges {
		canvas.SetColor(0, 63, 255)
		n1 := framework.SilentNodes[e.NodeID1]
		n2 := framework.SilentNodes[e.NodeID2]
		if n1 != nil || n2 != nil {
			canvas.SetColor(63, 63, 63)
		}
		drawEdge(canvas, e.NodeID1, e.NodeID2)
	}

	for e := range framework.RequiredEdges {
		canvas.SetColor(0, 255, 63)
		if _, ok := framework.ConnectEdges[e]; ok {
			continue
		}
		n1 := framework.SilentNodes[e.NodeID1]
		n2 := framework.SilentNodes[e.NodeID2]
		if n1 != nil || n2 != nil {
			canvas.SetColor(63, 63, 63)
		}
		drawEdge(canvas, e.NodeID1, e.NodeID2)
	}
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
