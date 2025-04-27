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

type Region struct {
	X, Y float64
	R    float64
}

type Position struct {
	X, Y   float64
	ax, ay float64
	region *Region
}

func NewPosition(region *Region) *Position {
	var pos *Position

	if region != nil {
		// choice random position in the region
		randR := rand.Float64() * region.R
		randTheta := rand.Float64() * math.Pi * 2
		pos = &Position{
			X:      region.X + randR*math.Cos(randTheta),
			Y:      region.Y + randR*math.Sin(randTheta),
			ax:     (rand.Float64() - 0.5) * 0.01 * region.R,
			ay:     (rand.Float64() - 0.5) * 0.01 * region.R,
			region: region,
		}

		if !pos.isInsideRegion(pos.X, pos.Y) {
			panic("initial position is not in the region")
		}

	} else {
		pos = &Position{
			X:  rand.Float64()*math.Pi*2 - math.Pi,
			Y:  rand.Float64()*math.Pi - math.Pi/2,
			ax: (rand.Float64() - 0.5) * 0.01,
			ay: (rand.Float64() - 0.5) * 0.01,
		}
	}

	return pos
}

func (p *Position) MoveRandom() {
	if p.region != nil {
		for {
			px := p.X + p.ax
			py := p.Y + p.ay
			if p.isInsideRegion(px, py) {
				p.X = px
				p.Y = py
				break
			}
			p.ax = (rand.Float64() - 0.5) * 0.1 * p.region.R
			p.ay = (rand.Float64() - 0.5) * 0.1 * p.region.R
		}

	} else {
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
}

func (p *Position) isInsideRegion(px, py float64) bool {
	if p.region == nil {
		panic("region is nil")
	}

	dx := px - p.region.X
	dy := py - p.region.Y
	return dx*dx+dy*dy <= p.region.R*p.region.R
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
