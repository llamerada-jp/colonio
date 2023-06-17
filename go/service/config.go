/*
 * Copyright 2019-2021 Yuji Ito <llamerada.jp@gmail.com>
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
package service

import (
	"errors"

	"github.com/llamerada-jp/colonio/go/seed"
)

type Config struct {
	seed.Config

	DocumentRoot *string           `json:"documentRoot"`
	Headers      map[string]string `json:"headers"`
	Overrides    map[string]string `json:"overrides"`
	Port         uint16            `json:"port"`
	Path         string            `json:"path"`
	CertFile     string            `json:"certFile"`
	KeyFile      string            `json:"keyFile"`
	UseTCP       bool              `json:"useTcp"`
}

func (c *Config) validate() error {
	if len(c.CertFile) == 0 {
		return errors.New("config value of `certFile` required")
	}

	if len(c.KeyFile) == 0 {
		return errors.New("config value of `keyFile` required")
	}

	return nil
}
