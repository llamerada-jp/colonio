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
	"testing"

	"github.com/llamerada-jp/colonio/config"
	"github.com/stretchr/testify/assert"
)

var (
	testPlaneConfig = &config.GeometryPlane{
		XMin: 0,
		XMax: 100,
		YMin: 0,
		YMax: 100,
	}
)

func TestPlane(t *testing.T) {
	cs := NewPlaneCoordinateSystem(testPlaneConfig)

	// Check default position
	p := cs.GetLocalPosition()
	assert.True(t, 0 <= p.X && p.X < 100)
	assert.True(t, 0 <= p.Y && p.Y < 100)

	// Test SetLocalPosition
	pNew := &Coordinate{X: 50, Y: 50}
	err := cs.SetLocalPosition(pNew)
	assert.NoError(t, err)

	// Test SetLocalPosition (out of range)
	pInvalid := &Coordinate{X: 101, Y: 50}
	err = cs.SetLocalPosition(pInvalid)
	assert.Error(t, err)

	// Test GetLocalPosition
	local := cs.GetLocalPosition()
	assert.True(t, local.Equal(pNew))
}

func TestPlaneDistance(t *testing.T) {
	cs := NewPlaneCoordinateSystem(testPlaneConfig)

	// Test GetDistance
	p1 := &Coordinate{X: 0, Y: 0}
	p2 := &Coordinate{X: 3, Y: 4}
	assert.Equal(t, 5.0, cs.GetDistance(p1, p2))
}

func TestPlaneShift(t *testing.T) {
	cs := NewPlaneCoordinateSystem(testPlaneConfig)

	tests := []struct {
		base     *Coordinate
		position *Coordinate
		expect   *Coordinate
	}{
		{
			base:     &Coordinate{X: 0, Y: 0},
			position: &Coordinate{X: 3, Y: 4},
			expect:   &Coordinate{X: 3, Y: 4},
		},
		{
			base:     &Coordinate{X: 3, Y: 4},
			position: &Coordinate{X: 0, Y: 0},
			expect:   &Coordinate{X: -3, Y: -4},
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			p := cs.Shift(tt.base, tt.position)
			assert.True(t, p.Equal(tt.expect))
		})
	}
}
