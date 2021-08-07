/*
 * Copyright 2019-2021 Yuji Ito <llamerada.jp@gmail.com>
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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/golang/glog"
	"github.com/llamerada-jp/colonio-seed/seed"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type seedConfig struct {
	seed.Config
	Host string `json:"host"`
	Port int    `json:"port"`
	Path string `json:"path"`
}

var configFile string

var rootCmd = &cobra.Command{
	Use:   "seed",
	Short: "colonio-seed is seed/server program for Colonio node library.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		flag.CommandLine.Parse([]string{})
	},
	Run: func(cmd *cobra.Command, args []string) {
		glog.Info("Start colonio-seed.")
		// Read configure file
		f, err := ioutil.ReadFile(configFile)
		if err != nil {
			glog.Fatal(err)
		}

		config := &seedConfig{}
		if err := json.Unmarshal(f, config); err != nil {
			glog.Fatal(err)
		}
		glog.Infof("Run using configfile %s", configFile)

		seed, err := seed.NewSeed(&config.Config)
		if err != nil {
			glog.Fatal(err)
		}
		if err := seed.Start(); err != nil {
			glog.Fatal(err)
		}

		http.Handle(config.Path, seed)
		if err := http.ListenAndServe(fmt.Sprintf("%s:%d", config.Host, config.Port), nil); err != nil {
			glog.Fatal(err)
		}
	},
}

func init() {
	flags := rootCmd.PersistentFlags()

	flags.StringVarP(&configFile, "config", "f", "seed.json", "specify the configuration file")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
