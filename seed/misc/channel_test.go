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
package misc

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestChannel_Send(t *testing.T) {
	ch := NewChannel[int](2)
	sent := 0
	mtx := &sync.Mutex{}

	go func() {
		for i := 0; i < 3; i++ {
			err := ch.Send(i)
			assert.NoError(t, err)
			mtx.Lock()
			sent++
			mtx.Unlock()
		}
	}()

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return sent == 2
	}, 100*time.Millisecond, 10*time.Millisecond)

	v := <-ch.c
	assert.Equal(t, 0, v)

	assert.Eventually(t, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return sent == 3
	}, 100*time.Millisecond, 10*time.Millisecond)

	ch.Close()
	err := ch.Send(4)
	assert.ErrorIs(t, err, ErrChannelClosed)

	// can close channel multiple times
	ch.Close()
}

func TestChannel_SendWithNotFull(t *testing.T) {
	ch := NewChannel[int](2)

	for i := 0; i < 2; i++ {
		ok, err := ch.SendWhenNotFull(i)
		assert.NoError(t, err)
		assert.True(t, ok)
	}

	ok, err := ch.SendWhenNotFull(2)
	assert.NoError(t, err)
	assert.False(t, ok)

	v := <-ch.c
	assert.Equal(t, 0, v)

	ok, err = ch.SendWhenNotFull(3)
	assert.NoError(t, err)
	assert.True(t, ok)
}
