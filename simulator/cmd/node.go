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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/llamerada-jp/colonio/simulator/circle"
	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/llamerada-jp/colonio/simulator/sphere"
	"github.com/spf13/cobra"
)

var nodeConfig = struct {
	seedURL       string
	story         string
	concurrency   uint
	writer        string
	mongodbConfig datastore.MongodbConfig
}{}

var nodeCmd = &cobra.Command{
	Use: "node",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(nodeConfig.seedURL) == 0 {
			return errors.New("seed URL should be set")
		}

		cmd.SilenceUsage = true

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var rawWriter datastore.RawWriter
		switch nodeConfig.writer {
		case "mongodb":
			mongo, err := datastore.NewMongodbWriter(ctx, &nodeConfig.mongodbConfig)
			if err != nil {
				return fmt.Errorf("failed to create datastore: %w", err)
			}
			rawWriter = mongo

		case "stdout":
			rawWriter = datastore.NewStdWriter()

		default:
			rawWriter = datastore.NewNoneWriter()
		}

		writer := datastore.NewWriter(rawWriter)
		defer writer.Close()

		wg := &sync.WaitGroup{}
		wg.Add(int(nodeConfig.concurrency))

		mtx := &sync.Mutex{}
		var globalErr error

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			<-sig
			cancel()
		}()

		logger.Info("start node",
			slog.String("seedURL", nodeConfig.seedURL),
			slog.String("story", nodeConfig.story),
			slog.Uint64("concurrency", uint64(nodeConfig.concurrency)),
			slog.String("mongodbURI", nodeConfig.mongodbConfig.URI))

		for i := uint(0); i < nodeConfig.concurrency; i++ {
			go func() {
				if err := run(ctx, logger.With(slog.String("no", fmt.Sprintf("#%d", i))), writer); err != nil {
					cancel()
					slog.Error("error on node", slog.String("error", err.Error()))

					mtx.Lock()
					if globalErr == nil {
						globalErr = err
					}
					mtx.Unlock()
				}
				wg.Done()
			}()
		}

		wg.Wait()

		return globalErr
	},
}

func run(ctx context.Context, logger *slog.Logger, writer *datastore.Writer) error {
	switch nodeConfig.story {
	case "circle":
		return circle.RunNode(ctx, logger, nodeConfig.seedURL, writer)

	case "sphere":
		return sphere.RunNode(ctx, logger, nodeConfig.seedURL, writer)

	default:
		return errors.New("unexpected story name")
	}
}

func init() {
	flags := nodeCmd.PersistentFlags()

	flags.StringVarP(&nodeConfig.seedURL, "seed-url", "u", valueFromEnvString("SEED_URL", "https://localhost:8443"), "URL of the seed.")
	flags.StringVarP(&nodeConfig.story, "story", "s", valueFromEnvString("STORY", "sphere"), "situation of the simulation.")
	flags.StringVarP(&nodeConfig.writer, "writer", "w", valueFromEnvString("WRITER", "mongodb"), "writer type.")
	flags.UintVarP(&nodeConfig.concurrency, "concurrency", "c", valueFromEnvUint("CONCURRENCY", 1), "number of concurrent requests.")
	bindMongodbConfig(flags, &nodeConfig.mongodbConfig, true)

	rootCmd.AddCommand(nodeCmd)
}
