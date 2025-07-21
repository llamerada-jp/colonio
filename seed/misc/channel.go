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
	"fmt"
	"sync"
)

var ErrChannelClosed = fmt.Errorf("channel is closed")

type Channel[T any] struct {
	mtx sync.RWMutex
	c   chan T
}

func NewChannel[T any](size int) *Channel[T] {
	return &Channel[T]{
		c: make(chan T, size),
	}
}

func (c *Channel[T]) C() chan T {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	return c.c
}

func (c *Channel[T]) Send(value T) error {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	if c.c == nil {
		return ErrChannelClosed
	}

	c.c <- value
	return nil
}

func (c *Channel[T]) SendWhenNotFull(value T) (bool, error) {
	// this method exclusively locks the channel to check if it is full
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.c == nil {
		return false, ErrChannelClosed
	}

	if len(c.c) < cap(c.c) {
		c.c <- value
		return true, nil
	}
	return false, nil
}

func (c *Channel[T]) Close() {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.c == nil {
		return
	}
	close(c.c)
	c.c = nil
}
