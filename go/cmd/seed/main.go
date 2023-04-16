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
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"

	"github.com/llamerada-jp/colonio/go/seed"
	"github.com/quic-go/quic-go/http3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var configFile string

type config struct {
	seed.Config

	DocumentRoot *string           `json:"documentRoot,omitempty"`
	Headers      map[string]string `json:"headers"`
	Overrides    map[string]string `json:"overrides"`
	Port         uint16            `json:"port"`
	Path         string            `json:"path"`
	CertFile     string            `json:"certFile"`
	KeyFile      string            `json:"keyFile"`
	UseTCP       bool              `json:"useTcp"`
}

var cmd = &cobra.Command{
	Use: "seed",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		flag.CommandLine.Parse([]string{})
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		mux := http.NewServeMux()
		baseDir := filepath.Dir(configFile)

		// read config file
		configData, err := os.ReadFile(configFile)
		if err != nil {
			return err
		}
		config := &config{}
		if err := json.Unmarshal(configData, config); err != nil {
			return err
		}

		if err := validateConf(config); err != nil {
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
		if config.DocumentRoot != nil {
			log.Printf("publish DocumentRoot: %s", *config.DocumentRoot)
			mux.Handle("/", http.FileServer(http.Dir(calcPath(baseDir, *config.DocumentRoot))))
			for pattern, path := range config.Overrides {
				mux.Handle(pattern, http.FileServer(http.Dir(calcPath(baseDir, path))))
			}
		}

		// Publish colonio-seed
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		seed, err := seed.NewSeedHandler(ctx, &config.Config, nil)
		if err != nil {
			return err
		}
		mux.Handle(config.Path+"/", http.StripPrefix(config.Path, seed))

		// Start HTTP/3 service.
		if config.UseTCP {
			log.Println("start seed with tcp mode")
			return http3.ListenAndServe(
				fmt.Sprintf(":%d", config.Port),
				calcPath(baseDir, config.CertFile),
				calcPath(baseDir, config.KeyFile),
				&handlerWrapper{
					handler: mux,
					headers: headers,
				})
		}

		log.Println("start seed")
		server := http3.Server{
			Handler: mux,
			Addr:    fmt.Sprintf(":%d", config.Port),
		}
		return server.ListenAndServeTLS(
			calcPath(baseDir, config.CertFile),
			calcPath(baseDir, config.KeyFile))
	},
}

func validateConf(c *config) error {
	if len(c.CertFile) == 0 {
		return errors.New("config value of `certFile` required")
	}

	if len(c.KeyFile) == 0 {
		return errors.New("config value of `keyFile` required")
	}

	return nil
}

func calcPath(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// warp http response writer to get http status code
type logWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (lw *logWrapper) WriteHeader(statusCode int) {
	lw.statusCode = statusCode
	lw.ResponseWriter.WriteHeader(statusCode)
}

// warp http handler to add some headers and to output access logs
type handlerWrapper struct {
	handler http.Handler
	headers map[string]string
}

func (h *handlerWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := rand.Int63()
	log.Printf("(%016x) proto:%s, remote addr:%s, method:%s uri:%s size:%d\n", id, r.Proto, r.RemoteAddr, r.Method, r.RequestURI, r.ContentLength)

	writerWithLog := &logWrapper{
		ResponseWriter: w,
	}
	defer func() {
		log.Printf("(%016x) status code:%d", id, writerWithLog.statusCode)
	}()

	headerWriter := w.Header()
	for k, v := range h.headers {
		headerWriter.Add(k, v)
	}

	h.handler.ServeHTTP(writerWithLog, r)
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
