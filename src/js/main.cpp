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
#include <emscripten.h>

#include <map>
#include <string>

extern "C" {
#include "colonio/colonio.h"

typedef unsigned long COLONIO_PTR_T;
typedef unsigned long COLONIO_ID_T;

extern void js_on_output_log(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T json_ptr, int json_siz);

EMSCRIPTEN_KEEPALIVE unsigned int js_error_get_code(COLONIO_PTR_T err);
EMSCRIPTEN_KEEPALIVE COLONIO_PTR_T js_error_get_message(COLONIO_PTR_T err);
EMSCRIPTEN_KEEPALIVE int js_error_get_message_length(COLONIO_PTR_T err);

EMSCRIPTEN_KEEPALIVE COLONIO_PTR_T
js_init(COLONIO_PTR_T set_position_on_success_, COLONIO_PTR_T set_position_on_failure_);
EMSCRIPTEN_KEEPALIVE void js_connect(
    COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T url, unsigned int url_siz, COLONIO_PTR_T token, unsigned int token_siz,
    COLONIO_PTR_T on_success, COLONIO_PTR_T on_failure);
EMSCRIPTEN_KEEPALIVE COLONIO_PTR_T js_access_map(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T name, unsigned int name_siz);
EMSCRIPTEN_KEEPALIVE COLONIO_PTR_T
js_access_pubsub_2d(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T name, unsigned int name_siz);
EMSCRIPTEN_KEEPALIVE void js_disconnect(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T on_success, COLONIO_PTR_T on_failure);
EMSCRIPTEN_KEEPALIVE void js_enable_output_log(COLONIO_PTR_T colonio_ptr);
EMSCRIPTEN_KEEPALIVE void js_get_local_nid(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T nid_ptr);
EMSCRIPTEN_KEEPALIVE void js_set_position(COLONIO_PTR_T colonio_ptr, double x, double y, COLONIO_ID_T id);
EMSCRIPTEN_KEEPALIVE unsigned int js_invoke(COLONIO_PTR_T colonio_ptr);
EMSCRIPTEN_KEEPALIVE void js_quit(COLONIO_PTR_T colonio_ptr);

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
EMSCRIPTEN_KEEPALIVE void js_value_set_string(COLONIO_PTR_T value_ptr, COLONIO_PTR_T ptr, unsigned int siz);
EMSCRIPTEN_KEEPALIVE void js_value_free(COLONIO_PTR_T value_ptr);

EMSCRIPTEN_KEEPALIVE void js_map_init(
    COLONIO_PTR_T get_on_success, COLONIO_PTR_T get_on_failure, COLONIO_PTR_T set_on_success,
    COLONIO_PTR_T set_on_failure);
EMSCRIPTEN_KEEPALIVE void js_map_get_value(COLONIO_PTR_T map_ptr, COLONIO_PTR_T key_ptr, COLONIO_ID_T id);
EMSCRIPTEN_KEEPALIVE void js_map_set_value(
    COLONIO_PTR_T map_ptr, COLONIO_PTR_T key_ptr, COLONIO_PTR_T val_ptr, uint32_t opt, COLONIO_ID_T id);

EMSCRIPTEN_KEEPALIVE void js_pubsub_2d_init(
    COLONIO_PTR_T publish_on_success, COLONIO_PTR_T publish_on_failure, COLONIO_PTR_T on);
EMSCRIPTEN_KEEPALIVE void js_pubsub_2d_publish(
    COLONIO_PTR_T pubsub_2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz, double x, double y, double r,
    COLONIO_PTR_T val_ptr, uint32_t opt, COLONIO_ID_T id);
EMSCRIPTEN_KEEPALIVE void js_pubsub_2d_on(
    COLONIO_PTR_T pubsub_2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz, COLONIO_ID_T id);
EMSCRIPTEN_KEEPALIVE void js_pubsub_2d_off(COLONIO_PTR_T pubsub_2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz);
}

struct Cache {
  std::map<std::string, colonio_map_t> map;
  std::map<std::string, colonio_pubsub_2d_t> pubsub_2d;
};

