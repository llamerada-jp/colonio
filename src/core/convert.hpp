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

#ifdef __clang__
#  include <picojson.h>
#else
#  pragma GCC diagnostic push
#  pragma GCC diagnostic ignored "-Wmaybe-uninitialized"
#  include <picojson.h>
#  pragma GCC diagnostic pop
#endif

#include <ctime>
#include <iomanip>
#include <istream>
#include <sstream>
#include <string>
#include <vector>

#include "coordinate.hpp"
#include "definition.hpp"

namespace colonio {
namespace Convert {
/**
 * Convert integer to string.
 * @param num A source integer.
 * @return A integer as string.
 */
template<class T>
std::string int2str(T num) {
  std::ostringstream os;
  os << std::hex << std::setfill('0') << std::setw(sizeof(T) * 2) << num;
  return os.str();
}
template<>
std::string int2str<uint8_t>(uint8_t num);

/**
 * Convert string to integer.
 * @param str A source string.
 * @return A converted integer.
 */
template<class T>
T str2int(const std::string& str) {
  std::istringstream is(str);
  T v;
  is >> std::hex >> v;
  return v;
}
template<>
uint8_t str2int<uint8_t>(const std::string& str);

/**
 * Convert JSON to bool.
 * @param json Source JSON.
 * @return A converted integer.
 */
inline bool json2bool(const picojson::value& json) {
  return json.get<std::string>() == "T";
}

Coordinate json2coordinate(const picojson::value& json);

template<class T>
T json2enum(const picojson::value& json) {
  // @TODO check range
  return static_cast<T>(str2int<uint32_t>(json.get<std::string>()));
}

/**
 * Convert JSON to integer.
 * @param json Source JSON.
 * @return A converted integer.
 */
template<class T>
T json2int(const picojson::value& json) {
  return str2int<T>(json.get<std::string>());
}

std::time_t json2time(const picojson::value& json);

/**
 * Convert JSON to binary data.
 * It have JSON to contain binary data as hex string.
 * @param json Source JSON.
 * @return Binary data.
 */
std::string json2bin(const picojson::value& json);

/**
 * Convert bool to JSON.
 * @param b Source boolean.
 * @return Boolean as JSON.
 */
inline picojson::value bool2json(bool b) {
  return picojson::value(std::string(b ? "T" : "F"));
}

/**
 * Convert integer to JSON.
 * @param num Source integer.
 * @return Integer as JSON.
 */
template<class T>
picojson::value int2json(T num) {
  return picojson::value(int2str<T>(num));
}

template<class T>
picojson::value enum2json(T e) {
  return picojson::value(int2json(static_cast<uint32_t>(e)));
}

/**
 * Convert binary data contained string type to JSON.
 * Binary data is converted to hex string and packed by JSON.
 * @param bin Source binary data.
 * @return Binary data as JSON.
 */
picojson::value bin2json(const std::string& bin);

/**
 * Convert binary data to JSON.
 * Binary data is converted to hex string and packed by JSON.
 * @param bin Source binary data.
 * @param size Source binary data size.
 * @return Binary data as JSON.
 */
picojson::value bin2json(const uint8_t* bin, unsigned int size);

picojson::value coordinate2json(const Coordinate& coordinate);

}  // namespace Convert
}  // namespace colonio
