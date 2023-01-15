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
	"context"
	"log"
	"time"

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
	suite.node1, err = colonio.NewColonio(colonio.NewConfig())
	suite.NoError(err)
	suite.node2, err = colonio.NewColonio(colonio.NewConfig())
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
}

func (suite *E2eSuite) TestE2E() {
	suite.Eventually(func() bool {
		suite.T().Log("sending message and waiting it")
		suite.node1.MessagingSetHandler("twice", func(request *colonio.MessagingRequest, writer colonio.MessagingResponseWriter) {
			str, err := request.Message.GetString()
			suite.NoError(err)
			writer.Write(str + str)
		})
		defer suite.node1.MessagingUnsetHandler("twice")

		result, err := suite.node2.MessagingPost(suite.node1.GetLocalNid(), "twice", "test", 0)
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

	suite.T().Log("node2 set 2D position")
	suite.node2.SetPosition(2.0, 0.0)

	// set 1 to avoid dead-lock caused by ps1.Publish waits finish of ps2.On.
	recvPS2 := make(chan string, 1)
	suite.node2.SpreadSetHandler("hoge", func(request *colonio.SpreadRequest) {
		str, err := request.Message.GetString()
		suite.NoError(err)
		recvPS2 <- str
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
			err := suite.node1.SpreadPost(2.0, 0.0, 1.1, "hoge", "test publish", 0)
			suite.NoError(err)
			log.Println("the message was not received")
			return false
		}
	}, time.Minute, time.Second)

	suite.T().Log("getting a not existed value")

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
}

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
	suite.node1.MessagingSetHandler("set", func(request *colonio.MessagingRequest, writer colonio.MessagingResponseWriter) {
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

func (suite *E2eSuite) TestPassValues() {
	suite.node1.MessagingSetHandler("nil", func(mr *colonio.MessagingRequest, mrw colonio.MessagingResponseWriter) {
		suite.True(mr.Message.IsNil())
		mrw.Write(nil)
	})

	suite.node1.MessagingSetHandler("bool", func(mr *colonio.MessagingRequest, mrw colonio.MessagingResponseWriter) {
		suite.True(mr.Message.IsBool())
		v, err := mr.Message.GetBool()
		suite.NoError(err)
		suite.Equal(v, true)
		mrw.Write(false)
	})

	suite.node1.MessagingSetHandler("int", func(mr *colonio.MessagingRequest, mrw colonio.MessagingResponseWriter) {
		suite.True(mr.Message.IsInt())
		v, err := mr.Message.GetInt()
		suite.NoError(err)
		suite.Equal(int(v), 1)
		mrw.Write(2)
	})

	suite.node1.MessagingSetHandler("double", func(mr *colonio.MessagingRequest, mrw colonio.MessagingResponseWriter) {
		suite.True(mr.Message.IsDouble())
		v, err := mr.Message.GetDouble()
		suite.NoError(err)
		suite.Equal(v, 3.14)
		mrw.Write(2.71)
	})

	suite.node1.MessagingSetHandler("string", func(mr *colonio.MessagingRequest, mrw colonio.MessagingResponseWriter) {
		suite.True(mr.Message.IsString())
		v, err := mr.Message.GetString()
		suite.NoError(err)
		suite.Equal(v, "hello☀\n")
		mrw.Write("world🌏\n")
	})

	suite.node1.MessagingSetHandler("binary", func(mr *colonio.MessagingRequest, mrw colonio.MessagingResponseWriter) {
		suite.True(mr.Message.IsBinary())
		v, err := mr.Message.GetBinary()
		suite.NoError(err)
		suite.Equal(string(v), "🤩👾")
		mrw.Write([]byte("漏電対策"))
	})

	vNil, err := suite.node2.MessagingPost(suite.node1.GetLocalNid(), "nil", nil, 0)
	suite.NoError(err)
	suite.True(vNil.IsNil())

	vBool, err := suite.node2.MessagingPost(suite.node1.GetLocalNid(), "bool", true, 0)
	suite.NoError(err)
	suite.True(vBool.IsBool())
	nBool, err := vBool.GetBool()
	suite.NoError(err)
	suite.Equal(nBool, false)

	vInt, err := suite.node2.MessagingPost(suite.node1.GetLocalNid(), "int", 1, 0)
	suite.NoError(err)
	suite.True(vInt.IsInt())
	nInt, err := vInt.GetInt()
	suite.NoError(err)
	suite.Equal(int(nInt), 2)

	vDouble, err := suite.node2.MessagingPost(suite.node1.GetLocalNid(), "double", 3.14, 0)
	suite.NoError(err)
	suite.True(vDouble.IsDouble())
	nDouble, err := vDouble.GetDouble()
	suite.NoError(err)
	suite.Equal(nDouble, 2.71)

	vString, err := suite.node2.MessagingPost(suite.node1.GetLocalNid(), "string", "hello☀\n", 0)
	suite.NoError(err)
	suite.True(vString.IsString())
	nString, err := vString.GetString()
	suite.NoError(err)
	suite.Equal(nString, "world🌏\n")

	vBinary, err := suite.node2.MessagingPost(suite.node1.GetLocalNid(), "binary", []byte("🤩👾"), 0)
	suite.NoError(err)
	suite.True(vBinary.IsBinary())
	nBinary, err := vBinary.GetBinary()
	suite.NoError(err)
	suite.Equal(string(nBinary), "漏電対策")
}
