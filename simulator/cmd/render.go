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
	"context"
	"errors"
	"fmt"

	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/llamerada-jp/colonio/simulator/sphere"
	"github.com/spf13/cobra"
)

type renderConfig struct {
	inputFileName string
	outputPath    string
	story         string
	mongodbConfig datastore.MongodbConfig
}

func (r *renderConfig) check() error {
	if r.inputFileName == "" && r.mongodbConfig.URI == "" {
		return errors.New("input file name or mongodb URI should be set")
	}

	if r.inputFileName != "" && r.mongodbConfig.URI != "" {
		return errors.New("input file name and mongodb URI should not be set at the same time")
	}

	return nil
}

type renderCmdImpl struct {
	config *renderConfig
}

func (r *renderCmdImpl) run() error {
	ctx := context.Background()
	rawReader, err := r.makeReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to create reader: %w", err)
	}

	reader := datastore.NewReader(rawReader)

	return r.forkStory(ctx, reader)
}

func (r *renderCmdImpl) makeReader(ctx context.Context) (datastore.RawReader, error) {
	if r.config.inputFileName != "" {
		return datastore.NewFileReader(r.config.inputFileName)
	}

	return datastore.NewMongodbReader(ctx, &r.config.mongodbConfig)
}

func (r *renderCmdImpl) forkStory(ctx context.Context, reader *datastore.Reader) error {
	switch r.config.story {
	case "sphere":
		return sphere.RunRender(ctx, reader)

	default:
		return errors.New("unexpected story name")
	}
}

var render *renderCmdImpl

var renderCmd = &cobra.Command{
	Use: "render",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := render.config.check(); err != nil {
			return err
		}
		cmd.SilenceUsage = true

		return render.run()
	},
}

func init() {
	render = &renderCmdImpl{
		config: &renderConfig{},
	}

	flags := renderCmd.Flags()

	flags.StringVarP(&render.config.story, "story", "s", valueFromEnvString("STORY", "sphere"), "situation of the simulation.")
	flags.StringVar(&render.config.inputFileName, "input-file", valueFromEnvString("INPUT_FILE", ""), "input file name.")
	flags.StringVar(&render.config.outputPath, "output-path", valueFromEnvString("OUTPUT_PATH", "."), "output path.")
	bindMongodbConfig(flags, &render.config.mongodbConfig, false)

	rootCmd.AddCommand(renderCmd)
}