static std::map<COLONIO_PTR_T, Cache> caches;

std::function<void(COLONIO_ID_T, double, double)> set_position_on_success;
std::function<void(COLONIO_ID_T, COLONIO_PTR_T)> set_position_on_failure;

unsigned int js_error_get_code(COLONIO_PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return e->code;
}

COLONIO_PTR_T js_error_get_message(COLONIO_PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return reinterpret_cast<COLONIO_PTR_T>(e->message);
}

int js_error_get_message_length(COLONIO_PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return e->message_siz;
}

COLONIO_PTR_T js_init(COLONIO_PTR_T set_position_on_success_, COLONIO_PTR_T set_position_on_failure_) {
  colonio_t* colonio = new colonio_t();

  // TODO detect an error
  colonio_init(colonio, 0);

  set_position_on_success = reinterpret_cast<void (*)(COLONIO_ID_T, double, double)>(set_position_on_success_);
  set_position_on_failure = reinterpret_cast<void (*)(COLONIO_ID_T, COLONIO_PTR_T)>(set_position_on_failure_);

  return reinterpret_cast<COLONIO_PTR_T>(colonio);
}

void js_connect(
    COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T url, unsigned int url_siz, COLONIO_PTR_T token, unsigned int token_siz,
    COLONIO_PTR_T on_success, COLONIO_PTR_T on_failure) {
  colonio_t* colonio = reinterpret_cast<colonio_t*>(colonio_ptr);

  colonio_connect_async(
      colonio, reinterpret_cast<const char*>(url), url_siz, reinterpret_cast<const char*>(token), token_siz,
      reinterpret_cast<void (*)(colonio_t*)>(on_success),
      reinterpret_cast<void (*)(colonio_t*, const colonio_error_t*)>(on_failure));
}

COLONIO_PTR_T js_access_map(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T name, unsigned int name_siz) {
  std::string name_str(reinterpret_cast<const char*>(name), name_siz);

  std::map<std::string, colonio_map_t>& map_cache = caches[colonio_ptr].map;

  if (map_cache.find(name_str) == map_cache.end()) {
    colonio_map_t handler =
        colonio_access_map(reinterpret_cast<colonio_t*>(colonio_ptr), reinterpret_cast<const char*>(name), name_siz);
    map_cache.insert(std::make_pair(name_str, handler));
  }
  return reinterpret_cast<COLONIO_PTR_T>(&map_cache.at(name_str));
}

COLONIO_PTR_T js_access_pubsub_2d(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T name, unsigned int name_siz) {
  std::string name_str(reinterpret_cast<const char*>(name), name_siz);

  std::map<std::string, colonio_pubsub_2d_t>& pubsub_2d_cache = caches[colonio_ptr].pubsub_2d;

  if (pubsub_2d_cache.find(name_str) == pubsub_2d_cache.end()) {
    colonio_pubsub_2d_t handler = colonio_access_pubsub_2d(
        reinterpret_cast<colonio_t*>(colonio_ptr), reinterpret_cast<const char*>(name), name_siz);
    pubsub_2d_cache.insert(std::make_pair(name_str, handler));
  }
  return reinterpret_cast<COLONIO_PTR_T>(&pubsub_2d_cache.at(name_str));
}

void js_disconnect(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T on_success, COLONIO_PTR_T on_failure) {
  colonio_disconnect_async(
      reinterpret_cast<colonio_t*>(colonio_ptr), reinterpret_cast<void (*)(colonio_t*)>(on_success),
      reinterpret_cast<void (*)(colonio_t*, const colonio_error_t*)>(on_failure));
}

void wrap_on_output_log(colonio_t* colonio_ptr, const char* json_ptr, unsigned int json_siz) {
  js_on_output_log(reinterpret_cast<COLONIO_PTR_T>(colonio_ptr), reinterpret_cast<COLONIO_PTR_T>(json_ptr), json_siz);
}

void js_enable_output_log(COLONIO_PTR_T colonio_ptr) {
  colonio_t* colonio = reinterpret_cast<colonio_t*>(colonio_ptr);

  colonio_set_on_output_log(colonio, wrap_on_output_log);
}

