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
#include <emscripten.h>

#include <map>
#include <string>

#include "colonio/colonio.h"

extern "C" {
typedef unsigned long COLONIO_PTR_T;
typedef unsigned long COLONIO_ID_T;

extern void js_on_output_log(COLONIO_PTR_T colonio_ptr, int level, COLONIO_PTR_T message_ptr, int message_siz);
extern void js_on_debug_event(COLONIO_PTR_T colonio_ptr, int level, COLONIO_PTR_T message_ptr, int message_siz);

EMSCRIPTEN_KEEPALIVE COLONIO_PTR_T js_init(COLONIO_PTR_T on_require_invoke);
EMSCRIPTEN_KEEPALIVE void js_connect(
    COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T url, COLONIO_PTR_T token, COLONIO_PTR_T on_success,
    COLONIO_PTR_T on_failure);
EMSCRIPTEN_KEEPALIVE COLONIO_PTR_T js_access_map(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T name);
EMSCRIPTEN_KEEPALIVE COLONIO_PTR_T js_access_pubsub2d(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T name);
EMSCRIPTEN_KEEPALIVE void js_disconnect(COLONIO_PTR_T colonio_ptr);
EMSCRIPTEN_KEEPALIVE void js_enable_output_log(COLONIO_PTR_T colonio_ptr);
EMSCRIPTEN_KEEPALIVE void js_enable_debug_event(COLONIO_PTR_T colonio_ptr);
EMSCRIPTEN_KEEPALIVE void js_get_my_nid(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T nid_ptr);
EMSCRIPTEN_KEEPALIVE void js_set_position(COLONIO_PTR_T colonio_ptr, double x, double y);
EMSCRIPTEN_KEEPALIVE unsigned int js_invoke(COLONIO_PTR_T colonio_ptr);

EMSCRIPTEN_KEEPALIVE COLONIO_PTR_T js_value_init();
EMSCRIPTEN_KEEPALIVE int js_value_get_type(COLONIO_PTR_T value_ptr);
EMSCRIPTEN_KEEPALIVE bool js_value_get_bool(COLONIO_PTR_T value_ptr);
EMSCRIPTEN_KEEPALIVE int64_t js_value_get_int(COLONIO_PTR_T value_ptr);
EMSCRIPTEN_KEEPALIVE double js_value_get_double(COLONIO_PTR_T value_ptr);
EMSCRIPTEN_KEEPALIVE int js_value_get_string_length(COLONIO_PTR_T value_ptr);
EMSCRIPTEN_KEEPALIVE COLONIO_PTR_T js_value_get_string(COLONIO_PTR_T value_ptr);
EMSCRIPTEN_KEEPALIVE void js_value_set_bool(COLONIO_PTR_T value_ptr, bool v);
EMSCRIPTEN_KEEPALIVE void js_value_set_int(COLONIO_PTR_T value_ptr, int64_t v);
EMSCRIPTEN_KEEPALIVE void js_value_set_double(COLONIO_PTR_T value_ptr, double v);
EMSCRIPTEN_KEEPALIVE void js_value_set_string(COLONIO_PTR_T value_ptr, COLONIO_PTR_T ptr, unsigned int len);
EMSCRIPTEN_KEEPALIVE void js_value_free(COLONIO_PTR_T value_ptr);

EMSCRIPTEN_KEEPALIVE void js_map_init(COLONIO_PTR_T on_map_get_ptr, COLONIO_PTR_T on_map_set_ptr);
EMSCRIPTEN_KEEPALIVE void js_map_get_value(COLONIO_PTR_T map_ptr, COLONIO_PTR_T key_ptr, COLONIO_ID_T id);
EMSCRIPTEN_KEEPALIVE void
js_map_set_value(COLONIO_PTR_T map_ptr, COLONIO_PTR_T key_ptr, COLONIO_PTR_T val_ptr, COLONIO_ID_T id, int opt);

EMSCRIPTEN_KEEPALIVE void js_pubsub2d_init(COLONIO_PTR_T on_pubsub2d_pub_ptr, COLONIO_PTR_T on_pubsub2d_on_ptr);
EMSCRIPTEN_KEEPALIVE void js_pubsub2d_publish(
    COLONIO_PTR_T pubsub2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz, double x, double y, double r,
    COLONIO_PTR_T val_ptr, COLONIO_ID_T id);
EMSCRIPTEN_KEEPALIVE void
js_pubsub2d_on(COLONIO_PTR_T pubsub2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz, COLONIO_ID_T id);
EMSCRIPTEN_KEEPALIVE void js_pubsub2d_off(COLONIO_PTR_T pubsub2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz);
}

