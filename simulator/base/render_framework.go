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
	"slices"
	"strings"
	"time"

	"github.com/llamerada-jp/colonio/simulator/canvas"
	"github.com/llamerada-jp/colonio/simulator/datastore"
)

type NodeInfo struct {
	timestamp time.Time
	Record    RecordInterface
}

type Edge struct {
	NodeID1 string
	NodeID2 string
}

type PacketInfo struct {
	PostTime    time.Time
	ReceiveTime time.Time
}

type RenderFramework struct {
	reader            *datastore.Reader
	canvas            *canvas.Canvas
	filer             *canvas.Filer
	edgeKind          int
	packetInfos       map[string]*PacketInfo
	recordReader      func(*datastore.Reader) (*time.Time, string, RecordInterface, error)
	drawer            func(*RenderFramework, *canvas.Canvas, int)
	AliveNodes        map[string]*NodeInfo
	SilentNodes       map[string]*NodeInfo
	ConnectEdges      map[Edge]time.Time
	RequiredEdges     map[Edge]time.Time
	PacketCount       int
	PacketLossRate    float64
	PacketDurationP90 time.Duration
	PacketDurationP99 time.Duration
}

func NewRenderFramework(reader *datastore.Reader, canvas *canvas.Canvas, filer *canvas.Filer, edgeKind int,
	recordReader func(*datastore.Reader) (*time.Time, string, RecordInterface, error),
	drawer func(*RenderFramework, *canvas.Canvas, int)) *RenderFramework {

	r := &RenderFramework{
		reader:        reader,
		canvas:        canvas,
		filer:         filer,
		edgeKind:      edgeKind,
		recordReader:  recordReader,
		drawer:        drawer,
		AliveNodes:    make(map[string]*NodeInfo),
		SilentNodes:   make(map[string]*NodeInfo),
		ConnectEdges:  make(map[Edge]time.Time),
		RequiredEdges: make(map[Edge]time.Time),
		packetInfos:   make(map[string]*PacketInfo),
	}
	return r
}

func (f *RenderFramework) Start() error {
	var timestamp time.Time
	sec := 0

	for {
		recordTime, nodeID, recordIf, err := f.recordReader(f.reader)
		record := recordIf.GetRecord()
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
		case StateStart:
			f.AliveNodes[nodeID] = &NodeInfo{
				timestamp: timestamp,
				Record:    recordIf,
			}

		case StateNormal:
			// skip record if node is not alive
			_, alive := f.AliveNodes[nodeID]
			_, silent := f.SilentNodes[nodeID]
			if !alive && !silent {
				continue
			}
			f.AliveNodes[nodeID] = &NodeInfo{
				timestamp: timestamp,
				Record:    recordIf,
			}

			for uuid, postTimeStr := range record.Post {
				postTime, err := time.Parse(time.RFC3339Nano, postTimeStr)
				if err != nil {
					panic(err)
				}
				if packetInfo, ok := f.packetInfos[uuid]; ok {
					packetInfo.PostTime = postTime
				} else {
					f.packetInfos[uuid] = &PacketInfo{
						PostTime: postTime,
					}
				}
			}

			for uuid, receiveTimeStr := range record.Receive {
				receiveTime, err := time.Parse(time.RFC3339Nano, receiveTimeStr)
				if err != nil {
					panic(err)
				}
				if packetInfo, ok := f.packetInfos[uuid]; ok {
					packetInfo.ReceiveTime = receiveTime
				} else {
					f.packetInfos[uuid] = &PacketInfo{
						ReceiveTime: receiveTime,
					}
				}
			}

		case StateStop:
			delete(f.AliveNodes, nodeID)
			delete(f.SilentNodes, nodeID)
			continue
		}

		f.appendEdges(timestamp, f.ConnectEdges, nodeID, record.ConnectedNodeIDs)
		if f.edgeKind&0x1 != 0 {
			f.appendEdges(timestamp, f.RequiredEdges, nodeID, record.RequiredNodeIDs1D)
		}
		if f.edgeKind&0x2 != 0 {
			f.appendEdges(timestamp, f.RequiredEdges, nodeID, record.RequiredNodeIDs2D)
		}

		for timestamp.Before(*recordTime) {
			f.aggregatePacketInfos(timestamp)
			sec += 1
			f.drawer(f, f.canvas, sec)
			f.canvas.Render()
			if f.filer != nil {
				f.filer.Save()
			}
			if f.canvas.HasQuitSignal() {
				return nil
			}
			f.cleanupNodes(timestamp)
			f.cleanupEdges(timestamp, f.ConnectEdges)
			f.cleanupEdges(timestamp, f.RequiredEdges)
			timestamp = timestamp.Add(time.Second)
		}
	}
}

// will return loss rate, duration p90 sec, duration p99 sec
func (f *RenderFramework) aggregatePacketInfos(timestamp time.Time) {
	durations := make([]time.Duration, 0)
	count := 0
	receivedCount := 0

	for uuid, packetInfo := range f.packetInfos {
		baseTime := packetInfo.PostTime
		if baseTime.IsZero() {
			baseTime = packetInfo.ReceiveTime
		}
		// remove info before 70 seconds
		if baseTime.Add(70 * time.Second).Before(timestamp) {
			delete(f.packetInfos, uuid)
			continue
		}
		// skip if info within 10 seconds
		if baseTime.Add(10 * time.Second).After(timestamp) {
			continue
		}
		// recording order is reversed, so wait until both are aligned
		if packetInfo.PostTime.IsZero() {
			continue
		}
		count++
		if !packetInfo.ReceiveTime.IsZero() {
			receivedCount++
			durations = append(durations, packetInfo.ReceiveTime.Sub(packetInfo.PostTime))
		}
	}

	f.PacketCount = count
	f.PacketLossRate = float64(count-receivedCount) / float64(count)

	if len(durations) == 0 {
		f.PacketDurationP90 = 0
		f.PacketDurationP99 = 0
		return
	}

	// sort durations
	slices.Sort(durations)
	// calculate p90 and p99
	f.PacketDurationP90 = durations[int(float64(len(durations))*0.9)]
	f.PacketDurationP99 = durations[int(float64(len(durations))*0.99)]
}

func (f *RenderFramework) appendEdges(timestamp time.Time, edges map[Edge]time.Time, nodeID string, nodes []string) {
	for _, node := range nodes {
		if strings.Compare(nodeID, node) < 0 {
			edges[Edge{NodeID1: nodeID, NodeID2: node}] = timestamp
		} else {
			edges[Edge{NodeID1: node, NodeID2: nodeID}] = timestamp
		}
	}
}

func (f *RenderFramework) cleanupNodes(timestamp time.Time) {
	for nodeID, n := range f.AliveNodes {
		if timestamp.After(n.timestamp.Add(3 * time.Second)) {
			delete(f.AliveNodes, nodeID)
			f.SilentNodes[nodeID] = n
		} else {
			delete(f.SilentNodes, nodeID)
		}
	}
}

func (f *RenderFramework) cleanupEdges(timestamp time.Time, edges map[Edge]time.Time) {
	for e, t := range edges {
		if timestamp.After(t.Add(3 * time.Second)) {
			delete(edges, e)
		}
	}
}
