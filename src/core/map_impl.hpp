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

#include "api_gate.hpp"
#include "colonio/map.hpp"

namespace colonio {
class MapImpl : public Map {
 public:
  MapImpl(APIGate& api_gate_, APIChannel::Type channel_);

  Value get(const Value& key) override;
  void set(const Value& key, const Value& value, MapOption::Type opt = MapOption::NONE) override;

 private:
  APIGate& api_gate;
  APIChannel::Type channel;
};
}  // namespace colonio
