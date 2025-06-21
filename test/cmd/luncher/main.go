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
	"path/filepath"
	"strings"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/llamerada-jp/colonio/seed"
	"github.com/llamerada-jp/colonio/test/util/server"
	"github.com/spf13/cobra"
)

const (
	KEYWORD_SUCCESS = "SUCCESS"
	KEYWORD_FAILURE = "FAIL"
	SERVER_PORT     = 8080
)

var (
	documentPath string
	jsModulePath string
)

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

		// create seed & server
		seed := seed.NewSeed(seed.WithLogger(logger))

		if len(documentPath) == 0 {
			return fmt.Errorf("document root path is required")
		}
		absDocumentPath, err := filepath.Abs(documentPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for document path: %w", err)
		}

		if len(jsModulePath) == 0 {
			return fmt.Errorf("output path is required")
		}
		absJsModulePath, err := filepath.Abs(jsModulePath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for output path: %w", err)
		}

		helper := server.NewHelper(seed,
			server.WithLogger(logger),
			server.WithPort(SERVER_PORT),
			// document root
			server.WithDocument(absDocumentPath),
			// document override
			server.WithDocumentOverride("/colonio.js", absJsModulePath),
			// handles for test
			server.WithHttpHandlerFunc(NormalPath, normalResponder),
			server.WithHttpHandlerFunc(ErrorPath, errorResponder),
		)

		// start service and seed routine
		helper.Start(ctx)
		defer helper.Stop()

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
				case KEYWORD_SUCCESS:
					finCh <- true

				case KEYWORD_FAILURE:
					finCh <- false
				}

			case *runtime.EventExceptionThrown:
				s := ev.ExceptionDetails.Error()
				fmt.Printf("* %s\n", s)
			}
		})

		// run test in headless browser
		if err := chromedp.Run(ctx, chromedp.Navigate(fmt.Sprintf("https://localhost:%d/index.html", SERVER_PORT))); err != nil {
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
	flags := cmd.Flags()
	flags.StringVarP(&documentPath, "document-root", "d", "", "document root path")
	flags.StringVarP(&jsModulePath, "js-module", "j", "", "js-module path")
}

func main() {
	if err := cmd.Execute(); err != nil {
		slog.Error(("error on test_luncher"), slog.String("err", err.Error()))
		os.Exit(1)
	}
}
