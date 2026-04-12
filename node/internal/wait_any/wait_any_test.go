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
package wait_any

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWaitAny_true(t *testing.T) {
	wa := NewWaitAny()
	wa.Add(3)

	go func() {
		wa.Done(false)
		wa.Done(true)
	}()

	assert.True(t, wa.Wait())
}

func TestWaitAny_false(t *testing.T) {
	wa := NewWaitAny()
	wa.Add(3)

	go func() {
		for i := 0; i < 3; i++ {
			wa.Done(false)
		}
	}()

	assert.False(t, wa.Wait())
}
