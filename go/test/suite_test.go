package test

import (
	"testing"

	"github.com/llamerada-jp/colonio/go/colonio"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test Suite")
}

var _ = Describe("Colonio-go", func() {
	Context("e2e", func() {
		E2e(colonio.NewColonio)
	})
})
