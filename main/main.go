/*
 * Copyright 2019 Yuji Ito <llamerada.jp@gmail.com>
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
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/colonio/colonio-seed/seed"
)

type SeedConfig struct {
	seed.Config
	Host string `json:"host"`
	Port int    `json:"port"`
	Path string `json:"path"`
}

type Seed struct {
	seed   *seed.Seed
	group  *seed.Group
	config *SeedConfig
}

func newSeed(config *SeedConfig) *Seed {
	s := &Seed{}
	s.seed = seed.NewSeed(s)
	s.config = config
	return s
}

func (s *Seed) start() {
	err := s.seed.Start(s.config.Host, s.config.Port)
	if err != nil {
		log.Fatal(err)
	}
}

func (s *Seed) Bind(uri string) (*seed.Group, error) {
	if uri == s.config.Path {
		if group, ok := s.seed.GetGroup("default"); ok {
			// return a group instance if the group is exist.
			return group, nil

		} else {
			s.group = s.seed.CreateGroup("default", s.config.CastConfig())
			return s.group, nil
		}

	} else {
		return nil, errors.New("invalid uri " + uri)
	}
}

func main() {
	// Parse parameter
	configFName := flag.String("config", "", "configure file")
	flag.Parse()

	// Read configure file
	var config *SeedConfig
	if *configFName == "" {
		log.Fatal("You must set the configure file name.")
	}
	if _, err := os.Stat(*configFName); err != nil {
		log.Fatal(err)

	} else {
		f, err := ioutil.ReadFile(*configFName)
		if err != nil {
			log.Fatal(err)
		}
		config = &SeedConfig{}
		if err := json.Unmarshal(f, config); err != nil {
			log.Fatal(err)
		}
	}

	s := newSeed(config)
	s.start()
}
