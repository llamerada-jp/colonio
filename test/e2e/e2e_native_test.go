//go:build !js

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

package e2e

import (
	"testing"

	"github.com/llamerada-jp/colonio/seed"
	"github.com/llamerada-jp/colonio/test/util/server"
	"github.com/stretchr/testify/suite"
)

func TestNative(t *testing.T) {
	seed := seed.NewSeed()
	helper := server.NewHelper(seed)
	helper.Start(t.Context())
	defer helper.Stop()
	t.Log("start seed")

	suite.Run(t, &SingleNodeMessaging{
		seedURL: helper.URL(),
	})
	suite.Run(t, &E2eSuite{
		seedURL: helper.URL(),
	})
}
