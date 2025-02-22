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
package seed

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var ErrSessionNotFound = errors.New("session not found")

type SessionStore interface {
	Get(ctx context.Context, stream grpc.ServerStream) (map[string]string, error)
	Save(ctx context.Context, stream grpc.ServerStream, session map[string]string) error
	Delete(ctx context.Context, stream grpc.ServerStream) error
}

type sessionRecord struct {
	timestamp time.Time
	data      map[string]string
}

type memoryStore struct {
	sessionName string
	maxAge      time.Duration
	mutex       sync.Mutex
	// session id -> session entry
	sessions map[string]*sessionRecord
}

func NewSessionMemoryStore(ctx context.Context, sessionName string, maxAge time.Duration) SessionStore {
	store := &memoryStore{
		sessionName: sessionName,
		maxAge:      maxAge,
		sessions:    make(map[string]*sessionRecord),
	}

	ticker := time.NewTicker(maxAge / 2)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				store.cleanup()
			}
		}
	}()

	return store
}

func (s *memoryStore) Get(ctx context.Context, stream grpc.ServerStream) (map[string]string, error) {
	sessionID := s.getSessionID(ctx)
	if sessionID == "" {
		return nil, ErrSessionNotFound
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	record, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	if time.Since(record.timestamp) > s.maxAge {
		go func() {
			s.Delete(ctx, stream)
		}()
		return nil, ErrSessionNotFound
	}

	record.timestamp = time.Now()

	return record.data, nil
}

func (s *memoryStore) Save(ctx context.Context, stream grpc.ServerStream, session map[string]string) error {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const sessionIDLength = 32

	sessionID := s.getSessionID(ctx)

	func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		// regenerate session id if session id is invalid
		if len(sessionID) != 0 {
			_, ok := s.sessions[sessionID]
			if !ok {
				sessionID = ""
			}
		}

		if len(sessionID) == 0 {
			for {
				sessionID = ""
				for i := 0; i < sessionIDLength; i++ {
					sessionID += string(charset[rand.Intn(len(charset))])
				}
				if _, ok := s.sessions[sessionID]; !ok {
					break
				}
			}
		}

		s.sessions[sessionID] = &sessionRecord{
			timestamp: time.Now(),
			data:      session,
		}
	}()

	md := metadata.New(map[string]string{s.sessionName: sessionID})
	var err error
	if stream != nil {
		err = stream.SetHeader(md)
	} else {
		err = grpc.SetHeader(ctx, md)
	}
	if err != nil {
		return fmt.Errorf("failed to set header: %w", err)
	}
	return nil
}

func (s *memoryStore) Delete(ctx context.Context, stream grpc.ServerStream) error {
	sessionID := s.getSessionID(ctx)
	if sessionID == "" {
		return nil
	}

	s.mutex.Lock()
	delete(s.sessions, sessionID)
	s.mutex.Unlock()

	md := metadata.New(map[string]string{s.sessionName: sessionID})
	var err error
	if stream != nil {
		err = stream.SetHeader(md)
	} else {
		err = grpc.SetHeader(ctx, md)
	}
	if err != nil {
		return fmt.Errorf("failed to set header: %w", err)
	}
	return nil
}

func (s *memoryStore) cleanup() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for sessionID, record := range s.sessions {
		if time.Since(record.timestamp) > s.maxAge {
			delete(s.sessions, sessionID)
		}
	}
}

func (s *memoryStore) getSessionID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	sessionIDs := md.Get(s.sessionName)
	if len(sessionIDs) != 1 || len(sessionIDs[0]) == 0 {
		return ""
	}

	return sessionIDs[0]
}
