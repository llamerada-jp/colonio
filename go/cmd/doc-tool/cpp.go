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
	"fmt"

	doctool "github.com/llamerada-jp/colonio/go/doc-tool"
	"github.com/spf13/cobra"
)

var src string
var dst string

var cppCmd = &cobra.Command{
	Use: "cpp",
	RunE: func(cmd *cobra.Command, args []string) error {
		if src == "" || dst == "" {
			return fmt.Errorf("should specify file")
		}

		return doctool.Process(src, dst)
	},
}

func init() {
	flagSet := cppCmd.PersistentFlags()
	flagSet.StringVar(&src, "src", "", "source filename")
	flagSet.StringVar(&dst, "dst", "", "destination filename")

	rootCmd.AddCommand(cppCmd)
}
