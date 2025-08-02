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
package raft

import (
	"github.com/google/uuid"
	"github.com/llamerada-jp/colonio/internal/shared"
	etcdraft "go.etcd.io/raft/v3"
)

type nodeConfig struct {
	clusterID uuid.UUID
	sequence  uint64
	member    map[uint64]*shared.NodeID
}

type node struct {
	node      etcdraft.Node
	clusterID uuid.UUID
	sequence  uint64
	member    map[uint64]*shared.NodeID
}

func newNode(config *nodeConfig) *node {
	n := &node{
		clusterID: config.clusterID,
		sequence:  config.sequence,
		member:    config.member,
	}

	raftConfig := &etcdraft.Config{
		ID:            config.sequence,
		ElectionTick:  10,
		HeartbeatTick: 1,
		// Storage:                   rc.raftStorage,
		MaxSizePerMsg:             1024 * 1024,
		MaxInflightMsgs:           256,
		MaxUncommittedEntriesSize: 1 << 30,
	}

	if config.sequence == 0 {
		peers := make([]etcdraft.Peer, len(config.member))
		i := 0
		for seq := range config.member {
			peers[i] = etcdraft.Peer{
				ID: seq,
			}
			i++
		}
		n.node = etcdraft.StartNode(raftConfig, peers)
	} else {
		n.node = etcdraft.RestartNode(raftConfig)
	}

	return n
}

func (n *node) start() error {
	return nil
}
