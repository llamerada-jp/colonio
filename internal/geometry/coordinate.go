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
	"github.com/llamerada-jp/colonio/internal/proto"
)

type Coordinate struct {
	X float64
	Y float64
}

func NewCoordinate(x, y float64) *Coordinate {
	return &Coordinate{
		X: x,
		Y: y,
	}
}

func NewCoordinateFromProto(p *proto.Coordinate) *Coordinate {
	return &Coordinate{
		X: p.X,
		Y: p.Y,
	}
}

func (c *Coordinate) Proto() *proto.Coordinate {
	return &proto.Coordinate{
		X: c.X,
		Y: c.Y,
	}
}

func (c *Coordinate) Equal(c2 *Coordinate) bool {
	return c.X == c2.X && c.Y == c2.Y
}

func (c *Coordinate) Smaller(c2 *Coordinate) bool {
	if c.X == c2.X {
		return c.Y < c2.Y
	} else {
		return c.X < c2.X
	}
}
