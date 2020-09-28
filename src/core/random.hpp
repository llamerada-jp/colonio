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
#pragma once

#include <cstdint>
#include <random>

namespace colonio {

class Random {
 public:
  Random();
  virtual ~Random();

  uint32_t generate_u32();
  uint32_t generate_u32(uint32_t min, uint32_t max);
  uint64_t generate_u64();
  double generate_double(double min, double max);

 private:
  std::mt19937 rnd32;
  std::mt19937_64 rnd64;

  Random(const Random&);
  void operator=(const Random&);
};

}  // namespace colonio
