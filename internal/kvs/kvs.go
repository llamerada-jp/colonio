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
package kvs

import (
	"context"
	"log/slog"

	"github.com/llamerada-jp/colonio/internal/kvs/raft"
	"github.com/llamerada-jp/colonio/internal/network/transferer"
	"github.com/llamerada-jp/colonio/internal/shared"
)

type Handler interface {
	KVSGetStability() (bool, []*shared.NodeID)
}

type Config struct {
	Logger     *slog.Logger
	Handler    Handler
	Transferer *transferer.Transferer
}

type KVS struct {
	logger      *slog.Logger
	handler     Handler
	transferer  *transferer.Transferer
	raftManager *raft.Manager
}

func NewKVS(config *Config) *KVS {
	k := &KVS{
		logger:     config.Logger,
		handler:    config.Handler,
		transferer: config.Transferer,
	}

	k.raftManager = raft.NewManager(&raft.Config{
		Logger:     config.Logger,
		Handler:    k,
		Transferer: config.Transferer,
	})

	return k
}

func (k *KVS) Start(ctx context.Context, localNodeID *shared.NodeID) error {
	go k.raftManager.Run(ctx, localNodeID)
	return nil
}

func (k *KVS) RaftGetStability() (bool, []*shared.NodeID) {
	return k.handler.KVSGetStability()
}
