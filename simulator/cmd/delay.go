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
	"path/filepath"

	"github.com/llamerada-jp/colonio/simulator/delay"
	"github.com/spf13/cobra"
)

var delayConfig = struct {
	delayMaterial string
}{}

var delayCmd = &cobra.Command{
	Use: "delay",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(delayConfig.delayMaterial) == 0 {
			return errors.New("delay material path should be set")
		}

		cmd.SilenceUsage = true

		return delay.ApplyDelay(
			filepath.Join(delayConfig.delayMaterial, "servers.csv"),
			filepath.Join(delayConfig.delayMaterial, "pings.csv"),
		)
	},
}

func init() {
	flags := delayCmd.Flags()
	flags.StringVarP(&delayConfig.delayMaterial, "delay", "d", "", "delay material path")

	rootCmd.AddCommand(delayCmd)
}
