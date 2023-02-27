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
#pragma once

#include "async_helper.hpp"
#include "log_helper.hpp"
#include "test_seed.hpp"

bool check_bin_equal(const void* ptr1, const void* ptr2, std::size_t siz) {
  const uint8_t* bin1 = reinterpret_cast<const uint8_t*>(ptr1);
  const uint8_t* bin2 = reinterpret_cast<const uint8_t*>(ptr2);

  for (std::size_t i = 0; i < siz; i++) {
    if (bin1[i] != bin2[i]) {
      return false;
    }
  }

  return true;
}