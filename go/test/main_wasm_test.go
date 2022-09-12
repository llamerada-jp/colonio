//go:build js

package test

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestNative(t *testing.T) {
	suite.Run(t, new(SingleNodeSuite))
	suite.Run(t, new(E2eSuite))
}
