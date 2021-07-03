package test

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/llamerada-jp/colonio/pkg/colonio"
	colonio_if "github.com/llamerada-jp/colonio/pkg/interfaces"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func testE2E() {
	It("does E2E test", func() {
		By("starting seed for test")
		cur, _ := os.Getwd()
		cmd := exec.Command(os.Getenv("COLONIO_SEED_BIN_PATH"), "--config", cur+"/seed_config.json")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Start()
		Expect(err).ShouldNot(HaveOccurred())
		defer func() {
			err = cmd.Process.Kill()
			Expect(err).ShouldNot(HaveOccurred())
		}()

		By("creating a new colonio instance")
		node1, err := colonio.NewColonio()
		node2, err := colonio.NewColonio()
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
		recv := make(chan string)
		ps2.On("hoge", func(v colonio_if.Value) {
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
