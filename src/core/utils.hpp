/*
 * Copyright 2017-2019 Yuji Ito <llamerada.jp@gmail.com>
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

#include <string>
#include <picojson.h>

namespace colonio {
struct Packet;

namespace Utils {
template<typename T>
bool check_json_optional(const picojson::object& obj, const std::string& key, T* dst) {
  auto it = obj.find(key);
  if (it == obj.end()) {
    return false;

  } else if (it->second.is<T>()) {
    *dst = it->second.get<T>();
    return true;

  } else {
    // @todo error
    assert(false);
    throw;
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
    // @todo error
    assert(false);
    throw;
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
unsigned int get_json<unsigned int>(const picojson::object& obj, const std::string& key,
                                    const unsigned int& default_value);

std::string dump_packet(const Packet& packet, unsigned int indent = 0);
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
picojson::object& insert_get_json_object(picojson::object& parent, const std::string& key);
bool is_safevalue(double v);
double pmod(double a, double b);
void replace_string(std::string* str, const std::string& from, const std::string& to);
}  // namespace Utils
}  // namespace colonio
