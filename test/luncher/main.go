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
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/llamerada-jp/colonio/seed"
	"github.com/llamerada-jp/colonio/seed/util"
	"github.com/spf13/cobra"
)

var configFile string
var url string
var keyword_success string
var keyword_failure string

var cmd = &cobra.Command{
	Use: "test_luncher",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := slog.Default()
		cmd.SilenceUsage = true

		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("ignore-certificate-errors", "1"),
		)
		allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
		defer cancel()
		ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
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

		// set handlers for test
		setSeedTransportHandlers(service.RootMux)

		// start service and seed routine
		go func() {
			seed.Run(ctx)
		}()
		go func() {
			if err := service.Run(); err != nil {
				slog.Error("failed to run service", slog.String("error", err.Error()))
				os.Exit(1)
			}
		}()

		// wait for keyword to stop
		finCh := make(chan bool, 1)
		chromedp.ListenTarget(ctx, func(ev interface{}) {
			switch ev := ev.(type) {
			case *runtime.EventConsoleAPICalled:
				var line string

				for _, arg := range ev.Args {
					line += " "
					switch arg.Type {
					case runtime.TypeString:
						var s string
						if err := json.Unmarshal(arg.Value, &s); err != nil {
							slog.Error("failed to decode", slog.String("value", string(arg.Value)))
						}
						line += s

					case runtime.TypeNumber:
						var n float64
						if err := json.Unmarshal(arg.Value, &n); err != nil {
							slog.Error("failed to decode", slog.String("value", string(arg.Value)))
						}
						if n == float64(int64(n)) {
							line += fmt.Sprintf("%d", int64(n))
						} else {
							line += fmt.Sprintf("%g", n)
						}

					case runtime.TypeBoolean:
						var b bool
						if err := json.Unmarshal(arg.Value, &b); err != nil {
							slog.Error("failed to decode", slog.String("value", string(arg.Value)))
						}
						line += fmt.Sprintf("%t", b)

					case runtime.TypeUndefined:
						line += "undefined"

					case runtime.TypeObject:
						line += "object"

					case runtime.TypeFunction:
						line += "function"

					default:
						line += "unknown"
					}
				}

				fmt.Println("(browser)" + line)
				switch strings.TrimSpace(line) {
				case keyword_success:
					finCh <- true

				case keyword_failure:
					finCh <- false
				}

			case *runtime.EventExceptionThrown:
				s := ev.ExceptionDetails.Error()
				fmt.Printf("* %s\n", s)
			}
		})

		// run test in headless browser
		if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
			return err
		}

		// get result of the test
		result := <-finCh
		if !result {
			return fmt.Errorf("test failed")
		}

		return nil
	},
}

func init() {
	flags := cmd.PersistentFlags()
	flags.StringVarP(&configFile, "config", "c", "seed.json", "path to configuration file")
	flags.StringVarP(&url, "url", "u", "https://localhost:8080/index.html", "URL to access to start the test")
	flags.StringVarP(&keyword_success, "success", "s", "SUCCESS", "keyword will be output when the test succeeds")
	flags.StringVarP(&keyword_failure, "failure", "f", "FAIL", "keyword will be output when the test fails")
}

func main() {
	if err := cmd.Execute(); err != nil {
		slog.Error(("error on test_luncher"), slog.String("err", err.Error()))
		os.Exit(1)
	}
}
