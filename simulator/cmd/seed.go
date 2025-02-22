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
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	sd "github.com/llamerada-jp/colonio/seed"
	"github.com/spf13/cobra"
)

var (
	httpPort     uint16
	httpCertFile string
	httpKeyFile  string
)

var seedCmd = &cobra.Command{
	Use: "seed",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))

		mux := http.NewServeMux()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		seed := sd.NewSeed(sd.WithLogger(logger))
		seed.RegisterService(mux)
		go seed.Run(ctx)

		server := &http.Server{
			Addr:    fmt.Sprintf(":%d", httpPort),
			Handler: mux,
		}

		logger.Info("Seed server started", slog.Uint64("port", uint64(httpPort)))

		return server.ListenAndServeTLS(httpCertFile, httpKeyFile)
	},
}

func init() {
	flags := nodeCmd.PersistentFlags()
	flags.Uint16Var(&httpPort, "port", 8443, "HTTP port")
	flags.StringVar(&httpCertFile, "cert", "seed.crt", "HTTP server Certificate file")
	flags.StringVar(&httpKeyFile, "key", "seed.key", "HTTP server Key file")

	rootCmd.AddCommand(seedCmd)
}
