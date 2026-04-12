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
package geometry

import (
	"fmt"
	"math"
	"math/rand"
)

type planeCoordinateSystem struct {
	xMin          float64
	xMax          float64
	yMin          float64
	yMax          float64
	localPosition Coordinate
}

func NewPlaneCoordinateSystem(xMin, xMax, yMin, yMax float64) CoordinateSystem {
	return &planeCoordinateSystem{
		xMin: xMin,
		xMax: xMax,
		yMin: yMin,
		yMax: yMax,
		localPosition: Coordinate{
			X: rand.Float64()*(xMax-xMin) + xMin,
			Y: rand.Float64()*(yMax-yMin) + yMin,
		},
	}
}

func (p *planeCoordinateSystem) GetDistance(p1, p2 *Coordinate) float64 {
	return math.Sqrt(math.Pow(p1.X-p2.X, 2) + math.Pow(p1.Y-p2.Y, 2))
}

func (p *planeCoordinateSystem) GetLocalPosition() Coordinate {
	return p.localPosition
}

func (p *planeCoordinateSystem) GetPrecision() float64 {
	return 1.0 / math.MaxUint32
}

func (p *planeCoordinateSystem) SetLocalPosition(position *Coordinate) error {
	if position.X < p.xMin || p.xMax < position.X {
		return fmt.Errorf("the specified X coordinate is out of range (x:%f, min:%f, max:%f)", position.X, p.xMin, p.xMax)
	}

	if position.Y < p.yMin || p.yMax < position.Y {
		return fmt.Errorf("the specified Y coordinate is out of range (y:%f, min:%f, max:%f)", position.Y, p.yMin, p.yMax)
	}

	p.localPosition = *position

	return nil
}

func (p *planeCoordinateSystem) Shift(base, position *Coordinate) *Coordinate {
	return &Coordinate{
		X: position.X - base.X,
		Y: position.Y - base.Y,
	}
}
