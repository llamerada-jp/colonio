/*
 * Copyright 2017-2020 Yuji Ito <llamerada.jp@gmail.com>
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

#include "random.hpp"

// Random value generator.
#ifndef EMSCRIPTEN
unsigned int generate_seed() {
  std::random_device seed_gen;
  unsigned int seed = seed_gen();
  // avoid WSL2 issue https://github.com/microsoft/WSL/issues/5767
  if (seed == 0xFFFFFFFF) {
    seed = static_cast<unsigned int>(clock());
  }
  return seed;
}
#else
extern "C" {
extern double utils_get_random_seed();
}

int generate_seed() {
  return INT_MAX * utils_get_random_seed();
}
#endif

namespace colonio {

Random::Random() : rnd32(generate_seed()), rnd64(generate_seed()) {
}

Random::~Random() {
}

uint32_t Random::generate_u32() {
  return rnd32();
}

uint32_t Random::generate_u32(uint32_t min, uint32_t max) {
  std::uniform_int_distribution<uint32_t> dist(min, max);
  return dist(rnd32);
}

uint64_t Random::generate_u64() {
  return rnd64();
}

double Random::generate_double(double min, double max) {
  std::uniform_real_distribution<double> dist(min, max);
  return dist(rnd32);
}
}  // namespace colonio
