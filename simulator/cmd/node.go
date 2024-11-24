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
	"sync"

	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/llamerada-jp/colonio/simulator/sphere"
	"github.com/spf13/cobra"
)

var nodeConfig = struct {
	seedURL       string
	simType       string
	concurrency   uint
	mongodbConfig datastore.MongodbConfig
}{}

var nodeCmd = &cobra.Command{
	Use: "node",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(nodeConfig.seedURL) == 0 {
			return errors.New("seed URL should be set")
		}

		cmd.SilenceUsage = true

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		writer, err := datastore.NewMongodb(ctx, &nodeConfig.mongodbConfig)
		if err != nil {
			return fmt.Errorf("failed to create datastore: %w", err)
		}

		wg := &sync.WaitGroup{}
		wg.Add(int(nodeConfig.concurrency))

		mtx := &sync.Mutex{}
		var globalErr error

		for i := uint(0); i < nodeConfig.concurrency; i++ {
			go func() {
				if err := run(ctx, writer); err != nil {
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

func run(ctx context.Context, writer *datastore.Mongodb) error {
	switch nodeConfig.simType {
	case "sphere":
		return sphere.RunNode(ctx, nodeConfig.seedURL, writer)
	default:
		return errors.New("unknown simulation type")
	}
}

func init() {
	flags := nodeCmd.PersistentFlags()

	flags.StringVarP(&nodeConfig.seedURL, "seed-url", "u", valueFromEnvString("SEED_URL", "https://localhost:8080/seed"), "URL of the seed.")
	flags.StringVarP(&nodeConfig.simType, "story", "s", valueFromEnvString("STORY", "sphere"), "situation of the simulation.")
	flags.UintVarP(&nodeConfig.concurrency, "concurrency", "c", valueFromEnvUint("CONCURRENCY", 1), "number of concurrent requests.")

	flags.StringVar(&nodeConfig.mongodbConfig.URI, "mongodb-uri", valueFromEnvString("MONGODB_URI", "mongodb://localhost:27017"), "URI of the mongodb to export logs.")
	flags.StringVar(&nodeConfig.mongodbConfig.User, "mongodb-user", valueFromEnvString("MONGODB_USER", "simulator"), "user of the mongodb.")
	flags.StringVar(&nodeConfig.mongodbConfig.Password, "mongodb-password", valueFromEnvString("MONGODB_PASSWORD", "simulator"), "password of the mongodb.")
	flags.StringVar(&nodeConfig.mongodbConfig.Database, "mongodb-database", valueFromEnvString("MONGODB_DATABASE", "simulator"), "database name of the mongodb.")
	flags.StringVar(&nodeConfig.mongodbConfig.Collection, "mongodb-collection", valueFromEnvString("MONGODB_COLLECTION", "logs"), "collection name of the mongodb.")

	rootCmd.AddCommand(nodeCmd)
}
