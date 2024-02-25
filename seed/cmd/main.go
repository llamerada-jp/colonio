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
	"flag"
	"log/slog"
	"os"

	"github.com/llamerada-jp/colonio/seed"
	"github.com/llamerada-jp/colonio/seed/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var configFile string

var cmd = &cobra.Command{
	Use: "seed",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		flag.CommandLine.Parse([]string{})
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := slog.Default()
		cmd.SilenceUsage = true

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		config, err := util.ReadConfig(configFile)
		if err != nil {
			return err
		}

		// create server
		service := util.NewService(config, logger)

		// create seed
		seed, seedHandler := seed.NewSeed(config.Cluster, logger, nil)
		service.SetHandler(seedHandler)

		// run seed & server
		go func() {
			seed.Run(ctx)
		}()

		return service.Run()
	},
}

func init() {
	flags := cmd.PersistentFlags()

	flags.StringVarP(&configFile, "config", "c", "seed.json", "path to configuration file")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
}

func main() {
	if err := cmd.Execute(); err != nil {
		slog.Error("error on seed", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
