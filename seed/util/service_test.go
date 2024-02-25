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
package util

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceHTTP2(t *testing.T) {
	// create server
	cert := os.Getenv("COLONIO_TEST_CERT")
	key := os.Getenv("COLONIO_TEST_KEY")
	if cert == "" || key == "" {
		panic("Please set COLONIO_TEST_CERT and COLONIO_TEST_KEY")
	}

	// create service
	serviceConfig := &ServiceConfig{
		Headers:  map[string]string{},
		Port:     8080,
		CertFile: cert,
		KeyFile:  key,
		SeedPath: "/dummy",
	}
	service := NewService(serviceConfig, slog.Default())
	// set mux instead of seed as dummy
	service.SetHandler(http.NewServeMux())
	service.RootMux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// run service
	go func() {
		err := service.Run()
		require.NoError(t, err)
	}()
	defer service.Stop()
	service.WaitForRun()

	// access to service
	client := &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, err := http.NewRequest("GET", "https://localhost:8080/test", nil)
	assert.NoError(t, err)
	req = req.WithContext(context.Background())

	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "HTTP/2.0", resp.Proto)
}
