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

package datastore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slog"
)

type MongodbConfig struct {
	URI        string
	User       string
	Password   string
	Database   string
	Collection string
}

type Mongodb struct {
	cancel     context.CancelFunc
	client     *mongo.Client
	collection *mongo.Collection
	mtx        sync.Mutex
	buffer     []interface{}
}

type Entry struct {
	Timestamp time.Time `bson:"timestamp"`
	NodeID    string    `bson:"nodeID"`
	Record    []byte    `bson:"record"`
}

func NewMongodb(ctx context.Context, config *MongodbConfig) (*Mongodb, error) {
	bsonOpts := &options.BSONOptions{
		UseJSONStructTags: true,
		NilSliceAsEmpty:   true,
	}

	clientOpts := options.Client().
		ApplyURI(config.URI).
		SetBSONOptions(bsonOpts)

	client, err := mongo.Connect(context.Background(), clientOpts)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	m := &Mongodb{
		cancel:     cancel,
		client:     client,
		collection: client.Database(config.Database).Collection(config.Collection),
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
				err := m.flush()
				if err != nil {
					slog.Error("error on flush", slog.String("error", err.Error()))
				}
			}
		}
	}()

	return m, nil
}

func (m *Mongodb) Close() {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.cancel()
	m.client.Disconnect(context.Background())
	m.client = nil
}

func (m *Mongodb) Push(nodeID string, record any) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.client == nil {
		return
	}

	jsRecrod, err := json.Marshal(record)
	if err != nil {
		slog.Error("error on marshal", slog.String("error", err.Error()))
		return
	}

	m.buffer = append(m.buffer, &Entry{
		Timestamp: time.Now(),
		NodeID:    nodeID,
		Record:    jsRecrod,
	})
}

func (m *Mongodb) ReadAll(receiver func(*time.Time, string, []byte) error) error {
	filter := bson.D{}
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}})

	cursor, err := m.collection.Find(context.Background(), filter, opts)
	if err != nil {
		return fmt.Errorf("failed to find from mongodb: %w", err)
	}

	for cursor.Next(context.Background()) {
		var entry Entry
		if err := cursor.Decode(&entry); err != nil {
			return fmt.Errorf("failed to decode from mongodb: %w", err)
		}
		err = receiver(&entry.Timestamp, entry.NodeID, entry.Record)
		if err != nil {
			return fmt.Errorf("failed to receive: %w", err)
		}
	}

	return nil
}

func (m *Mongodb) flush() error {
	m.mtx.Lock()
	if len(m.buffer) == 0 {
		return nil
	}
	buffer := m.buffer
	m.buffer = nil
	m.mtx.Unlock()

	_, err := m.collection.InsertMany(context.Background(), buffer)
	if err != nil {
		return err
	}

	return nil
}
