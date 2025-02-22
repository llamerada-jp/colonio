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
	"fmt"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/llamerada-jp/colonio/internal/shared"
)

const (
	COOKIE_NAME_SESSION = "colonio_session"
	SESSION_KEY_NODE_ID = "nodeID"
)

type session struct {
	sessionStore sessions.Store
	session      *sessions.Session
	request      *http.Request
	response     http.ResponseWriter
}

func newSession(sessionStore sessions.Store, request *http.Request, response http.ResponseWriter) (*session, error) {
	s, err := sessionStore.Get(request, COOKIE_NAME_SESSION)
	if err != nil {
		s, err = sessionStore.New(request, COOKIE_NAME_SESSION)
		if err != nil {
			return nil, fmt.Errorf("failed to create session: %w", err)
		}
	}

	return &session{
		sessionStore: sessionStore,
		session:      s,
		request:      request,
		response:     response,
	}, nil
}

func (s *session) getNodeID() *shared.NodeID {
	if s == nil {
		return nil
	}

	value, ok := s.session.Values[SESSION_KEY_NODE_ID].(string)
	if !ok {
		return nil
	}

	nodeID := shared.NewNodeIDFromString(value)
	if nodeID == nil || !nodeID.IsNormal() {
		return nil
	}

	return nodeID
}

func (s *session) setNodeID(nodeID *shared.NodeID) {
	if s == nil {
		return
	}

	s.session.Values[SESSION_KEY_NODE_ID] = nodeID.String()
}

func (s *session) write() error {
	return s.sessionStore.Save(s.request, s.response, s.session)
}
