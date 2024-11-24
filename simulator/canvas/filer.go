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
package canvas

import (
	"fmt"
)

type Filer struct {
	canvas *Canvas
	prefix string
	digits int
	index  int
}

func NewFiler(canvas *Canvas, prefix string, digits int) *Filer {
	return &Filer{
		canvas: canvas,
		prefix: prefix,
		digits: digits,
		index:  0,
	}
}

func (f *Filer) Save() error {
	filename := fmt.Sprintf("%s%0*d.png", f.prefix, f.digits, f.index)
	if err := f.canvas.savePNG(filename); err != nil {
		return err
	}
	f.index++
	return nil
}
