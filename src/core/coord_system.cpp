/*
 * Copyright 2017 Yuji Ito <llamerada.jp@gmail.com>
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

#include "coord_system.hpp"

namespace colonio {
CoordSystem::CoordSystem(double min_x, double min_y, double max_x, double max_y, double precision) :
    MIN_X(min_x), MIN_Y(min_y), MAX_X(max_x), MAX_Y(max_y), PRECISION(precision) {
}

CoordSystem::~CoordSystem() {
}
}  // namespace colonio
