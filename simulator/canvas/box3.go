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

import "github.com/veandco/go-sdl2/sdl"

var _ objectRenderer = &box3{}

type box3 struct {
	x     float64
	y     float64
	z     float64
	width float64
	color *sdl.Color
}

func (b *box3) getZIndex() float64 {
	return b.z
}

func (b *box3) render(context *context) {
	color := context.getColorByZIndex(b.color, b.z)
	context.renderer.SetDrawColor(color.R, color.G, color.B, color.A)

	x, y := context.getCanvasPosition(b.x, b.y)
	rect := sdl.Rect{
		X: x - int32(b.width/2.0),
		Y: y - int32(b.width/2.0),
		W: int32(b.width),
		H: int32(b.width),
	}
	context.renderer.FillRect(&rect)
}
