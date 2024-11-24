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
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	width  = 720
	height = 720
)

type Canvas struct {
	// canvas containing any instances of OpenGL
	program uint32

	window       *glfw.Window
	windowWidth  int
	windowHeight int
	pixelWidth   float64
	pixelHeight  float64
	rateX        float64
	rateY        float64

	filePrefix   string
	postfixDigit uint
	index        uint

	colorR float32
	colorG float32
	colorB float32
}

// NewCanvas makes new utility instance of OpenGL
func NewCanvas(width, height uint, filePrefix string, postfixDigit uint) *Canvas {
	if err := glfw.Init(); err != nil {
		log.Fatalln("failed to initialize glfw:", err)
	}

	glfw.WindowHint(glfw.Resizable, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(int(width), int(height), "colonio simulator", nil, nil)
	if err != nil {
		log.Fatalln("failed to CreateWindow:", err)
	}
	window.MakeContextCurrent()
	glfw.SwapInterval(1)

	if err := gl.Init(); err != nil {
		log.Fatalln("failed to initialize gl:", err)
	}

	canvas := &Canvas{
		window:       window,
		filePrefix:   filePrefix,
		postfixDigit: postfixDigit,
	}

	canvas.setupProgram()

	gl.ClearColor(1.0, 1.0, 1.0, 1.0)

	return canvas
}

// Quit OpenGL
func (c *Canvas) Quit() {
	glfw.Terminate()
}

// Loop swap and clear buffer, and poll events. return false is program should quit
func (c *Canvas) Loop() bool {
	c.window.SwapBuffers()

	if c.index != 0 && len(c.filePrefix) != 0 {
		c.saveImage()
	}
	c.index++

	// clear and draw
	defer func() {
		glfw.PollEvents()
		c.checkWindowSize()
		gl.Enable(gl.DEPTH_TEST)
		gl.DepthFunc(gl.LESS)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	}()

	return !c.window.ShouldClose()
}

// SetRGB set fill color
func (c *Canvas) SetRGB(red, green, blue float32) {
	c.colorR = red
	c.colorG = green
	c.colorB = blue
}

// Line3 draw a line at 3d coordinate space
func (c *Canvas) Line3(x1, y1, z1, x2, y2, z2 float64) {
	vertices := []float32{
		float32(x1), float32(y1), float32(z1),
		float32(x2), float32(y2), float32(z2),
	}

	fragments := []float32{
		c.colorR, c.colorG, c.colorB,
		c.colorR, c.colorG, c.colorB,
	}

	var vertexArrayObject uint32
	gl.GenVertexArrays(1, &vertexArrayObject)
	defer gl.DeleteVertexArrays(1, &vertexArrayObject)
	gl.BindVertexArray(vertexArrayObject)

	vertexBuffer := c.makeAndUseBuffer(0, vertices)
	defer gl.DeleteBuffers(1, &vertexBuffer)

	fragmentBuffer := c.makeAndUseBuffer(1, fragments)
	defer gl.DeleteBuffers(1, &fragmentBuffer)

	gl.DrawArrays(gl.LINES, 0, 2)

	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
}

// Point3 draw point at 3d coordinate space
func (c *Canvas) Point3(x, y, z float64) {
	pointWidth := 4.0 * c.pixelWidth
	pointHeight := 4.0 * c.pixelHeight
	vertices := []float32{
		float32(x - pointWidth), float32(y - pointHeight), float32(z),
		float32(x + pointWidth), float32(y - pointHeight), float32(z),
		float32(x + pointWidth), float32(y + pointHeight), float32(z),
		float32(x - pointWidth), float32(y - pointHeight), float32(z),
		float32(x - pointWidth), float32(y + pointHeight), float32(z),
	}

	fragments := []float32{
		c.colorR, c.colorG, c.colorB,
		c.colorR, c.colorG, c.colorB,
		c.colorR, c.colorG, c.colorB,
		c.colorR, c.colorG, c.colorB,
		c.colorR, c.colorG, c.colorB,
	}

	var vertexArrayObject uint32
	gl.GenVertexArrays(1, &vertexArrayObject)
	defer gl.DeleteVertexArrays(1, &vertexArrayObject)
	gl.BindVertexArray(vertexArrayObject)

	vertexBuffer := c.makeAndUseBuffer(0, vertices)
	defer gl.DeleteBuffers(1, &vertexBuffer)

	fragmentBuffer := c.makeAndUseBuffer(1, fragments)
	defer gl.DeleteBuffers(1, &fragmentBuffer)

	gl.DrawArrays(gl.TRIANGLES, 0, 3)
	gl.DrawArrays(gl.TRIANGLES, 2, 3)

	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
}

// Box3 draw box at 2d coordinate space
func (c *Canvas) Box3(x, y, z, w float64) {
	pointWidth := w * c.pixelWidth
	pointHeight := w * c.pixelHeight
	vertices := []float32{
		float32(x - pointWidth), float32(y - pointHeight), float32(z),
		float32(x + pointWidth), float32(y - pointHeight), float32(z),
		float32(x + pointWidth), float32(y + pointHeight), float32(z),
		float32(x - pointWidth), float32(y - pointHeight), float32(z),
		float32(x - pointWidth), float32(y + pointHeight), float32(z),
	}

	fragments := []float32{
		c.colorR, c.colorG, c.colorB,
		c.colorR, c.colorG, c.colorB,
		c.colorR, c.colorG, c.colorB,
		c.colorR, c.colorG, c.colorB,
		c.colorR, c.colorG, c.colorB,
	}

	var vertexArrayObject uint32
	gl.GenVertexArrays(1, &vertexArrayObject)
	defer gl.DeleteVertexArrays(1, &vertexArrayObject)
	gl.BindVertexArray(vertexArrayObject)

	vertexBuffer := c.makeAndUseBuffer(0, vertices)
	defer gl.DeleteBuffers(1, &vertexBuffer)

	fragmentBuffer := c.makeAndUseBuffer(1, fragments)
	defer gl.DeleteBuffers(1, &fragmentBuffer)

	gl.DrawArrays(gl.TRIANGLES, 0, 3)
	gl.DrawArrays(gl.TRIANGLES, 2, 3)

	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
}

func (c *Canvas) setupProgram() {
	program := gl.CreateProgram()
	vertexShader := c.setupShader(`
#version 330 core

layout(location = 0) in vec3 vertexPosition_modelspace;
layout(location = 1) in vec3 vertexColor;

out vec3 fragmentColor;
uniform mat4 MVP;

void main(){
	//gl_Position =  MVP * vec4(vertexPosition_modelspace,1);
	gl_Position = vec4(vertexPosition_modelspace.x, vertexPosition_modelspace.y, vertexPosition_modelspace.z, 1.0);
	fragmentColor = vertexColor;
}
`+"\x00", gl.VERTEX_SHADER)
	defer gl.DeleteShader(vertexShader)

	fragmentShader := c.setupShader(`
#version 330 core

in vec3 fragmentColor;
out vec3 color;

void main(){
	color = fragmentColor;
	// color = vec3(1.0, 1.0, 1.0);
}
`+"\x00", gl.FRAGMENT_SHADER)
	defer gl.DeleteShader(fragmentShader)

	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)

	gl.LinkProgram(program)
	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)

		lotText := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(lotText))
		log.Fatalf("failed to compile at setupProgram %v", lotText)
	}

	c.program = program
	gl.UseProgram(c.program)
}

