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

var _ objectRenderer = &line2{}

type line2 struct {
	x1    float64
	y1    float64
	x2    float64
	y2    float64
	color *sdl.Color
}

func (l *line2) getZIndex() float64 {
	return 80
}

func (l *line2) render(context *context) {
	context.renderer.SetDrawColor(l.color.R, l.color.G, l.color.B, l.color.A)

	x1, y1 := context.getCanvasPosition(l.x1, l.y1)
	x2, y2 := context.getCanvasPosition(l.x2, l.y2)
	context.renderer.DrawLine(x1, y1, x2, y2)
}
