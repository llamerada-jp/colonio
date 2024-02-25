//go:build !js

/*
 * Copyright 2017- Yuji Ito <llamerada.jp@gmail.com>
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

package e2e

import (
	"crypto/tls"
	"net/http"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var seed *exec.Cmd

func setup(t *testing.T) error {
	cur, _ := os.Getwd()
	seed = exec.Command(os.Getenv("COLONIO_SEED_BIN_PATH"), "--config", path.Join(cur, "..", "seed.json"))
	seed.Stdout = os.Stdout
	seed.Stderr = os.Stderr
	t.Log("start seed")
	err := seed.Start()
	if err != nil {
		return err
	}

	require.Eventually(t, func() bool {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
		_, err := client.Get("https://localhost:8080/")
		return err == nil
	}, 10*time.Second, time.Second)
	t.Log("check seed state OK")

	return nil
}

func teardown() error {
	return seed.Process.Kill()
}

func TestNative(t *testing.T) {
	err := setup(t)
	require.NoError(t, err)

	suite.Run(t, new(SingleNodeMessaging))
	suite.Run(t, new(E2eSuite))

	err = teardown()
	assert.NoError(t, err)
}