func (c *Canvas) setupShader(source string, shaderType uint32) uint32 {
	shader := gl.CreateShader(shaderType)
	sourceChars, freeFunc := gl.Strs(source)
	defer freeFunc()
	gl.ShaderSource(shader, 1, sourceChars, nil)
	gl.CompileShader(shader)
	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)

		lotText := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(lotText))
		log.Fatalf("failed to compile at setupShader %v: %v", source, lotText)
	}

	return shader
}

func (c *Canvas) makeAndUseBuffer(location uint32, slice []float32) uint32 {
	var buffer uint32
	gl.GenBuffers(1, &buffer)
	gl.BindBuffer(gl.ARRAY_BUFFER, buffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(slice)*4, gl.Ptr(slice), gl.STREAM_DRAW)
	gl.VertexAttribPointer(location, 3, gl.FLOAT, false, 0, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(location)
	return buffer
}

func (c *Canvas) checkWindowSize() {
	width, height := c.window.GetSize()
	if width != c.windowWidth || height != c.windowHeight {
		c.windowWidth = width
		c.windowHeight = height
		c.pixelWidth = 1.0 / float64(width)
		c.pixelHeight = 1.0 / float64(height)
		if width > height {
			c.rateX = float64(height) / float64(width)
			c.rateY = 1.0
		} else {
			c.rateX = 1.0
			c.rateY = float64(width) / float64(height)
		}
	}
}

func (c *Canvas) saveImage() {
	fileName := fmt.Sprintf("%s%0."+strconv.Itoa(int(c.postfixDigit))+"d.png", c.filePrefix, c.index)

	dataBuffer := make([]uint8, c.windowWidth*c.windowHeight*3)

	gl.ReadBuffer(gl.BACK)

	gl.ReadPixels(0, 0, int32(c.windowWidth), int32(c.windowHeight),
		gl.BGR, gl.UNSIGNED_BYTE, gl.Ptr(&dataBuffer[0]))

	img := image.NewRGBA(image.Rect(0, 0, c.windowHeight, c.windowHeight))
	idx := 0
	for y := 0; y < c.windowHeight; y++ {
		for x := 0; x < c.windowWidth; x++ {
			img.Set(x, c.windowHeight-y-1, color.RGBA{dataBuffer[idx+2], dataBuffer[idx+1], dataBuffer[idx], 255})
			idx += 3
		}
	}

	f, err := os.Create(fileName)
	if err != nil {
		log.Fatalln("File create error : ", err)
	}
	defer f.Close()

	err = png.Encode(f, img)
	if err != nil {
		log.Fatalln("Encodeing png error : ", err)
	}
}
