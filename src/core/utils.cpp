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
#include <libgen.h>
#if defined(__APPLE__) && defined(__MACH__)
#  include <mach-o/dyld.h>
#endif
#ifdef __linux__
#  include <unistd.h>
#endif
#include <sys/param.h>

#include <cassert>
#include <cstdarg>
#include <cstring>
#include <iomanip>
#include <memory>
#include <mutex>
#include <random>
#include <sstream>
#include <string>
#include <thread>

#include "convert.hpp"
#include "definition.hpp"
#include "packet.hpp"
#include "utils.hpp"

namespace colonio {
// Random value generator.
static std::random_device seed_gen;
static std::mt19937 rnd32(seed_gen());
static std::mt19937_64 rnd64(seed_gen());
static std::mutex mutex32;
static std::mutex mutex64;

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

std::string Utils::dump_binary(const std::string& bin) {
  if (&bin == nullptr) {
    return "null";

  } else {
    std::stringstream out;

    out << std::hex;
    for (int idx = 0; idx < bin.size(); idx++) {
      if (idx != 0) {
        out << " ";
      }
      out << std::setw(2) << std::setfill('0') << (0xFF & bin[idx]);
    }

    return out.str();
  }
}

std::string Utils::dump_packet(const Packet& packet, unsigned int indent) {
  std::string is;
  std::stringstream out;

  for (int i = 0; i < indent; i++) {
    is += " ";
  }

  out << is << "dst_nid : " << packet.dst_nid.to_str() << std::endl;
  out << is << "src_nid : " << packet.src_nid.to_str() << std::endl;
  out << is << "id : " << Convert::int2str(packet.id) << std::endl;
  out << is << "mode : " << Convert::int2str(packet.mode) << std::endl;
  out << is << "channel : " << Convert::int2str(packet.channel) << std::endl;
  out << is << "module_channel : " << Convert::int2str(packet.module_channel) << std::endl;
  out << is << "command_id : " << Convert::int2str(packet.command_id) << std::endl;
  out << is << "content : " << dump_binary(*packet.content);

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

std::string Utils::get_current_thread_id() {
  std::stringstream ss;
  ss << std::this_thread::get_id();
  return ss.str();
}

/**
 * Get the last component of a pathname.
 * If suffix is matched to last of the pathname, remove it from return value.
 * @param path Pathname.
 * @param cutoff_ext Cut off the extension from basename if the basename have a externsion and option is true.
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

/**
 * Split path to dirname and basename, and get dirname.
 * @param path A target path to split.
 * @return A string of dirname.
 */
std::string Utils::file_dirname(const std::string& path) {
  std::unique_ptr<char[]> buffer = std::make_unique<char[]>(path.size() + 1);

  memcpy(buffer.get(), path.c_str(), path.size());
  buffer[path.size()] = '\0';

  return std::string(dirname(buffer.get()));
}

uint32_t Utils::get_rnd_32() {
  std::lock_guard<std::mutex> guard(mutex32);
  return rnd32();
}

uint64_t Utils::get_rnd_64() {
  std::lock_guard<std::mutex> guard(mutex64);
  return rnd64();
}

bool Utils::is_safevalue(double v) {
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

void Utils::output_assert(
    const std::string& func, const std::string& file, unsigned long line, const std::string& exp,
    const std::string& mesg) {
  printf(
      "Assersion failed: (%s) func: %s, file: %s, line: %ld\n%s\n", exp.c_str(), func.c_str(), file.c_str(), line,
      mesg.c_str());
  exit(-1);
}

double Utils::pmod(double a, double b) {
  assert(b > 0);
  return a - std::floor(a / b) * b;
}

/**
 * Replace all the string in a string.
 * @param str Target string.
 * @param from String befor replace.
 * @param to String after replace.
 */
void Utils::replace_string(std::string* str, const std::string& from, const std::string& to) {
  std::string::size_type pos = str->find(from);
  while (pos != std::string::npos) {
    str->replace(pos, from.size(), to);
    pos = str->find(from, pos + to.size());
  }
}
}  // namespace colonio
