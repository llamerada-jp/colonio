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
	"log"
	"log/slog"
	"time"

	"github.com/llamerada-jp/colonio"
	"github.com/llamerada-jp/colonio/config"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/suite"
)

type E2eSuite struct {
	suite.Suite
	seedURL string
	node1   colonio.Colonio
	node2   colonio.Colonio
}

func (suite *E2eSuite) SetupSuite() {
	var err error
	suite.T().Log("creating a new colonio instance")
	suite.node1, err = colonio.NewColonio(
		colonio.WithLogger(slog.Default().With(slog.String("node", "node1"))),
		colonio.WithSeedURL(suite.seedURL),
		colonio.WithICEServers([]config.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		}),
		colonio.WithHttpClient(testUtil.NewInsecureHttpClient()),
		colonio.WithSphereGeometry(6378137.0))
	suite.NoError(err)

	suite.node2, err = colonio.NewColonio(
		colonio.WithLogger(slog.Default().With(slog.String("node", "node2"))),
		colonio.WithSeedURL(suite.seedURL),
		colonio.WithICEServers([]config.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		}),
		colonio.WithHttpClient(testUtil.NewInsecureHttpClient()),
		colonio.WithSphereGeometry(6378137.0))
	suite.NoError(err)

	suite.T().Log("node1 connect to seed")
	err = suite.node1.Start(context.Background())
	suite.NoError(err)
	suite.T().Log("node2 connect to seed")
	err = suite.node2.Start(context.Background())
	suite.NoError(err)
	suite.Eventually(func() bool {
		return suite.node1.IsOnline() && suite.node2.IsOnline()
	}, 60*time.Second, time.Second)
}

func (suite *E2eSuite) TearDownSuite() {
	suite.T().Log("node1 disconnect from seed")
	suite.node1.Stop()
	suite.T().Log("node2 disconnect from seed")
	suite.node2.Stop()
}

func (suite *E2eSuite) TestE2E() {
	suite.T().Log("sending message and waiting it")
	suite.node1.MessagingSetHandler("twice", func(request *colonio.MessagingRequest, writer colonio.MessagingResponseWriter) {
		writer.Write(append(request.Message, request.Message...))
	})
	defer suite.node1.MessagingUnsetHandler("twice")

	suite.Eventually(func() bool {
		result, err := suite.node2.MessagingPost(suite.node1.GetLocalNodeID(), "twice", []byte("test"))
		if err != nil {
			log.Println(err)
			return false
		}
		suite.Equal("testtest", string(result))
		return true
	}, 3*time.Minute, time.Second)

	suite.T().Log("node1 set 2D position")
	suite.node1.UpdateLocalPosition(1.0, 0.0)

	suite.T().Log("node2 set 2D position")
	suite.node2.UpdateLocalPosition(2.0, 0.0)

	// set 1 to avoid dead-lock caused by ps1.Publish waits finish of ps2.On.
	recvPS2 := make(chan string, 1)
	suite.node2.SpreadSetHandler("hoge", func(request *colonio.SpreadRequest) {
		recvPS2 <- string(request.Message)
	})
	defer suite.node2.SpreadUnsetHandler("hoge")

	suite.T().Log("publishing message and waiting it")
	suite.Eventually(func() bool {
		select {
		case v, ok := <-recvPS2:
			suite.True(ok)
			suite.Equal("test publish", v)
			return true

		default:
			err := suite.node1.SpreadPost(2.0, 0.0, 1.1, "hoge", []byte("test publish"))
			suite.NoError(err)
			log.Println("the message was not received")
			return false
		}
	}, 3*time.Minute, time.Second)

	suite.T().Log("getting a not existed value")

	/*
		_, err := suite.node1.KvsGet("key1")
		if suite.Error(err) {
			suite.ErrorIs(colonio.ErrKvsNotFound, err)
		}

		err = suite.node2.KvsSet("key1", "val1", colonio.KvsProhibitOverwrite)
		suite.NoError(err)

		v, err := suite.node1.KvsGet("key1")
		suite.NoError(err)
		s, err := v.GetString()
		suite.NoError(err)
		suite.Equal("val1", s)

		err = suite.node2.KvsSet("key1", "val2", colonio.KvsProhibitOverwrite)
		if suite.Error(err) {
			suite.ErrorIs(colonio.ErrKvsProhibitOverwrite, err)
		}

		err = suite.node1.KvsSet("key2", "val2", 0)
		suite.NoError(err)

		stored := make(map[string]string)
		kld := suite.node1.KvsGetLocalData()
		for _, key := range kld.GetKeys() {
			val, err := kld.GetValue(key)
			suite.NoError(err)
			str, err := val.GetString()
			suite.NoError(err)
			_, ok := stored[key]
			suite.False(ok)
			stored[key] = str
		}
		kld.Free()
		kld = suite.node2.KvsGetLocalData()
		for _, key := range kld.GetKeys() {
			val, err := kld.GetValue(key)
			suite.NoError(err)
			str, err := val.GetString()
			suite.NoError(err)
			_, ok := stored[key]
			suite.False(ok)
			stored[key] = str
		}
		kld.Free()
		suite.Len(stored, 2)
		suite.Equal("val1", stored["key1"])
		suite.Equal("val2", stored["key2"])
		//*/
}

/*
func (suite *E2eSuite) TestGetSetInCB() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				log.Println("Hi!")
			}
		}
	}()

	// fatal error: all goroutines are asleep - deadlock!
	suite.node1.MessagingSetHandler("set", func(request *types.MessagingRequest, writer types.MessagingResponseWriter) {
		err := suite.node1.KvsSet("key", request.Message, 0)
		suite.NoError(err)
		writer.Write(true)
	})
	defer suite.node1.MessagingUnsetHandler("set")

	suite.node1.MessagingSetHandler("get", func(request *colonio.MessagingRequest, writer colonio.MessagingResponseWriter) {
		v, err := suite.node1.KvsGet("key")
		suite.NoError(err)
		writer.Write(v)
	})
	defer suite.node1.MessagingUnsetHandler("get")

	_, err := suite.node2.MessagingPost(suite.node1.GetLocalNid(), "set", "dummy value", 0)
	suite.NoError(err)
	v, err := suite.node2.MessagingPost(suite.node1.GetLocalNid(), "get", "", 0)
	suite.NoError(err)
	s, err := v.GetString()
	suite.NoError(err)
	suite.Equal(s, "dummy value")
}
//*/