static std::map<std::string, colonio_map_t> map_cache;
static std::map<std::string, colonio_pubsub2d_t> pubsub2d_cache;

COLONIO_PTR_T js_init(COLONIO_PTR_T on_require_invoke) {
  colonio_t* colonio = new colonio_t();

  colonio_init(colonio, reinterpret_cast<void (*)(colonio_t*, unsigned int)>(on_require_invoke));

  return reinterpret_cast<COLONIO_PTR_T>(colonio);
}

void js_connect(
    COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T url, COLONIO_PTR_T token, COLONIO_PTR_T on_success,
    COLONIO_PTR_T on_failure) {
  colonio_t* colonio = reinterpret_cast<colonio_t*>(colonio_ptr);

  colonio_connect(
      reinterpret_cast<colonio_t*>(colonio_ptr), reinterpret_cast<const char*>(url),
      reinterpret_cast<const char*>(token), reinterpret_cast<void (*)(colonio_t*)>(on_success),
      reinterpret_cast<void (*)(colonio_t*)>(on_failure));
}

COLONIO_PTR_T js_access_map(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T name) {
  std::string name_str(reinterpret_cast<const char*>(name));

  if (map_cache.find(name_str) == map_cache.end()) {
    colonio_map_t handler =
        colonio_access_map(reinterpret_cast<colonio_t*>(colonio_ptr), reinterpret_cast<const char*>(name));
    map_cache.insert(std::make_pair(name_str, handler));
  }
  return reinterpret_cast<COLONIO_PTR_T>(&map_cache.at(name_str));
}

COLONIO_PTR_T js_access_pubsub2d(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T name) {
  std::string name_str(reinterpret_cast<const char*>(name));

  if (pubsub2d_cache.find(name_str) == pubsub2d_cache.end()) {
    colonio_pubsub2d_t handler =
        colonio_access_pubsub2d(reinterpret_cast<colonio_t*>(colonio_ptr), reinterpret_cast<const char*>(name));
    pubsub2d_cache.insert(std::make_pair(name_str, handler));
  }
  return reinterpret_cast<COLONIO_PTR_T>(&pubsub2d_cache.at(name_str));
}

void js_disconnect(COLONIO_PTR_T colonio_ptr) {
  colonio_disconnect(reinterpret_cast<colonio_t*>(colonio_ptr));
  delete reinterpret_cast<colonio_t*>(colonio_ptr);
}

void wrap_on_output_log(
    colonio_t* colonio_ptr, COLONIO_LOG_LEVEL level, const char* message_ptr, unsigned int message_siz) {
  js_on_output_log(
      reinterpret_cast<COLONIO_PTR_T>(colonio_ptr), static_cast<int>(level),
      reinterpret_cast<COLONIO_PTR_T>(message_ptr), message_siz - 1);
}

void js_enable_output_log(COLONIO_PTR_T colonio_ptr) {
  colonio_t* colonio = reinterpret_cast<colonio_t*>(colonio_ptr);

  colonio_set_on_output_log(colonio, wrap_on_output_log);
}

void wrap_on_debug_event(
    colonio_t* colonio_ptr, COLONIO_DEBUG_EVENT event, const char* message_ptr, unsigned int message_siz) {
  js_on_debug_event(
      reinterpret_cast<COLONIO_PTR_T>(colonio_ptr), static_cast<int>(event),
      reinterpret_cast<COLONIO_PTR_T>(message_ptr), message_siz - 1);
}

