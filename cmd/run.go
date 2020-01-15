/*
 * Copyright 2019-2020 Yuji Ito <llamerada.jp@gmail.com>
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

package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"log/syslog"
	"os"

	"github.com/colonio/colonio-seed/pkg/seed"
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

var logger seed.Logger

func newSeed(config *SeedConfig) *Seed {
	s := &Seed{}

	s.seed = seed.NewSeed(s)
	s.config = config
	return s
}

func (s *Seed) start() {
	err := s.seed.Start(s.config.Host, s.config.Port)
	if err != nil {
		logger.E.Fatal("Fatal on Start", err)
	}
}

func (s *Seed) Bind(uri string) (*seed.Group, error) {
	if uri == s.config.Path {
		if group, ok := s.seed.GetGroup("default"); ok {
			// return a group instance if the group is exist.
			return group, nil

		} else {
			var err error
			s.group, err = s.seed.CreateGroup("default", s.config.CastConfig())
			if err != nil {
				logger.E.Fatal("Fatal on CreateGroup", err)
			}
			return s.group, nil
		}

	} else {
		return nil, errors.New("invalid uri " + uri)
	}
}

func initLogger(isSyslog bool, isVerbose bool) {
	var e io.Writer
	var w io.Writer
	var i io.Writer
	var d io.Writer
	if isSyslog {
		var err error
		e, err = syslog.New(syslog.LOG_ERR, "colonio")
		if err != nil {
			log.Fatalln(err)
		}

		w, err = syslog.New(syslog.LOG_WARNING, "colonio")
		if err != nil {
			log.Fatalln(err)
		}

		i, err = syslog.New(syslog.LOG_INFO, "colonio")
		if err != nil {
			log.Fatalln(err)
		}

		if isVerbose {
			d, err = syslog.New(syslog.LOG_DEBUG, "colonio")
			if err != nil {
				log.Fatalln(err)
			}

		} else {
			d = ioutil.Discard
		}
	} else {
		e = os.Stderr
		w = os.Stderr
		i = os.Stderr
		if isVerbose {
			d = os.Stderr
		} else {
			d = ioutil.Discard
		}
	}

	logger.E = log.New(e, "[ERROR]", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
	logger.W = log.New(w, "[WARN]", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
	logger.I = log.New(i, "[INFO]", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
	logger.D = log.New(d, "[DEBUG]", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)

	seed.SetLogger(&logger)
}

func Execute() {
	// Parse parameter
	configFName := flag.String("config", "", "Specify the configuration file.")
	isSyslog := flag.Bool("syslog", false, "Output log using syslog.")
	isVerbose := flag.Bool("verbose", false, "Output verbose logs.")
	flag.Parse()

	initLogger(*isSyslog, *isVerbose)

	// Read configure file
	var config *SeedConfig
	if *configFName == "" {
		logger.E.Fatal("You must set the configure file name.")
	}
	if _, err := os.Stat(*configFName); err != nil {
		logger.E.Fatal(err)

	} else {
		f, err := ioutil.ReadFile(*configFName)
		if err != nil {
			logger.E.Fatal(err)
		}
		config = &SeedConfig{}
		if err := json.Unmarshal(f, config); err != nil {
			logger.E.Fatal(err)
		}
	}

	s := newSeed(config)
	s.start()
}
