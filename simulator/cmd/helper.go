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
	"os"
	"strconv"

	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/spf13/pflag"
)

const (
	kEnvVerPrefix = "COLONIO_SIMULATOR_"
)

func valueFromEnvString(key, defaultValue string) string {
	if v, ok := os.LookupEnv(kEnvVerPrefix + key); ok {
		return v
	}
	return defaultValue
}

func valueFromEnvUint(key string, defaultValue uint) uint {
	if str, ok := os.LookupEnv(kEnvVerPrefix + key); ok {
		if v, err := strconv.Atoi(str); err == nil {
			return uint(v)
		}
	}
	return defaultValue
}

func valueFromEnvFloat64(key string, defaultValue float64) float64 {
	if str, ok := os.LookupEnv(kEnvVerPrefix + key); ok {
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			return v
		}
	}
	return defaultValue
}

func bindMongodbConfig(flags *pflag.FlagSet, config *datastore.MongodbConfig, withDefaultURI bool) {
	defaultURI := "mongodb://localhost:27017"
	if !withDefaultURI {
		defaultURI = ""
	}

	flags.StringVar(&config.URI, "mongodb-uri", valueFromEnvString("MONGODB_URI", defaultURI), "URI of the mongodb to export logs.")
	flags.StringVar(&config.User, "mongodb-user", valueFromEnvString("MONGODB_USER", "simulator"), "user of the mongodb.")
	flags.StringVar(&config.Password, "mongodb-password", valueFromEnvString("MONGODB_PASSWORD", "simulator"), "password of the mongodb.")
	flags.StringVar(&config.Database, "mongodb-database", valueFromEnvString("MONGODB_DATABASE", "simulator"), "database name of the mongodb.")
	flags.StringVar(&config.Collection, "mongodb-collection", valueFromEnvString("MONGODB_COLLECTION", "logs"), "collection name of the mongodb.")
}
