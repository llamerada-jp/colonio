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
	"errors"
	"fmt"

	"github.com/llamerada-jp/colonio/simulator/generate"
	"github.com/spf13/cobra"
)

var generateConfig = struct {
	values          string
	areas           uint
	concurrencyRate uint
}{}

var generateCmd = &cobra.Command{
	Use: "generate",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(generateConfig.values) == 0 {
			return errors.New("values file should be set")
		}

		cmd.SilenceUsage = true

		err := generate.GeneratePods(generateConfig.values,
			int(generateConfig.areas), generateConfig.concurrencyRate)
		if err != nil {
			return fmt.Errorf("failed to generate pods manifest: %v", err)
		}

		return nil
	},
}

func init() {
	flags := generateCmd.Flags()

	flags.StringVarP(&generateConfig.values, "values", "v", "", "values file")
	flags.UintVarP(&generateConfig.concurrencyRate, "concurrency-rate", "r", 4000000, "concurrency = population / concurrency-rate")
	flags.UintVarP(&generateConfig.areas, "areas", "a", 32, "number of areas")

	rootCmd.AddCommand(generateCmd)
}
