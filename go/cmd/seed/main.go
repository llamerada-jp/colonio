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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/llamerada-jp/colonio/go/seed"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var configFile string

type config struct {
	seed.Config

	DocumentRoot string            `json:"documentRoot"`
	Headers      map[string]string `json:"headers"`
	Overrides    map[string]string `json:"overrides"`
	Port         uint16            `json:"port"`
	Path         string            `json:"path"`
}

var cmd = &cobra.Command{
	Use: "seed-test",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		flag.CommandLine.Parse([]string{})
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		mux := http.NewServeMux()
		baseDir := filepath.Dir(configFile)

		// read config file
		configData, err := ioutil.ReadFile(configFile)
		if err != nil {
			return err
		}
		config := &config{}
		if err := json.Unmarshal(configData, config); err != nil {
			return err
		}

		// Set HTTP headers to enable SharedArrayBuffer for WebAssembly Threads
		headers := config.Headers
		if headers == nil {
			headers = make(map[string]string)
		}
		headers["Cross-Origin-Opener-Policy"] = "same-origin"
		headers["Cross-Origin-Embedder-Policy"] = "require-corp"

		// Publish static documents
		mux.Handle("/", &headerWrapper{
			handler: http.FileServer(http.Dir(filepath.Join(baseDir, config.DocumentRoot))),
			headers: headers,
		})
		for pattern, path := range config.Overrides {
			mux.Handle(pattern,
				&headerWrapper{
					handler: http.FileServer(http.Dir(filepath.Join(baseDir, path))),
					headers: headers,
				})
		}

		// Publish colonio-seed
		seed, err := seed.NewSeed(&config.Config)
		if err != nil {
			return err
		}
		mux.Handle(config.Path, &headerWrapper{
			handler: seed,
			headers: headers,
		})
		if err := seed.Start(); err != nil {
			return err
		}

		// Start HTTP service.
		return http.ListenAndServe(fmt.Sprintf(":%d", config.Port), mux)
	},
}

type headerWrapper struct {
	handler http.Handler
	headers map[string]string
}

func (h *headerWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	headerWriter := w.Header()
	for k, v := range h.headers {
		headerWriter.Add(k, v)
	}
	h.handler.ServeHTTP(w, r)
}

func init() {
	flags := cmd.PersistentFlags()

	flags.StringVarP(&configFile, "config", "c", "seed.json", "path to configuration file")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
}

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
