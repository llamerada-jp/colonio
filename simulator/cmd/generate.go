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
	"os"
	"text/template"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

var generateConfig = struct {
	template        string
	values          string
	areas           uint
	concurrencyRate uint
}{}

type valueEntry struct {
	Name       string  `json:"name"`
	Population uint    `json:"population"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Radius     float64 `json:"radius"`
}

var generateCmd = &cobra.Command{
	Use: "generate",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(generateConfig.template) == 0 {
			return errors.New("template file should be set")
		}

		if len(generateConfig.values) == 0 {
			return errors.New("values file should be set")
		}

		cmd.SilenceUsage = true

		tempRaw, err := os.ReadFile(generateConfig.template)
		if err != nil {
			return fmt.Errorf("failed to read template file: %v", err)
		}

		temp := template.New("node")
		_, err = temp.Parse(string(tempRaw))
		if err != nil {
			return fmt.Errorf("failed to parse template: %v", err)
		}

		valuesRaw, err := os.ReadFile(generateConfig.values)
		if err != nil {
			return fmt.Errorf("failed to read values file: %v", err)
		}

		var values []valueEntry
		err = yaml.Unmarshal(valuesRaw, &values)
		if err != nil {
			return fmt.Errorf("failed to unmarshal values file: %v", err)
		}
		if len(values) == 0 {
			return fmt.Errorf("values file is empty")
		}

		for i, v := range values {
			if i >= int(generateConfig.areas) {
				break
			}

			valueMap := map[string]string{
				"name":        v.Name,
				"concurrency": fmt.Sprintf("%d", v.Population/generateConfig.concurrencyRate),
				"latitude":    fmt.Sprintf("%f", v.Latitude),
				"longitude":   fmt.Sprintf("%f", v.Longitude),
				"radius":      fmt.Sprintf("%f", v.Radius*10),
			}
			if err := temp.Execute(os.Stdout, valueMap); err != nil {
				return fmt.Errorf("failed to execute template: %v", err)
			}
			if i < len(values)-1 {
				fmt.Println("---")
			}
		}

		return nil
	},
}

func init() {
	flags := generateCmd.Flags()

	flags.StringVarP(&generateConfig.template, "template", "t", "", "template file")
	flags.StringVarP(&generateConfig.values, "values", "v", "", "values file")
	flags.UintVarP(&generateConfig.concurrencyRate, "concurrency-rate", "r", 4000000, "concurrency = population / concurrency-rate")
	flags.UintVarP(&generateConfig.areas, "areas", "a", 32, "number of areas")

	rootCmd.AddCommand(generateCmd)
}
