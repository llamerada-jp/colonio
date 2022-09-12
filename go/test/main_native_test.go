//go:build !js

package test

import (
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

var seed *exec.Cmd

func setup() error {
	cur, _ := os.Getwd()
	seed = exec.Command(os.Getenv("COLONIO_SEED_BIN_PATH"), "--config", path.Join(cur, "seed.json"))
	seed.Stdout = os.Stdout
	seed.Stderr = os.Stderr
	return seed.Start()
}

func teardown() error {
	return seed.Process.Kill()
}

func TestNative(t *testing.T) {
	err := setup()
	assert.NoError(t, err)

	suite.Run(t, new(SingleNodeSuite))
	suite.Run(t, new(E2eSuite))

	err = teardown()
	assert.NoError(t, err)
}
