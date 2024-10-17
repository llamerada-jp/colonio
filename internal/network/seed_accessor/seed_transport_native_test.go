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

package seed_accessor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/llamerada-jp/colonio/test/testing_seed"
	"github.com/stretchr/testify/assert"
)

const (
	request        = "🤖"
	responseNormal = "✋"
	responseError  = "🙅"
)

func normalHandler(t *testing.T, w http.ResponseWriter, r *http.Request) {
	bin, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, request, string(bin))
	w.Header().Set("Content-Type", "application/octet-stream")
	_, err = w.Write([]byte(responseNormal))
	assert.NoError(t, err)
}

func errorHandler(_ *testing.T, w http.ResponseWriter, r *http.Request) {
	http.Error(w, responseError, http.StatusInternalServerError)
}

func TestTransportNative(t *testing.T) {
	seed := testing_seed.NewTestingSeed(
		testing_seed.WithHandleTestingFunc(t, "/hello", normalHandler),
		testing_seed.WithHandleTestingFunc(t, "/error", errorHandler),
	)
	defer seed.Stop()

	ctx := context.Background()

	cases := []struct {
		title  string
		url    string
		res    []byte
		status int
		err    bool
	}{
		{
			title:  "normal case",
			url:    fmt.Sprintf("https://localhost:%d/hello", seed.Port()),
			res:    []byte(responseNormal),
			status: http.StatusOK,
			err:    false,
		},
		{
			title: "error response",
			url:   fmt.Sprintf("https://localhost:%d/error", seed.Port()),
			// http.Error using Println, so responseError has "\n"
			res:    []byte(responseError + "\n"),
			status: http.StatusInternalServerError,
			err:    false,
		},
		{
			title:  "offline",
			url:    fmt.Sprintf("https://localhost:%d/hello", seed.Port()+1),
			res:    nil,
			status: 0,
			err:    true,
		},
	}
	tr := NewSeedTransportNative(&SeedTransporterOption{
		Verification: false,
	})

	for _, c := range cases {
		t.Log(c.title)
		res, status, err := tr.Send(ctx, c.url, []byte(request))
		assert.Equal(t, c.res, res)
		assert.Equal(t, c.status, status)
		if c.err {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}
