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
package delay

import (
	"math"
	"strconv"
)

type location struct {
	longitude, latitude float64
}

func newLocationByString(lon, lat string) (*location, error) {
	lonFloat, err := strconv.ParseFloat(lon, 64)
	if err != nil {
		return nil, err
	}
	latFloat, err := strconv.ParseFloat(lat, 64)
	if err != nil {
		return nil, err
	}
	return &location{longitude: lonFloat, latitude: latFloat}, nil
}

func (l *location) distance(other *location) float64 {
	// Haversine formula
	const R = 6371 // Radius of the Earth in kilometers
	dLat := (other.latitude - l.latitude) * (math.Pi / 180)
	dLon := (other.longitude - l.longitude) * (math.Pi / 180)
	a := (math.Sin(dLat/2) * math.Sin(dLat/2)) +
		(math.Cos(l.latitude*(math.Pi/180)) * math.Cos(other.latitude*(math.Pi/180)) *
			math.Sin(dLon/2) * math.Sin(dLon/2))
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
