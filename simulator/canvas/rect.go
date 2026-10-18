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
	"github.com/veandco/go-sdl2/sdl"
)

var _ objectRenderer = &rect{}

type rect struct {
	x, y  int32
	w, h  int32
	color *sdl.Color
}

func (b *rect) getZIndex() float64 {
	return 80
}

func (b *rect) render(context *context) {
	context.renderer.SetDrawColor(b.color.R, b.color.G, b.color.B, b.color.A)

	rect := sdl.Rect{
		X: b.x,
		Y: b.y,
		W: b.w,
		H: b.h,
	}
	context.renderer.FillRect(&rect)
}
