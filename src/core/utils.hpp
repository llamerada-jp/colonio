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

#ifdef __clang__
#  include <picojson.h>
#else
#  pragma GCC diagnostic push
#  pragma GCC diagnostic ignored "-Wmaybe-uninitialized"
#  include <picojson.h>
#  pragma GCC diagnostic pop
#endif

#include <cassert>
#include <string>

#include "internal_exception.hpp"

namespace colonio {
class Packet;

/**
 * ASSERT macro is helper function to show message when assertion.
 * @param FORMAT Format string of an assersion message that similar to printf.
 */
#ifndef NDEBUG
#  define colonio_assert(EX, FORMAT, ...)                                                                      \
    if (!(EX)) {                                                                                               \
      Utils::output_assert(__func__, __FILE__, __LINE__, #EX, Utils::format_string(FORMAT, 0, ##__VA_ARGS__)); \
    }

#else
#  define colonio_assert(EX, FORMAT, ...) \
    if (!(EX)) {                          \
      exit(EXIT_FAILURE);                 \
    }
#endif

/**
 * THROW macro is helper function to throw exception with line number and file name.
 * @param FORMAT Format string of an exception message that similar to printf.
 */
#define colonio_throw(CODE, FORMAT, ...) \
  throw InternalException(__LINE__, __FILE__, CODE, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

/**
 * FATAL macro is helper function to throw fatal exception.
 * @param FORMAT Format string of an exception message that similar to printf.
 */
#define colonio_fatal(FORMAT, ...) \
  throw FatalException(__LINE__, __FILE__, Utils::format_string(FORMAT, 0, ##__VA_ARGS__))

namespace Utils {
std::string format_string(const std::string& format, int dummy, ...);
void output_assert(
    const std::string& func, const std::string& file, unsigned long line, const std::string& exp,
    const std::string& message);

template<typename T>
bool check_json_optional(const picojson::object& obj, const std::string& key, T* dst) {
  auto it = obj.find(key);
  if (it == obj.end() || it->second.is<picojson::null>()) {
    return false;

  } else if (it->second.is<T>()) {
    *dst = it->second.get<T>();
    return true;

  } else {
    colonio_assert(
        false, "Wrong json type.(key : %s, json : %s)", key.c_str(), picojson::value(obj).serialize().c_str());
    return false;
  }
}
template<>
bool check_json_optional<unsigned int>(const picojson::object& obj, const std::string& key, unsigned int* dst);

template<typename T>
T get_json(const picojson::object& obj, const std::string& key) {
  auto it = obj.find(key);
  if (it != obj.end() && it->second.is<T>()) {
    return it->second.get<T>();
  } else {
    colonio_fatal(
        "Key dose not exist in JSON.(key : %s, json : %s)", key.c_str(), picojson::value(obj).serialize().c_str());
  }
}

template<typename T>
T get_json(const picojson::object& obj, const std::string& key, const T& default_value) {
  auto it = obj.find(key);
  if (it != obj.end() && it->second.is<T>()) {
    return it->second.get<T>();

  } else {
    return default_value;
  }
}
template<>
unsigned int
get_json<unsigned int>(const picojson::object& obj, const std::string& key, const unsigned int& default_value);

std::string dump_binary(const std::string* bin);
std::string dump_packet(const Packet& packet, unsigned int indent = 2);
int64_t get_current_msec();
std::string get_current_thread_id();

template<typename T>
const T* get_json_value(const picojson::object& parent, const std::string& key) {
  auto it = parent.find(key);
  if (it == parent.end()) {
    return nullptr;

  } else {
    return &(it->second.get<T>());
  }
}

std::string file_basename(const std::string& path, bool cutoff_ext = false);
std::string file_dirname(const std::string& path);
bool is_safevalue(double v);
double float_mod(double a, double b);
void replace_string(std::string* str, const std::string& from, const std::string& to);
}  // namespace Utils
}  // namespace colonio
