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
package utils

import (
	"math"
	"math/rand"
)

type Position struct {
	X, Y   float64
	ax, ay float64
}

func NewPosition() *Position {
	return &Position{
		X:  rand.Float64()*math.Pi*2 - math.Pi,
		Y:  rand.Float64()*math.Pi - math.Pi/2,
		ax: rand.Float64() * 0.01,
		ay: rand.Float64() * 0.01,
	}
}

func (p *Position) MoveRandom() {
	p.ax = updateAcceleration(p.ax)
	p.ay = updateAcceleration(p.ay)
	p.X += p.ax
	p.Y += p.ay

	if p.X > math.Pi {
		p.X -= math.Pi * 2
	}
	if p.X < -math.Pi {
		p.X += math.Pi * 2
	}
	if p.Y > math.Pi/2 {
		p.Y = math.Pi / 2
		p.ay = -p.ay
	}
	if p.Y < -math.Pi/2 {
		p.Y = -math.Pi / 2
		p.ay = -p.ay
	}
}

func updateAcceleration(a float64) float64 {
	a += rand.Float64()*0.001 - 0.0005
	if a > 0.01 {
		a = 0.01
	}
	if a < -0.01 {
		a = -0.01
	}
	return a
}
