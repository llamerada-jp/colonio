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

import "sync"

type WaitAny struct {
	mtx     sync.Mutex
	cond    *sync.Cond
	remains int
	result  bool
}

func NewWaitAny() *WaitAny {
	wa := &WaitAny{}
	wa.cond = sync.NewCond(&wa.mtx)
	return wa
}

func (w *WaitAny) Add(delta int) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	w.remains += delta
}

func (w *WaitAny) Done(result bool) {
	w.cond.L.Lock()
	defer w.cond.L.Unlock()

	if w.remains <= 0 {
		panic("negative WaitAny counter")
	}

	w.remains -= 1
	if result {
		w.result = true
	}

	w.cond.Broadcast()
}

func (w *WaitAny) Wait() bool {
	w.cond.L.Lock()
	defer w.cond.L.Unlock()

	for !w.result && w.remains != 0 {
		w.cond.Wait()
	}

	return w.result
}
