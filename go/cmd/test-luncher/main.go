package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/llamerada-jp/colonio/go/service"
	"github.com/spf13/cobra"
)

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

var configFile string
var url string
var keyword_success string
var keyword_failure string

var cmd = &cobra.Command{
	Use: "test_luncher",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("ignore-certificate-errors", "1"),
		)
		allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
		defer cancel()
		ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
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
		go func() {
			baseDir := filepath.Dir(configFile)
			if err := service.Run(ctx, baseDir, config, nil); err != nil {
				log.Fatal(err)
			}
		}()

		// wait for keyword to stop
		finCh := make(chan bool, 1)
		chromedp.ListenTarget(ctx, func(ev interface{}) {
			switch ev := ev.(type) {
			case *runtime.EventConsoleAPICalled:
				for _, arg := range ev.Args {
					if arg.Type != runtime.TypeString {
						continue
					}
					var line string
					if err := json.Unmarshal(arg.Value, &line); err != nil {
						log.Println("failed to decode: ", line)
					}
					log.Println("(browser)", line)
					switch line {
					case keyword_success:
						finCh <- true

					case keyword_failure:
						finCh <- false
					}
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
		log.Println(err)
		os.Exit(1)
	}
}
