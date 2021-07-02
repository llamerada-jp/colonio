package test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test Suite")
}

var _ = Describe("Colonio-go", func() {
	Context("e2e", testE2E)
})
