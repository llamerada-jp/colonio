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
package circle

import (
	"context"
	"log/slog"

	"github.com/llamerada-jp/colonio/simulator/base"
	"github.com/llamerada-jp/colonio/simulator/datastore"
	"github.com/llamerada-jp/colonio/simulator/utils"
)

func RunNode(ctx context.Context, logger *slog.Logger, seedURL string, writer *datastore.Writer, region *utils.Region) error {
	record := &base.Record{}
	node, err := base.NewNode(logger, seedURL, writer, nil, record, region)
	if err != nil {
		return err
	}

	return node.Start(ctx)
}
