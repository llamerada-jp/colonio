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
	"slices"

	"github.com/veandco/go-sdl2/img"
	"github.com/veandco/go-sdl2/sdl"
)

type context struct {
	width    float64
	height   float64
	surface  *sdl.Surface
	renderer *sdl.Renderer
}

func (c *context) getCanvasPosition(x, y float64) (int32, int32) {
	return int32((x + 1.0) * c.width / 2.0), int32((1.0 - y) * c.height / 2.0)
}

func (c *context) getColorByZIndex(src *sdl.Color, z float64) *sdl.Color {
	rate := (1.0-z)/2.0 - 0.2
	if rate < 0.0 {
		rate = 0.0
	}
	if rate > 1.0 {
		rate = 1.0
	}

	return &sdl.Color{
		R: uint8(float64(255-src.R)*rate) + src.R,
		G: uint8(float64(255-src.G)*rate) + src.G,
		B: uint8(float64(255-src.B)*rate) + src.B,
		A: src.A,
	}
}

type Canvas struct {
	quitSignal   bool
	window       *sdl.Window
	context      *context
	currentColor *sdl.Color
	objects      []objectRenderer
}

type objectRenderer interface {
	getZIndex() float64
	render(context *context)
}

// NewCanvas makes new utility instance of OpenGL
func NewCanvas(width, height uint) *Canvas {
	if sdl.WasInit(sdl.INIT_EVENTS) == 0 {
		if err := sdl.Init(sdl.INIT_EVERYTHING); err != nil {
			panic(err)
		}
	}

	window, err := sdl.CreateWindow("colonio",
		sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		int32(width), int32(height), sdl.WINDOW_SHOWN)
	if err != nil {
		panic(err)
	}
	window.SetResizable(false)

	surface, err := window.GetSurface()
	if err != nil {
		panic(err)
	}

	renderer, err := sdl.CreateSoftwareRenderer(surface)
	if err != nil {
		panic(err)
	}

	return &Canvas{
		quitSignal: false,
		window:     window,
		context: &context{
			width:    float64(width),
			height:   float64(height),
			surface:  surface,
			renderer: renderer,
		},
		objects: []objectRenderer{},
	}
}

func (c *Canvas) HasQuitSignal() bool {
	if c.quitSignal {
		return true
	}

	for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
		switch t := event.(type) {
		case *sdl.QuitEvent:
			c.quitSignal = true
		case *sdl.KeyboardEvent:
			if t.Keysym.Sym == sdl.K_ESCAPE {
				c.quitSignal = true
			}
		}
	}

	return c.quitSignal
}

func (c *Canvas) Destroy() {
	c.context.renderer.Destroy()
	// c.surface.Free()
	c.window.Destroy()
	sdl.Quit()
}

func (c *Canvas) Clear() {
	c.objects = []objectRenderer{}
}

func (c *Canvas) Render() error {
	c.context.renderer.SetDrawColor(255, 255, 255, 255)
	c.context.renderer.Clear()

	slices.SortFunc(c.objects, func(i, j objectRenderer) int {
		zi := i.getZIndex()
		zj := j.getZIndex()
		if zi < zj {
			return 1
		}
		if zi > zj {
			return -1
		}
		return 0
	})

	for _, obj := range c.objects {
		obj.render(c.context)
	}
	c.objects = []objectRenderer{}

	return c.window.UpdateSurface()
}

func (c *Canvas) SetColor(r, g, b uint8) {
	c.currentColor = &sdl.Color{
		R: r,
		G: g,
		B: b,
		A: 255,
	}
}

func (c *Canvas) DrawLine3(x1, y1, z1, x2, y2, z2 float64) {
	c.objects = append(c.objects, &line3{
		x1:    x1,
		y1:    y1,
		z1:    z1,
		x2:    x2,
		y2:    y2,
		z2:    z2,
		color: c.currentColor,
	})
}

func (c *Canvas) DrawBox3(x, y, z, w float64) {
	c.objects = append(c.objects, &box3{
		x:     x,
		y:     y,
		z:     z,
		width: w,
		color: c.currentColor,
	})
}

func (c *Canvas) savePNG(filename string) error {
	return img.SavePNG(c.context.surface, filename)
}