void js_enable_debug_event(COLONIO_PTR_T colonio_ptr) {
  colonio_t* colonio = reinterpret_cast<colonio_t*>(colonio_ptr);

  colonio_set_on_debug_event(colonio, wrap_on_debug_event);
}

void js_get_my_nid(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T nid_ptr) {
  colonio_get_my_nid(reinterpret_cast<colonio_t*>(colonio_ptr), reinterpret_cast<char*>(nid_ptr));
}

void js_set_position(COLONIO_PTR_T colonio_ptr, double x, double y) {
  colonio_set_position(reinterpret_cast<colonio_t*>(colonio_ptr), x, y);
}

unsigned int js_invoke(COLONIO_PTR_T colonio_ptr) {
  colonio_t* colonio = reinterpret_cast<colonio_t*>(colonio_ptr);
  return colonio_invoke(colonio);
}

COLONIO_PTR_T js_value_init() {
  colonio_value_t* value = new colonio_value_t();
  colonio_value_init(value);
  return reinterpret_cast<COLONIO_PTR_T>(value);
}

int js_value_get_type(COLONIO_PTR_T value_ptr) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  return static_cast<int>(colonio_value_get_type(value));
}

bool js_value_get_bool(COLONIO_PTR_T value_ptr) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  return colonio_value_get_bool(value);
}

int64_t js_value_get_int(COLONIO_PTR_T value_ptr) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  return colonio_value_get_int(value);
}

double js_value_get_double(COLONIO_PTR_T value_ptr) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  return colonio_value_get_double(value);
}

int js_value_get_string_length(COLONIO_PTR_T value_ptr) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  return value->value.string_v.len;
}

COLONIO_PTR_T js_value_get_string(COLONIO_PTR_T value_ptr) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  return reinterpret_cast<COLONIO_PTR_T>(value->value.string_v.str);
}

void js_value_set_bool(COLONIO_PTR_T value_ptr, bool v) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  colonio_value_set_bool(value, v);
}

void js_value_set_int(COLONIO_PTR_T value_ptr, int64_t v) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  colonio_value_set_int(value, v);
}

void js_value_set_double(COLONIO_PTR_T value_ptr, double v) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  colonio_value_set_double(value, v);
}

void js_value_set_string(COLONIO_PTR_T value_ptr, COLONIO_PTR_T ptr, unsigned int len) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  colonio_value_set_string(value, reinterpret_cast<const char*>(ptr), len);
}

void js_value_free(COLONIO_PTR_T value_ptr) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  colonio_value_free(value);
  delete value;
}

std::function<void(COLONIO_ID_T, int, COLONIO_PTR_T)> on_map_get;
std::function<void(COLONIO_ID_T, int)> on_map_set;

void js_map_init(COLONIO_PTR_T on_map_get_ptr, COLONIO_PTR_T on_map_set_ptr) {
  on_map_get = reinterpret_cast<void (*)(COLONIO_ID_T, int, COLONIO_PTR_T)>(on_map_get_ptr);
  on_map_set = reinterpret_cast<void (*)(COLONIO_ID_T, int)>(on_map_set_ptr);
}

void wrap_map_get_value_success(colonio_map_t* map, void* ptr, const colonio_value_t* v) {
  on_map_get(reinterpret_cast<COLONIO_ID_T>(ptr), COLONIO_MAP_FAILURE_REASON_NONE, reinterpret_cast<COLONIO_PTR_T>(v));
}

void wrap_map_get_value_failure(colonio_map_t* map, void* ptr, COLONIO_MAP_FAILURE_REASON reason) {
  on_map_get(reinterpret_cast<COLONIO_ID_T>(ptr), reason, reinterpret_cast<COLONIO_ID_T>(nullptr));
}

void js_map_get_value(COLONIO_PTR_T map_ptr, COLONIO_PTR_T key_ptr, COLONIO_ID_T id) {
  colonio_map_t* map   = reinterpret_cast<colonio_map_t*>(map_ptr);
  colonio_value_t* key = reinterpret_cast<colonio_value_t*>(key_ptr);

  colonio_map_get(map, key, reinterpret_cast<void*>(id), wrap_map_get_value_success, wrap_map_get_value_failure);
}

