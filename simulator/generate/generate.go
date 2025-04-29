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
package generate

import (
	_ "embed"
	"fmt"
	"html/template"
	"os"

	"sigs.k8s.io/yaml"
)

//go:embed template/pod.yaml
var podTemplateStr string

type podValue struct {
	Name       string  `json:"name"`
	Population uint    `json:"population"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Radius     float64 `json:"radius"`
}

func GeneratePods(valuesFile string, areas int, rate uint) error {
	podTemplate := template.New("node")
	_, err := podTemplate.Parse(podTemplateStr)
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}

	valuesRaw, err := os.ReadFile(valuesFile)
	if err != nil {
		return fmt.Errorf("failed to read values file: %v", err)
	}

	var values []podValue
	err = yaml.Unmarshal(valuesRaw, &values)
	if err != nil {
		return fmt.Errorf("failed to unmarshal values file: %v", err)
	}
	if len(values) == 0 {
		return fmt.Errorf("values file is empty")
	}

	for i, v := range values {
		if i >= areas {
			break
		}

		valueMap := map[string]string{
			"name":        v.Name,
			"concurrency": fmt.Sprintf("%d", v.Population/rate),
			"latitude":    fmt.Sprintf("%f", v.Latitude),
			"longitude":   fmt.Sprintf("%f", v.Longitude),
			"radius":      fmt.Sprintf("%f", v.Radius*10),
		}
		if err := podTemplate.Execute(os.Stdout, valueMap); err != nil {
			return fmt.Errorf("failed to execute template: %v", err)
		}
		fmt.Printf("\n---\n")
	}

	return nil
}
