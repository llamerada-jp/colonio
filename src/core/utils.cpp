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
#include <libgen.h>
#if defined(__APPLE__) && defined(__MACH__)
#  include <mach-o/dyld.h>
#endif
#ifdef __linux__
#  include <unistd.h>
#endif
#include <sys/param.h>

#ifdef EMSCRIPTEN
#  include <emscripten.h>
#else
#  include <ctime>
#endif

#include <cassert>
#include <cstdarg>
#include <cstring>
#include <iomanip>
#include <memory>
#include <sstream>
#include <string>
#include <thread>

#include "definition.hpp"
#include "packet.hpp"
#include "utils.hpp"

namespace colonio {

template<>
bool Utils::check_json_optional<unsigned int>(const picojson::object& obj, const std::string& key, unsigned int* dst) {
  auto it = obj.find(key);
  if (it == obj.end() || it->second.is<picojson::null>()) {
    return false;

  } else if (it->second.is<double>()) {
    *dst = it->second.get<double>();
    return true;

  } else {
    // @todo error
    assert(false);
    throw;
  }
}

template<>
unsigned int
Utils::get_json<unsigned int>(const picojson::object& obj, const std::string& key, const unsigned int& default_value) {
  auto it = obj.find(key);
  if (it != obj.end() && it->second.is<double>()) {
    return it->second.get<double>();

  } else {
    return default_value;
  }
}

std::string Utils::dump_binary(const std::string* bin) {
  return dump_binary(bin->c_str(), bin->size());
}

std::string Utils::dump_binary(const void* bin, std::size_t len) {
  if (bin == nullptr) {
    return "null";
  }

  std::stringstream out;

  out << std::hex;
  for (unsigned int idx = 0; idx < len; idx++) {
    if (idx != 0) {
      out << " ";
    }
    out << std::setw(2) << std::setfill('0') << (0xFF & reinterpret_cast<const uint8_t*>(bin)[idx]);
  }

  return out.str();
}

/**
 * This method returns a formatted text like the printf method.
 * Format rule is the same as the printf, because this method uses snprintf inner it.
 * @param format string.
 * @param dummy dummy parameter, should be set 0.
 * @return formatted string as same as the printf.
 */
std::string Utils::format_string(const std::string& format, int dummy, ...) {
  std::vector<char> buffer(format.size() + 2);
  int r = 0;
  va_list args;
  va_list args_copy;

  va_start(args, dummy);
  va_copy(args_copy, args);
  while ((r = vsnprintf(buffer.data(), buffer.size(), format.c_str(), args_copy)) >= 0 &&
         static_cast<unsigned int>(r) >= buffer.size()) {
    buffer.resize(r + 1);
    va_end(args_copy);
    va_copy(args_copy, args);
  }
  va_end(args);
  va_end(args_copy);

  return std::string(buffer.data());
}

static std::chrono::system_clock::time_point msec_start = std::chrono::system_clock::now();
int64_t Utils::get_current_msec() {
  auto now = std::chrono::system_clock::now();
  return std::chrono::duration_cast<std::chrono::milliseconds>(now - msec_start).count();
}

/**
 * Get the last component of a pathname.
 * If suffix is matched to last of the pathname, remove it from return value.
 * @param path Pathname.
 * @param cutoff_ext Cut off the extension from basename when the basename have it if option is true.
 * @return The last component of a pathname.
 */
std::string Utils::file_basename(const std::string& path, bool cutoff_ext) {
  std::string basename;
  std::string::size_type pos = path.find_last_of('/');

  if (pos == std::string::npos) {
    basename = path;
  } else {
    basename = path.substr(pos + 1);
  }

  if (cutoff_ext) {
    pos = basename.find_last_of('.');
    if (pos == std::string::npos) {
      return basename;
    } else {
      return basename.substr(0, pos);
    }

  } else {
    return basename;
  }
}

bool Utils::is_safe_value(double v) {
  if (
#ifdef _MSC_VER
      !_finite(v)
#elif __cplusplus >= 201103L || !(defined(isnan) && defined(isinf))
      std::isnan(v) || std::isinf(v)
#else
      isnan(v) || isinf(v)
#endif
  ) {
    return false;
  } else {
    return true;
  }
}

double Utils::float_mod(double a, double b) {
  assert(b > 0);
  return a - std::floor(a / b) * b;
}
}  // namespace colonio