void js_get_local_nid(COLONIO_PTR_T colonio_ptr, COLONIO_PTR_T nid_ptr) {
  colonio_get_local_nid(reinterpret_cast<colonio_t*>(colonio_ptr), reinterpret_cast<char*>(nid_ptr), nullptr);
}

void wrap_set_position_on_success(colonio_t*, void* ptr, double newX, double newY) {
  set_position_on_success(reinterpret_cast<COLONIO_ID_T>(ptr), newX, newY);
}

void wrap_set_position_on_failure(colonio_t*, void* ptr, const colonio_error_t* errorPtr) {
  set_position_on_failure(reinterpret_cast<COLONIO_ID_T>(ptr), reinterpret_cast<COLONIO_PTR_T>(errorPtr));
}

void js_set_position(COLONIO_PTR_T colonio_ptr, double x, double y, COLONIO_ID_T id) {
  colonio_set_position_async(
      reinterpret_cast<colonio_t*>(colonio_ptr), x, y, reinterpret_cast<void*>(id), wrap_set_position_on_success,
      wrap_set_position_on_failure);
}

void js_quit(COLONIO_PTR_T colonio_ptr) {
  // TODO ditect an error
  colonio_quit(reinterpret_cast<colonio_t*>(colonio_ptr));
  delete reinterpret_cast<colonio_t*>(colonio_ptr);
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
  return value->value.string_v.siz;
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

void js_value_set_string(COLONIO_PTR_T value_ptr, COLONIO_PTR_T ptr, unsigned int siz) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  colonio_value_set_string(value, reinterpret_cast<const char*>(ptr), siz);
}

void js_value_free(COLONIO_PTR_T value_ptr) {
  colonio_value_t* value = reinterpret_cast<colonio_value_t*>(value_ptr);
  colonio_value_free(value);
  delete value;
}

std::function<void(COLONIO_ID_T, COLONIO_PTR_T)> map_get_on_success;
std::function<void(COLONIO_ID_T, COLONIO_PTR_T)> map_get_on_failure;
std::function<void(COLONIO_ID_T)> map_set_on_success;
std::function<void(COLONIO_ID_T, COLONIO_PTR_T)> map_set_on_failure;

void js_map_init(
    COLONIO_PTR_T get_on_success, COLONIO_PTR_T get_on_failure, COLONIO_PTR_T set_on_success,
    COLONIO_PTR_T set_on_failure) {
  map_get_on_success = reinterpret_cast<void (*)(COLONIO_ID_T, COLONIO_PTR_T)>(get_on_success);
  map_get_on_failure = reinterpret_cast<void (*)(COLONIO_ID_T, COLONIO_PTR_T)>(get_on_failure);
  map_set_on_success = reinterpret_cast<void (*)(COLONIO_ID_T)>(set_on_success);
  map_set_on_failure = reinterpret_cast<void (*)(COLONIO_ID_T, COLONIO_PTR_T)>(set_on_failure);
}

void wrap_map_get_on_success(colonio_map_t* map, void* ptr, const colonio_value_t* valuePtr) {
  map_get_on_success(reinterpret_cast<COLONIO_ID_T>(ptr), reinterpret_cast<COLONIO_PTR_T>(valuePtr));
}

void wrap_map_get_on_failure(colonio_map_t* map, void* ptr, const colonio_error_t* errorPtr) {
  map_get_on_failure(reinterpret_cast<COLONIO_ID_T>(ptr), reinterpret_cast<COLONIO_PTR_T>(errorPtr));
}

void js_map_get_value(COLONIO_PTR_T map_ptr, COLONIO_PTR_T key_ptr, COLONIO_ID_T id) {
  colonio_map_t* map   = reinterpret_cast<colonio_map_t*>(map_ptr);
  colonio_value_t* key = reinterpret_cast<colonio_value_t*>(key_ptr);

  colonio_map_get_async(map, key, reinterpret_cast<void*>(id), wrap_map_get_on_success, wrap_map_get_on_failure);
}

