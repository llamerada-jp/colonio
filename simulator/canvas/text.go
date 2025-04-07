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
	"log"
	"strings"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

var _ objectRenderer = &text{}

type text struct {
	x       float64
	y       float64
	font    *ttf.Font
	color   *sdl.Color
	caption string
}

func (t *text) getZIndex() float64 {
	return 90
}

func (t *text) render(context *context) {
	lines := strings.Split(t.caption, "\n")
	hSum := int32(0)
	for _, line := range lines {
		surface, err := t.font.RenderUTF8Blended(line, *t.color)
		if err != nil {
			log.Fatalf("failed to render text: %v", err)
		}
		defer surface.Free()

		texture, err := context.renderer.CreateTextureFromSurface(surface)
		if err != nil {
			log.Fatalf("failed to create texture from surface: %v", err)
		}
		defer texture.Destroy()
		x, y := context.getCanvasPosition(t.x, t.y)
		rect := sdl.Rect{
			X: x,
			Y: y + hSum,
			W: surface.W,
			H: surface.H,
		}
		if err := context.renderer.Copy(texture, nil, &rect); err != nil {
			log.Fatalf("failed to copy texture: %v", err)
		}
		hSum += surface.H
	}
}
