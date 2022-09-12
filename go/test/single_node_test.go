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
	"fmt"
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
	suite.node, err = colonio.NewColonio(colonio.DefaultLogger)
	suite.NoError(err)

	suite.T().Log("node connect to seed")
	suite.Eventually(func() bool {
		return suite.node.Connect("ws://localhost:8080/test", "") == nil
	}, time.Minute, time.Second)
}

func (suite *SingleNodeSuite) TearDownSuite() {
	suite.T().Log("node disconnect from seed")
	err := suite.node.Disconnect()
	suite.NoError(err)

	suite.T().Log("node quit colonio instance")
	err = suite.node.Quit()
	suite.NoError(err)
}

func (suite *SingleNodeSuite) TestSingle() {
	suite.T().Log("checking nid")
	suite.Equal(utf8.RuneCountInString(suite.node.GetLocalNid()), 32)

	receiveCount := 0
	suite.node.OnCall("expect", func(cp *colonio.CallParameter) interface{} {
		suite.T().Log("receive message expect")
		receiveCount++
		suite.Equal("expect", cp.Name)
		suite.Equal(uint32(0), cp.Options)

		param, err := cp.Value.GetString()
		suite.NoError(err)
		suite.Equal("expect param", param)

		result, err := colonio.NewValue("expect result")
		suite.NoError(err)
		return result
	})

	suite.node.OnCall("nearby", func(cp *colonio.CallParameter) interface{} {
		suite.T().Log("receive message nearby")
		receiveCount++
		suite.Equal("nearby", cp.Name)
		suite.Equal(colonio.ColonioCallAcceptNearby, cp.Options)

		param, err := cp.Value.GetString()
		suite.NoError(err)
		suite.Equal("nearby param", param)

		result, err := colonio.NewValue("nearby result")
		suite.NoError(err)
		return result
	})

	suite.node.OnCall("error", func(cp *colonio.CallParameter) interface{} {
		suite.T().Log("receive message error")
		receiveCount++
		suite.Equal("error", cp.Name)
		suite.Equal(uint32(0), cp.Options)

		param, err := cp.Value.GetString()
		suite.NoError(err)
		suite.Equal("error param", param)

		return fmt.Errorf("error result")
	})

	suite.T().Log("call message expect")
	result, err := suite.node.CallByNid(suite.node.GetLocalNid(), "expect", "expect param", 0)
	suite.NoError(err)
	resultStr, err := result.GetString()
	suite.NoError(err)
	suite.Equal("expect result", resultStr)

	suite.T().Log("call message nearby")
	result, err = suite.node.CallByNid("00000000000000000000000000000000", "nearby", "nearby param", colonio.ColonioCallAcceptNearby)
	suite.NoError(err)
	resultStr, err = result.GetString()
	suite.NoError(err)
	suite.Equal("nearby result", resultStr)

	/* TODO: fix error handling
	suite.T().Log("call message with error")
	result, err = suite.node.CallByNid(suite.node.GetLocalNid(), "error", "error param", 0)
	if suite.Error(err) {
		suite.Equal("error result", err.Error())
	}
	//*/

	suite.T().Log("checking count of calls")
	suite.Equal(2, receiveCount)
}
