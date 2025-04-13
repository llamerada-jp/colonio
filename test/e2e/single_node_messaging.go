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
package e2e

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/llamerada-jp/colonio"
	"github.com/llamerada-jp/colonio/internal/constants"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/suite"
)

type SingleNodeMessaging struct {
	suite.Suite
	seedURL string
	node    colonio.Colonio
}

func (suite *SingleNodeMessaging) SetupSuite() {
	var err error
	suite.T().Log("creating a new colonio instance")
	suite.node, err = colonio.NewColonio(
		colonio.WithSeedURL(suite.seedURL),
		colonio.WithICEServers(constants.TestingICEServers),
		colonio.WithHttpClient(testUtil.NewInsecureHttpClient()),
		colonio.WithSphereGeometry(6378137.0))
	suite.NoError(err)
	err = suite.node.Start(context.Background())
	suite.NoError(err)
	suite.Eventually(func() bool {
		return suite.node.IsOnline()
	}, 10*time.Second, time.Second)
}

func (suite *SingleNodeMessaging) TearDownSuite() {
	suite.T().Log("node disconnect from seed")
	suite.node.Stop()
}

func (suite *SingleNodeMessaging) TestSingle() {
	suite.T().Log("checking nid")
	suite.Equal(utf8.RuneCountInString(suite.node.GetLocalNodeID()), 32)

	receiveCount := 0
	suite.node.MessagingSetHandler("expect", func(request *colonio.MessagingRequest, writer colonio.MessagingResponseWriter) {
		suite.T().Log("receive message expect")
		receiveCount++

		suite.Equal(suite.node.GetLocalNodeID(), request.SourceNodeID)
		suite.Equal([]byte("expect message"), request.Message)
		suite.False(request.Options.AcceptNearby)
		suite.False(request.Options.IgnoreResponse)

		writer.Write([]byte("expect response"))
	})

	suite.node.MessagingSetHandler("nearby", func(request *colonio.MessagingRequest, writer colonio.MessagingResponseWriter) {
		suite.T().Log("receive message nearby")
		receiveCount++

		suite.Equal([]byte("nearby message"), request.Message)
		suite.True(request.Options.AcceptNearby)
		suite.False(request.Options.IgnoreResponse)

		writer.Write([]byte("nearby response"))
	})

	suite.T().Log("call message expect")
	response, err := suite.node.MessagingPost(suite.node.GetLocalNodeID(), "expect", []byte("expect message"))
	suite.NoError(err)
	suite.Equal([]byte("expect response"), response)

	suite.T().Log("call message nearby")
	response, err = suite.node.MessagingPost("00000000000000000000000000000000", "nearby", []byte("nearby message"), colonio.MessagingWithAcceptNearby())
	suite.NoError(err)
	suite.Equal([]byte("nearby response"), response)

	suite.T().Log("call message with error")
	response, err = suite.node.MessagingPost(suite.node.GetLocalNodeID(), "not exist", []byte("dummy"))
	suite.Nil(response)
	suite.Error(err)

	suite.T().Log("checking count of calls")
	suite.Equal(2, receiveCount)
}
