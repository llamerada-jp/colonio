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

package seed

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
	service "github.com/llamerada-jp/colonio/api/colonio/v1alpha/v1alphaconnect"
	"github.com/llamerada-jp/colonio/internal/shared"
	testUtil "github.com/llamerada-jp/colonio/test/util"
	"github.com/stretchr/testify/require"
)

func TestSeed(t *testing.T) {
	// Create a new Seed instance
	seed := NewSeed(
		WithLogger(testUtil.Logger(t)),
	)

	mux := http.NewServeMux()
	seed.RegisterService(mux)

	port := 8000 + uint16(rand.Uint32()%1000)
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Start the HTTP server in a goroutine
	cert := os.Getenv("COLONIO_TEST_CERT")
	key := os.Getenv("COLONIO_TEST_KEY")
	if cert == "" || key == "" {
		panic("Please set COLONIO_TEST_CERT and COLONIO_TEST_KEY")
	}

	go func() {
		err := httpSrv.ListenAndServeTLS(cert, key)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("HTTP server start error: %v", err)
		}
	}()

	// Shutdown the HTTP server when the test is done
	go func() {
		<-t.Context().Done()
		httpSrv.Shutdown(context.Background())
	}()

	// wait for the HTTP server to start
	require.Eventually(t, func() bool {
		client := testUtil.NewInsecureHttpClient()
		_, err := client.Get(fmt.Sprintf("https://localhost:%d", port))
		return err == nil
	}, 5*time.Second, 100*time.Millisecond)

	client := service.NewSeedServiceClient(
		testUtil.NewInsecureHttpClient(),
		fmt.Sprintf("https://localhost:%d", port),
	)

	// Try AssignNodeID
	res, err := client.AssignNode(t.Context(), &connect.Request[proto.AssignNodeRequest]{})
	require.NoError(t, err)
	nodeID, err := shared.NewNodeIDFromProto(res.Msg.GetNodeId())
	require.NoError(t, err)
	require.True(t, nodeID.IsNormal())

	// UnassignNodeID
	_, err = client.UnassignNode(t.Context(), &connect.Request[proto.UnassignNodeRequest]{})
	require.NoError(t, err)
}
