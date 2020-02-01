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

#include "convert.hpp"

#include <cassert>
#include <iomanip>
#include <iostream>
#include <string>
#include <vector>

#include "utils.hpp"

namespace colonio {

template<>
std::string Convert::int2str<uint8_t>(uint8_t num) {
  std::ostringstream os;
  os << std::hex << std::setfill('0') << std::setw(sizeof(uint8_t) * 2) << static_cast<uint32_t>(num);
  return os.str();
}

template<>
uint8_t Convert::str2int<uint8_t>(const std::string& str) {
  std::istringstream is(str);
  uint32_t v;
  is >> std::hex >> v;
  return static_cast<uint8_t>(v);
}

// Convert JSON to binary data.
std::string Convert::json2bin(const picojson::value& json) {
  const std::string& js_str = json.get<std::string>();
  std::vector<uint8_t> bin;
  for (unsigned int i = 0; i < js_str.size(); i += 2) {
    std::string hex(js_str, i, 2);
    bin.push_back(0xFF & std::stoi(hex, nullptr, 16));
  }
  return std::string(reinterpret_cast<char*>(bin.data()), bin.size());
}

Coordinate Convert::json2coordinate(const picojson::value& json) {
  const picojson::array& arr = json.get<picojson::array>();
  return Coordinate(arr.at(0).get<double>(), arr.at(1).get<double>());
}

/**
 * Convert JSON to time(sec).
 * This function convert JSON to time by way of relative time.
 * Timers may different from each other.
 * It suppose that the period to transport packet is very short.
 * The packet contain relative time from when the packet has send.
 * @param json Source JSON.
 * @return A time typed std::time_t.
 */
std::time_t Convert::json2time(const picojson::value& json) {
  int32_t diff = json2int<int32_t>(json);
  return time(nullptr) + diff;
}

// Convert binary data contained string type to JSON.
picojson::value Convert::bin2json(const std::string& bin) {
  return bin2json(reinterpret_cast<const uint8_t*>(bin.data()), bin.size());
}

// Convert binary data to JSON.
picojson::value Convert::bin2json(const uint8_t* bin, unsigned int size) {
  std::ostringstream os;
  os << std::hex << std::setfill('0');
  for (unsigned int i = 0; i < size; i++) {
    os << std::setw(2) << (0xFF & static_cast<int>(bin[i]));
  }
  return picojson::value(os.str());
}

picojson::value Convert::coordinate2json(const Coordinate& coordinate) {
  assert(Utils::is_safevalue(coordinate.x) && Utils::is_safevalue(coordinate.y));
  picojson::array arr;
  arr.push_back(picojson::value(coordinate.x));
  arr.push_back(picojson::value(coordinate.y));
  return picojson::value(arr);
}
}  // namespace colonio
