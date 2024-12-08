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
	"time"

	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/spf13/cobra"
)

var exportConfig = struct {
	outputFileName string
	mongodbConfig  datastore.MongodbConfig
}{}

var exportCmd = &cobra.Command{
	Use: "export",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		reader, err := datastore.NewMongodb(ctx, &exportConfig.mongodbConfig)
		if err != nil {
			return fmt.Errorf("failed to create Mongodb datastore: %w", err)
		}

		writer, err := datastore.NewFileWriter(exportConfig.outputFileName)
		if err != nil {
			return fmt.Errorf("failed to create FileWriter: %w", err)
		}
		defer writer.Close()

		err = reader.ReadAll(func(timestamp *time.Time, nodeID string, record []byte) error {
			writer.Push(timestamp, nodeID, record)
			return nil
		})

		return err
	},
}

func init() {
	flags := exportCmd.Flags()

	flags.StringVar(&exportConfig.outputFileName, "output", valueFromEnvString("OUTPUT", ""), "output file name.")
	flags.StringVar(&exportConfig.mongodbConfig.URI, "mongodb-uri", valueFromEnvString("MONGODB_URI", "mongodb://localhost:27017"), "URI of the mongodb to export logs.")
	flags.StringVar(&exportConfig.mongodbConfig.User, "mongodb-user", valueFromEnvString("MONGODB_USER", "simulator"), "user of the mongodb.")
	flags.StringVar(&exportConfig.mongodbConfig.Password, "mongodb-password", valueFromEnvString("MONGODB_PASSWORD", "simulator"), "password of the mongodb.")
	flags.StringVar(&exportConfig.mongodbConfig.Database, "mongodb-database", valueFromEnvString("MONGODB_DATABASE", "simulator"), "database name of the mongodb.")
	flags.StringVar(&exportConfig.mongodbConfig.Collection, "mongodb-collection", valueFromEnvString("MONGODB_COLLECTION", "logs"), "collection name of the mongodb.")

	rootCmd.AddCommand(exportCmd)
}
