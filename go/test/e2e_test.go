package test

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"time"

	"github.com/llamerada-jp/colonio/go/colonio"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func E2e(generator func() (colonio.Colonio, error)) {
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
		node1, err := generator()
		Expect(err).ShouldNot(HaveOccurred())
		node2, err := generator()
		Expect(err).ShouldNot(HaveOccurred())

		By("node1 connect to seed")
		Eventually(func() error {
			return node1.Connect("ws://localhost:8080/test", "")
		}).ShouldNot(HaveOccurred())
		By("node2 connect to seed")
		Eventually(func() error {
			return node2.Connect("ws://localhost:8080/test", "")
		}).ShouldNot(HaveOccurred())

		By("node1 setup 2D info")
		node1.SetPosition(1.0, 0.0)
		ps1 := node1.AccessPubsub2D("ps")

		By("node2 setup 2D info")
		node2.SetPosition(2.0, 0.0)
		ps2 := node2.AccessPubsub2D("ps")
		// set 1 to avoid dead-lock caused by ps1.Publish waits finish of ps2.On.
		recv := make(chan string, 1)
		ps2.On("hoge", func(v colonio.Value) {
			str, err := v.GetString()
			Expect(err).ShouldNot(HaveOccurred())
			recv <- str
		})

		By("sending message and waiting it")
		Eventually(func() error {
			select {
			case v, ok := <-recv:
				Expect(ok).Should(BeTrue())
				Expect(v).Should(Equal("test"))
				return nil

			default:
				err := ps1.Publish("hoge", 2.0, 0.0, 1.1, "test", 0)
				Expect(err).ShouldNot(HaveOccurred())
				return fmt.Errorf("the message was not received")
			}
		}, time.Second*60, time.Second*1).Should(Succeed())

		By("getting a not existed value")
		map1 := node1.AccessMap("map")
		map2 := node2.AccessMap("map")

		_, err = map1.Get("key1")
		Expect(err).Should(MatchError(colonio.ErrNotExistKey))

		err = map2.Set("key1", "val1", colonio.MapErrorWithExist)
		Expect(err).ShouldNot(HaveOccurred())

		v, err := map1.Get("key1")
		Expect(v.GetString()).Should(Equal("val1"))
		Expect(err).ShouldNot(HaveOccurred())

		err = map2.Set("key1", "val2", colonio.MapErrorWithExist)
		Expect(err).Should(MatchError(colonio.ErrExistKey))

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
