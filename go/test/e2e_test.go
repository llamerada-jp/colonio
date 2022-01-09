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
	"os"
	"os/exec"
	"path"
	"runtime"
	"time"
	"unicode/utf8"

	"github.com/llamerada-jp/colonio/go/colonio"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func E2e(generator func(colonio.Logger) (colonio.Colonio, error)) {
	It("does E2E test", func() {
		if runtime.GOOS != "js" {
			By("starting seed for test")
			cur, _ := os.Getwd()
			cmd := exec.Command(os.Getenv("COLONIO_SEED_BIN_PATH"), "--config", path.Join(cur, "seed.json"))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Start()
			Expect(err).ShouldNot(HaveOccurred())
			defer func() {
				err = cmd.Process.Kill()
				Expect(err).ShouldNot(HaveOccurred())
			}()
		}

		By("creating a new colonio instance")
		node1, err := generator(colonio.DefaultLogger)
		Expect(err).ShouldNot(HaveOccurred())
		node2, err := generator(colonio.DefaultLogger)
		Expect(err).ShouldNot(HaveOccurred())

		By("node1 connect to seed")
		Eventually(func() error {
			return node1.Connect("ws://localhost:8080/test", "")
		}).ShouldNot(HaveOccurred())
		By("node2 connect to seed")
		Eventually(func() error {
			return node2.Connect("ws://localhost:8080/test", "")
		}).ShouldNot(HaveOccurred())

		By("checking nid")
		Expect(utf8.RuneCountInString(node1.GetLocalNid())).Should(Equal(32))

		Eventually(func() error {
			By("sending message and waiting it")
			node1.OnCall("twice", func(parameter *colonio.CallParameter) interface{} {
				str, err := parameter.Value.GetString()
				Expect(err).ShouldNot(HaveOccurred())
				return str + str
			})
			result, err := node2.CallByNid(node1.GetLocalNid(), "twice", "test", 0)
			if err != nil {
				return err
			}
			resultStr, err := result.GetString()
			if err != nil {
				return err
			}
			if resultStr != "testtest" {
				return fmt.Errorf("wrong text")
			}
			return nil
		}, time.Second*60, time.Second*1).Should(Succeed())

		By("node1 set 2D position")
		node1.SetPosition(1.0, 0.0)
		ps1, err := node1.AccessPubsub2D("ps")
		Expect(err).ShouldNot(HaveOccurred())

		By("node2 set 2D position")
		node2.SetPosition(2.0, 0.0)
		ps2, err := node2.AccessPubsub2D("ps")
		Expect(err).ShouldNot(HaveOccurred())

		// set 1 to avoid dead-lock caused by ps1.Publish waits finish of ps2.On.
		recvPS2 := make(chan string, 1)
		ps2.On("hoge", func(v colonio.Value) {
			str, err := v.GetString()
			Expect(err).ShouldNot(HaveOccurred())
			recvPS2 <- str
		})

		By("publishing message and waiting it")
		Eventually(func() error {
			select {
			case v, ok := <-recvPS2:
				Expect(ok).Should(BeTrue())
				Expect(v).Should(Equal("test publish"))
				return nil

			default:
				err := ps1.Publish("hoge", 2.0, 0.0, 1.1, "test publish", 0)
				Expect(err).ShouldNot(HaveOccurred())
				return fmt.Errorf("the message was not received")
			}
		}, time.Second*60, time.Second*1).Should(Succeed())

		By("getting a not existed value")
		map1, err := node1.AccessMap("map")
		Expect(err).ShouldNot(HaveOccurred())
		map2, err := node2.AccessMap("map")
		Expect(err).ShouldNot(HaveOccurred())

		_, err = map1.Get("key1")
		Expect(err).Should(MatchError(colonio.ErrNotExistKey))

		err = map2.Set("key1", "val1", colonio.MapErrorWithExist)
		Expect(err).ShouldNot(HaveOccurred())

		v, err := map1.Get("key1")
		Expect(v.GetString()).Should(Equal("val1"))
		Expect(err).ShouldNot(HaveOccurred())

		err = map2.Set("key1", "val2", colonio.MapErrorWithExist)
		Expect(err).Should(MatchError(colonio.ErrExistKey))

		err = map1.Set("key2", "val2", 0)
		Expect(err).ShouldNot(HaveOccurred())

		stored := make(map[string]string)
		map1.ForeachLocalValue(func(vKey, vValue colonio.Value, attr uint32) {
			key, _ := vKey.GetString()
			val, _ := vValue.GetString()
			_, ok := stored[key]
			Expect(ok).Should(BeFalse())
			stored[key] = val
		})
		map2.ForeachLocalValue(func(vKey, vValue colonio.Value, attr uint32) {
			key, _ := vKey.GetString()
			val, _ := vValue.GetString()
			_, ok := stored[key]
			Expect(ok).Should(BeFalse())
			stored[key] = val
		})
		Expect(stored).Should(HaveLen(2))
		Expect(stored["key1"]).Should(Equal("val1"))
		Expect(stored["key2"]).Should(Equal("val2"))

		By("node1 disconnect from seed")
		err = node1.Disconnect()
		Expect(err).ShouldNot(HaveOccurred())
		By("node2 disconnect from seed")
		err = node2.Disconnect()
		Expect(err).ShouldNot(HaveOccurred())

		By("node1 quit colonio instance")
		err = node1.Quit()
		Expect(err).ShouldNot(HaveOccurred())
		By("node2 quit colonio instance")
		err = node2.Quit()
		Expect(err).ShouldNot(HaveOccurred())
	})
}
