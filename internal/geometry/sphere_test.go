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
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSphere(t *testing.T) {
	cs := NewSphereCoordinateSystem(1)

	// Check default position
	p := cs.GetLocalPosition()
	assert.True(t, -1*math.Pi <= p.X && p.X < math.Pi)
	assert.True(t, -0.5*math.Pi <= p.Y && p.Y < 0.5*math.Pi)

	// Test SetLocalPosition
	pNew := &Coordinate{X: 2, Y: 1}
	err := cs.SetLocalPosition(pNew)
	assert.NoError(t, err)

	// Test SetLocalPosition (out of range)
	pInvalid := &Coordinate{X: 2, Y: 2}
	err = cs.SetLocalPosition(pInvalid)
	assert.Error(t, err)

	// Test GetLocalPosition
	local := cs.GetLocalPosition()
	assert.True(t, local.Equal(pNew))
}

func TestSphereDistance(t *testing.T) {
	tests := []struct {
		r  float64
		p1 *Coordinate
		p2 *Coordinate
		d  float64
	}{
		{
			r:  3,
			p1: &Coordinate{X: 0, Y: 0},
			p2: &Coordinate{X: 0, Y: 0},
			d:  0,
		},
		{
			r:  10,
			p1: &Coordinate{X: 0, Y: 0},
			p2: &Coordinate{X: 0.5 * math.Pi, Y: 0},
			d:  5 * math.Pi,
		},
		{
			r:  1,
			p1: &Coordinate{X: -0.5 * math.Pi, Y: 0.25 * math.Pi},
			p2: &Coordinate{X: 0.5 * math.Pi, Y: 0.25 * math.Pi},
			d:  math.Pi / 2,
		},
	}

	for i, test := range tests {
		t.Logf("test %d", i)
		cs := NewSphereCoordinateSystem(test.r)

		assert.InDelta(t, test.d, cs.GetDistance(test.p1, test.p2), 1e-10)
	}
}

func TestSphereShift(t *testing.T) {
	cs := NewSphereCoordinateSystem(10)

	tests := []struct {
		base     *Coordinate
		position *Coordinate
		expect   *Coordinate
	}{
		{
			base:     &Coordinate{X: 0, Y: 0},
			position: &Coordinate{X: 0, Y: 0},
			expect:   &Coordinate{X: 0, Y: 0},
		},
		{
			base:     &Coordinate{X: 0, Y: 0},
			position: &Coordinate{X: 1, Y: 0},
			expect:   &Coordinate{X: 1, Y: 0},
		},
	}

	for i, test := range tests {
		t.Logf("test %d", i)
		shifted := cs.Shift(test.base, test.position)
		assert.InDelta(t, test.expect.X, shifted.X, 1e-10)
		assert.InDelta(t, test.expect.Y, shifted.Y, 1e-10)
	}
}