void wrap_map_set_success(colonio_map_t* map, void* ptr) {
  map_set_on_success(reinterpret_cast<COLONIO_ID_T>(ptr));
}

void wrap_map_set_failure(colonio_map_t* map, void* ptr, const colonio_error_t* errorPtr) {
  map_set_on_failure(reinterpret_cast<COLONIO_ID_T>(ptr), reinterpret_cast<COLONIO_PTR_T>(errorPtr));
}

void js_map_set_value(
    COLONIO_PTR_T map_ptr, COLONIO_PTR_T key_ptr, COLONIO_PTR_T val_ptr, uint32_t opt, COLONIO_ID_T id) {
  colonio_map_t* map   = reinterpret_cast<colonio_map_t*>(map_ptr);
  colonio_value_t* key = reinterpret_cast<colonio_value_t*>(key_ptr);
  colonio_value_t* val = reinterpret_cast<colonio_value_t*>(val_ptr);

  colonio_map_set_async(map, key, val, opt, reinterpret_cast<void*>(id), wrap_map_set_success, wrap_map_set_failure);
}

std::function<void(COLONIO_ID_T)> ps2_publish_on_success;
std::function<void(COLONIO_ID_T, COLONIO_PTR_T)> ps2_publish_on_failure;
std::function<void(COLONIO_ID_T, COLONIO_PTR_T)> ps2_on;

void js_pubsub_2d_init(COLONIO_PTR_T publish_on_success, COLONIO_PTR_T publish_on_failure, COLONIO_PTR_T on) {
  ps2_publish_on_success = reinterpret_cast<void (*)(COLONIO_ID_T)>(publish_on_success);
  ps2_publish_on_failure = reinterpret_cast<void (*)(COLONIO_ID_T, COLONIO_PTR_T)>(publish_on_failure);
  ps2_on                 = reinterpret_cast<void (*)(COLONIO_ID_T, COLONIO_PTR_T)>(on);
}

void wrap_ps2_publish_on_success(colonio_pubsub_2d_t* pubsub_2d, void* ptr) {
  ps2_publish_on_success(reinterpret_cast<COLONIO_ID_T>(ptr));
}

void wrap_ps2_publish_on_failure(colonio_pubsub_2d_t* pubsub_2d, void* ptr, const colonio_error_t* errorPtr) {
  ps2_publish_on_failure(reinterpret_cast<COLONIO_ID_T>(ptr), reinterpret_cast<COLONIO_PTR_T>(errorPtr));
}

void js_pubsub_2d_publish(
    COLONIO_PTR_T pubsub_2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz, double x, double y, double r,
    COLONIO_PTR_T val_ptr, uint32_t opt, COLONIO_ID_T id) {
  colonio_pubsub_2d_publish_async(
      reinterpret_cast<colonio_pubsub_2d_t*>(pubsub_2d_ptr), reinterpret_cast<char*>(name_ptr), name_siz, x, y, r,
      reinterpret_cast<colonio_value_t*>(val_ptr), opt, reinterpret_cast<void*>(id), wrap_ps2_publish_on_success,
      wrap_ps2_publish_on_failure);
}

void wrap_ps2_on(colonio_pubsub_2d_t* pubsub_2d, void* ptr, const colonio_value_t* value) {
  ps2_on(reinterpret_cast<COLONIO_ID_T>(ptr), reinterpret_cast<COLONIO_PTR_T>(value));
}

void js_pubsub_2d_on(COLONIO_PTR_T pubsub_2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz, COLONIO_ID_T id) {
  colonio_pubsub_2d_on(
      reinterpret_cast<colonio_pubsub_2d_t*>(pubsub_2d_ptr), reinterpret_cast<char*>(name_ptr), name_siz,
      reinterpret_cast<void*>(id), wrap_ps2_on);
}

void js_pubsub_2d_off(COLONIO_PTR_T pubsub_2d_ptr, COLONIO_PTR_T name_ptr, unsigned int name_siz) {
  colonio_pubsub_2d_off(
      reinterpret_cast<colonio_pubsub_2d_t*>(pubsub_2d_ptr), reinterpret_cast<char*>(name_ptr), name_siz);
}
