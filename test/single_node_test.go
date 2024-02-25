package test

/*
 * Copyright 2017 Yuji Ito <llamerada.jp@gmail.com>
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

import (
	"crypto/tls"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/llamerada-jp/colonio/go/colonio"
	"github.com/stretchr/testify/suite"
)

type SingleNodeSuite struct {
	suite.Suite
	node colonio.Colonio
}

func (suite *SingleNodeSuite) SetupSuite() {
	var err error
	suite.T().Log("creating a new colonio instance")
	config := colonio.NewConfig()
	config.DisableSeedVerification = true
	suite.node, err = colonio.NewColonio(config)
	suite.NoError(err)

	suite.T().Log("node connect to seed")
	suite.Eventually(func() bool {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
		_, err := client.Get("https://localhost:8080/")
		return err == nil
	}, 10*time.Second, time.Second)

	err = suite.node.Connect("https://localhost:8080/test", "")
	suite.NoError(err)
}

func (suite *SingleNodeSuite) TearDownSuite() {
	suite.T().Log("node disconnect from seed")
	err := suite.node.Disconnect()
	suite.NoError(err)
}

func (suite *SingleNodeSuite) TestSingle() {
	suite.T().Log("checking nid")
	suite.Equal(utf8.RuneCountInString(suite.node.GetLocalNid()), 32)

	receiveCount := 0
	suite.node.MessagingSetHandler("expect", func(request *colonio.MessagingRequest, writer colonio.MessagingResponseWriter) {
		suite.T().Log("receive message expect")
		receiveCount++
		suite.Equal(suite.node.GetLocalNid(), request.SourceNid)
		suite.Equal(uint32(0), request.Options)

		param, err := request.Message.GetString()
		suite.NoError(err)
		suite.Equal("expect param", param)

		result, err := colonio.NewValue("expect result")
		suite.NoError(err)
		writer.Write(result)
	})

	suite.node.MessagingSetHandler("nearby", func(request *colonio.MessagingRequest, writer colonio.MessagingResponseWriter) {
		suite.T().Log("receive message nearby")
		receiveCount++
		suite.Equal(colonio.MessagingAcceptNearby, request.Options)

		param, err := request.Message.GetString()
		suite.NoError(err)
		suite.Equal("nearby param", param)

		result, err := colonio.NewValue("nearby result")
		suite.NoError(err)
		writer.Write(result)
	})

	/* TODO: implement error handling
	suite.node.MessagingSetHandler("error", func(request *colonio.MessagingRequest, writer colonio.MessagingResponseWriter) {
		suite.T().Log("receive message error")
		receiveCount++
		suite.Equal(uint32(0), request.Options)

		param, err := request.Message.GetString()
		suite.NoError(err)
		suite.Equal("error param", param)

		return fmt.Errorf("error result")
	})
	//*/

	suite.T().Log("call message expect")
	result, err := suite.node.MessagingPost(suite.node.GetLocalNid(), "expect", "expect param", 0)
	suite.NoError(err)
	resultStr, err := result.GetString()
	suite.NoError(err)
	suite.Equal("expect result", resultStr)

	suite.T().Log("call message nearby")
	result, err = suite.node.MessagingPost("00000000000000000000000000000000", "nearby", "nearby param", colonio.MessagingAcceptNearby)
	suite.NoError(err)
	resultStr, err = result.GetString()
	suite.NoError(err)
	suite.Equal("nearby result", resultStr)

	/* TODO: implement error handling
	suite.T().Log("call message with error")
	result, err = suite.node.CallByNid(suite.node.GetLocalNid(), "error", "error param", 0)
	if suite.Error(err) {
		suite.Equal("error result", err.Error())
	}
	//*/

	suite.T().Log("checking count of calls")
	suite.Equal(2, receiveCount)
}
