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
	"log"
	"time"
	"unicode/utf8"

	"github.com/llamerada-jp/colonio/go/colonio"
	"github.com/stretchr/testify/suite"
)

type E2eSuite struct {
	suite.Suite
	node1 colonio.Colonio
	node2 colonio.Colonio
}

func (suite *E2eSuite) SetupSuite() {
	var err error
	suite.T().Log("creating a new colonio instance")
	suite.node1, err = colonio.NewColonio(colonio.DefaultLogger)
	suite.NoError(err)
	suite.node2, err = colonio.NewColonio(colonio.DefaultLogger)
	suite.NoError(err)

	suite.T().Log("node1 connect to seed")
	suite.Eventually(func() bool {
		return suite.node1.Connect("ws://localhost:8080/test", "") == nil
	}, time.Minute, time.Second)
	suite.T().Log("node2 connect to seed")
	suite.Eventually(func() bool {
		return suite.node2.Connect("ws://localhost:8080/test", "") == nil
	}, time.Minute, time.Second)
}

func (suite *E2eSuite) TearDownSuite() {
	suite.T().Log("node1 disconnect from seed")
	err := suite.node1.Disconnect()
	suite.NoError(err)
	suite.T().Log("node2 disconnect from seed")
	err = suite.node2.Disconnect()
	suite.NoError(err)

	suite.T().Log("node1 quit colonio instance")
	err = suite.node1.Quit()
	suite.NoError(err)
	suite.T().Log("node2 quit colonio instance")
	err = suite.node2.Quit()
	suite.NoError(err)
}

func (suite *E2eSuite) TestE2E() {
	suite.T().Log("checking nid")
	suite.Equal(utf8.RuneCountInString(suite.node1.GetLocalNid()), 32)

	suite.Eventually(func() bool {
		suite.T().Log("sending message and waiting it")
		suite.node1.OnCall("twice", func(parameter *colonio.CallParameter) interface{} {
			str, err := parameter.Value.GetString()
			suite.NoError(err)
			return str + str
		})
		result, err := suite.node2.CallByNid(suite.node1.GetLocalNid(), "twice", "test", 0)
		if err != nil {
			log.Println(err)
			return false
		}
		resultStr, err := result.GetString()
		if err != nil {
			log.Println(err)
			return false
		}
		suite.Equal("testtest", resultStr)
		return true
	}, time.Minute, time.Second)

	suite.T().Log("node1 set 2D position")
	suite.node1.SetPosition(1.0, 0.0)
	ps1, err := suite.node1.AccessPubsub2D("ps")
	suite.NoError(err)

	suite.T().Log("node2 set 2D position")
	suite.node2.SetPosition(2.0, 0.0)
	ps2, err := suite.node2.AccessPubsub2D("ps")
	suite.NoError(err)

	// set 1 to avoid dead-lock caused by ps1.Publish waits finish of ps2.On.
	recvPS2 := make(chan string, 1)
	ps2.On("hoge", func(v colonio.Value) {
		str, err := v.GetString()
		suite.NoError(err)
		recvPS2 <- str
	})

	suite.T().Log("publishing message and waiting it")
	suite.Eventually(func() bool {
		select {
		case v, ok := <-recvPS2:
			suite.True(ok)
			suite.Equal("test publish", v)
			return true

		default:
			err := ps1.Publish("hoge", 2.0, 0.0, 1.1, "test publish", 0)
			suite.NoError(err)
			log.Println("the message was not received")
			return false
		}
	}, time.Minute, time.Second)

	suite.T().Log("getting a not existed value")
	map1, err := suite.node1.AccessMap("map")
	suite.NoError(err)
	map2, err := suite.node2.AccessMap("map")
	suite.NoError(err)

	_, err = map1.Get("key1")
	if suite.Error(err) {
		suite.ErrorIs(colonio.ErrNotExistKey, err)
	}

	err = map2.Set("key1", "val1", colonio.MapErrorWithExist)
	suite.NoError(err)

	v, err := map1.Get("key1")
	suite.NoError(err)
	s, err := v.GetString()
	suite.NoError(err)
	suite.Equal("val1", s)

	err = map2.Set("key1", "val2", colonio.MapErrorWithExist)
	if suite.Error(err) {
		suite.ErrorIs(colonio.ErrExistKey, err)
	}

	err = map1.Set("key2", "val2", 0)
	suite.NoError(err)

	stored := make(map[string]string)
	map1.ForeachLocalValue(func(vKey, vValue colonio.Value, attr uint32) {
		key, _ := vKey.GetString()
		val, _ := vValue.GetString()
		_, ok := stored[key]
		suite.False(ok)
		stored[key] = val
	})
	map2.ForeachLocalValue(func(vKey, vValue colonio.Value, attr uint32) {
		key, _ := vKey.GetString()
		val, _ := vValue.GetString()
		_, ok := stored[key]
		suite.False(ok)
		stored[key] = val
	})
	suite.Len(stored, 2)
	suite.Equal("val1", stored["key1"])
	suite.Equal("val2", stored["key2"])
}
