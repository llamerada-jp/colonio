//go:build js

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
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// constants should be equal to test/luncher/seed_transport.go
const (
	Request        = "🤖"
	responseNormal = "✋"
	responseError  = "🙅"

	SeedPort   = 8080
	NormalPath = "seed_transport_hello"
	ErrorPath  = "seed_transport_error"
)

func TestTransportWASM(t *testing.T) {
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
			url:    fmt.Sprintf("https://localhost:%d/%s", SeedPort, NormalPath),
			res:    []byte(responseNormal),
			status: http.StatusOK,
			err:    false,
		},
		{
			title: "error response",
			url:   fmt.Sprintf("https://localhost:%d/%s", SeedPort, ErrorPath),
			// http.Error using Println, so responseError has "\n"
			res:    []byte(responseError + "\n"),
			status: http.StatusInternalServerError,
			err:    false,
		},
		{
			title:  "offline",
			url:    fmt.Sprintf("https://localhost:%d/%s", SeedPort+1, NormalPath),
			res:    nil,
			status: 0,
			err:    true,
		},
	}
	tr := NewSeedTransportWASM(&SeedTransporterOption{
		Verification: false,
	})

	for _, c := range cases {
		t.Log(c.title)
		res, status, err := tr.Send(ctx, c.url, []byte(Request))
		assert.Equal(t, c.res, res)
		assert.Equal(t, c.status, status)
		if c.err {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}
