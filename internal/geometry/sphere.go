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

const (
	sphereXMin = -1.0 * math.Pi
	sphereXMax = math.Pi
	sphereYMin = -0.5 * math.Pi
	sphereYMax = 0.5 * math.Pi
)

type sphereCoordinateSystem struct {
	radius        float64
	localPosition Coordinate
}

func NewSphereCoordinateSystem(radius float64) CoordinateSystem {
	return &sphereCoordinateSystem{
		radius: radius,
		localPosition: Coordinate{
			X: rand.Float64()*(sphereXMax-sphereXMin) + sphereXMin,
			Y: rand.Float64()*(sphereYMax-sphereYMin) + sphereYMin,
		},
	}
}

func (s *sphereCoordinateSystem) GetDistance(p1, p2 *Coordinate) float64 {
	avrX := (p1.X - p2.X) / 2
	avrY := (p1.Y - p2.Y) / 2

	return s.radius * 2 *
		math.Asin(math.Sqrt(math.Pow(math.Sin(avrY), 2)+math.Cos(p1.Y)*math.Cos(p2.Y)*math.Pow(math.Sin(avrX), 2)))
}

func (s *sphereCoordinateSystem) GetLocalPosition() Coordinate {
	return s.localPosition
}

func (s *sphereCoordinateSystem) GetPrecision() float64 {
	return 0.0001 / 60.0
}

func (s *sphereCoordinateSystem) SetLocalPosition(position *Coordinate) error {
	if position.X < sphereXMin || sphereXMax < position.X {
		return fmt.Errorf("the specified X coordinate is out of range (x:%f)", position.X)
	}

	if position.Y < sphereYMin || sphereYMax < position.Y {
		return fmt.Errorf("the specified Y coordinate is out of range (y:%f)", position.Y)
	}

	s.localPosition = *position

	return nil
}

func (s *sphereCoordinateSystem) Shift(base, position *Coordinate) *Coordinate {
	// this method is vary simple and not accurate
	// especially, it is buggy around the poles
	x := position.X - base.X
	if x < sphereXMin {
		x += 2 * math.Pi
	}
	if x >= sphereXMax {
		x -= 2 * math.Pi
	}
	y := position.Y - base.Y
	if y < sphereYMin {
		y += math.Pi
		x = -x
	}
	if y >= sphereYMax {
		y -= math.Pi
		x = -x
	}
	return &Coordinate{
		X: x,
		Y: y,
	}
}
