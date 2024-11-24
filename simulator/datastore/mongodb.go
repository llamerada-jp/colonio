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

type MongodbWriter struct {
	ctx        context.Context
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

func NewMongodbWriter(ctx context.Context, config *MongodbConfig) (RawWriter, error) {
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

	m := &MongodbWriter{
		ctx:        ctx,
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

func (m *MongodbWriter) Write(timestamp time.Time, nodeID string, record []byte) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.client == nil {
		return nil
	}

	m.buffer = append(m.buffer, &Entry{
		Timestamp: timestamp,
		NodeID:    nodeID,
		Record:    record,
	})

	return nil
}

func (m *MongodbWriter) flush() error {
	m.mtx.Lock()
	if len(m.buffer) == 0 || m.client == nil {
		m.mtx.Unlock()
		return nil
	}
	buffer := m.buffer
	m.buffer = nil
	m.mtx.Unlock()

	_, err := m.collection.InsertMany(m.ctx, buffer)
	if err != nil {
		return err
	}

	return nil
}

func (m *MongodbWriter) Close() {
	m.flush()

	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.client != nil {
		m.cancel()
		m.client.Disconnect(m.ctx)
		m.client = nil
	}
}

type MongodbReader struct {
	ctx    context.Context
	client *mongo.Client
	cursor *mongo.Cursor
}

func NewMongodbReader(ctx context.Context, config *MongodbConfig) (RawReader, error) {
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

	collection := client.Database(config.Database).Collection(config.Collection)

	filter := bson.D{}
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}})

	cursor, err := collection.Find(context.Background(), filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find from mongodb: %w", err)
	}

	return &MongodbReader{
		ctx:    ctx,
		client: client,
		cursor: cursor,
	}, nil
}

func (m *MongodbReader) Read() (*time.Time, string, []byte, error) {
	if m.cursor == nil || !m.cursor.Next(m.ctx) {
		m.Close()
		return nil, "", nil, nil
	}

	var entry Entry
	if err := m.cursor.Decode(&entry); err != nil {
		return nil, "", nil, fmt.Errorf("failed to decode from mongodb: %w", err)
	}

	return &entry.Timestamp, entry.NodeID, entry.Record, nil
}

func (m *MongodbReader) Close() {
	if m.cursor != nil {
		m.cursor.Close(m.ctx)
		m.cursor = nil
	}

	if m.client != nil {
		m.client.Disconnect(m.ctx)
		m.client = nil
	}
}
