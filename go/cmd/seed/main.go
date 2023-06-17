package main

/*
 * Copyright 2017 Yuji Ito <llamerada.jp@gmail.com>
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

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/llamerada-jp/colonio/go/service"
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
		cmd.SilenceUsage = true

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// read config file
		configData, err := os.ReadFile(configFile)
		if err != nil {
			return err
		}
		config := &service.Config{}
		if err := json.Unmarshal(configData, config); err != nil {
			return err
		}

		// start service of the seed
		baseDir := filepath.Dir(configFile)
		return service.Run(ctx, baseDir, config, nil)
	},
}

func init() {
	flags := cmd.PersistentFlags()

	flags.StringVarP(&configFile, "config", "c", "seed.json", "path to configuration file")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
}

func main() {
	if err := cmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
