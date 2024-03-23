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
package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/llamerada-jp/colonio/config"
)

type ServiceConfig struct {
	baseDir string

	// Configs for HTTP server
	Headers  map[string]string `json:"headers"`
	Port     uint16            `json:"port"`
	CertFile string            `json:"certFile"`
	KeyFile  string            `json:"keyFile"`

	// Configs for file server
	DocumentRoot *string           `json:"documentRoot"`
	DocOverrides map[string]string `json:"docOverrides"`

	// Seed config
	Cluster  *config.Cluster `json:"cluster"`
	SeedPath string          `json:"seedPath"`
}

func ReadConfig(path string) (*ServiceConfig, error) {
	config := &ServiceConfig{}
	// read config file
	configData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	if err := json.Unmarshal(configData, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	config.baseDir = filepath.Dir(path)

	return config, nil
}

func (c *ServiceConfig) ToAbsPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(c.baseDir, path)
}

func (c *ServiceConfig) validate() error {
	if err := c.Cluster.Validate(); err != nil {
		return err
	}

	if len(c.CertFile) == 0 {
		return errors.New("config value of `certFile` required")
	}

	if len(c.KeyFile) == 0 {
		return errors.New("config value of `keyFile` required")
	}

	return nil
}