void wrap_map_set_value_success(colonio_map_t* map, void* ptr) {
  on_map_set(reinterpret_cast<COLONIO_ID_T>(ptr), COLONIO_MAP_FAILURE_REASON_NONE);
}

void wrap_map_set_value_failure(colonio_map_t* map, void* ptr, COLONIO_MAP_FAILURE_REASON reason) {
  on_map_set(reinterpret_cast<COLONIO_ID_T>(ptr), reason);
}

void js_map_set_value(COLONIO_PTR_T map_ptr, COLONIO_PTR_T key_ptr, COLONIO_PTR_T val_ptr, COLONIO_ID_T id, int opt) {
  colonio_map_t* map   = reinterpret_cast<colonio_map_t*>(map_ptr);
  colonio_value_t* key = reinterpret_cast<colonio_value_t*>(key_ptr);
  colonio_value_t* val = reinterpret_cast<colonio_value_t*>(val_ptr);

  colonio_map_set(
      map, key, val, reinterpret_cast<void*>(id), wrap_map_set_value_success, wrap_map_set_value_failure, opt);
}

std::function<void(COLONIO_ID_T, int)> on_pubsub2d_pub;
std::function<void(COLONIO_ID_T, COLONIO_PTR_T)> on_pubsub2d_on;

void js_pubsub2d_init(COLONIO_PTR_T on_pubsub2d_pub_ptr, COLONIO_PTR_T on_pubsub2d_on_ptr) {
  on_pubsub2d_pub = reinterpret_cast<void (*)(COLONIO_ID_T, int)>(on_pubsub2d_pub_ptr);
  on_pubsub2d_on  = reinterpret_cast<void (*)(COLONIO_ID_T, COLONIO_PTR_T)>(on_pubsub2d_on_ptr);
}

void wrap_pubsub2d_publish_success(colonio_pubsub2d_t* pubsub2d, void* ptr) {
  on_pubsub2d_pub(reinterpret_cast<COLONIO_ID_T>(ptr), COLONIO_PUBSUB2D_FAILURE_REASON_NONE);
}

void wrap_pubsub2d_publish_failure(colonio_pubsub2d_t* pubsub2d, void* ptr, COLONIO_PUBSUB2D_FAILURE_REASON reason) {
  on_pubsub2d_pub(reinterpret_cast<COLONIO_ID_T>(ptr), reason);
}

void js_pubsub2d_publish(
    COLONIO_PTR_T pubsub2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz, double x, double y, double r,
    COLONIO_PTR_T val_ptr, COLONIO_ID_T id) {
  colonio_pubsub2d_publish(
      reinterpret_cast<colonio_pubsub2d_t*>(pubsub2d_ptr), reinterpret_cast<char*>(name_ptr), name_siz, x, y, r,
      reinterpret_cast<colonio_value_t*>(val_ptr), reinterpret_cast<void*>(id), wrap_pubsub2d_publish_success,
      wrap_pubsub2d_publish_failure);
}

void wrap_pubsub2d_on_subscriber(colonio_pubsub2d_t* pubsub2d, void* ptr, const colonio_value_t* value) {
  on_pubsub2d_on(reinterpret_cast<COLONIO_ID_T>(ptr), reinterpret_cast<COLONIO_PTR_T>(value));
}

void js_pubsub2d_on(COLONIO_PTR_T pubsub2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz, COLONIO_ID_T id) {
  colonio_pubsub2d_on(
      reinterpret_cast<colonio_pubsub2d_t*>(pubsub2d_ptr), reinterpret_cast<char*>(name_ptr), name_siz,
      reinterpret_cast<void*>(id), wrap_pubsub2d_on_subscriber);
}

void js_pubsub2d_off(COLONIO_PTR_T pubsub2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz) {
  colonio_pubsub2d_off(
      reinterpret_cast<colonio_pubsub2d_t*>(pubsub2d_ptr), reinterpret_cast<char*>(name_ptr), name_siz);
}
