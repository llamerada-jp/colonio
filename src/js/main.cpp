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
#include <emscripten.h>

#include <functional>
#include <iostream>
#include <map>
#include <string>
#include <vector>

extern "C" {
#include "colonio/colonio.h"

typedef unsigned long PTR_T;
typedef unsigned long COLONIO_T;
typedef unsigned long VALUE_T;
typedef unsigned long LOCAL_DATA_T;
typedef unsigned long ID_T;

enum FuncID {
  LOGGER,
  MESSAGING_POST_ON_SUCCESS,
  MESSAGING_POST_ON_FAILURE,
  MESSAGING_HANDLER,
  KVS_GET_ON_SUCCESS,
  KVS_GET_ON_FAILURE,
  KVS_SET_ON_SUCCESS,
  KVS_SET_ON_FAILURE,
  KVS_LOCAL_DATA_HANDLER,
  SPREAD_POST_ON_SUCCESS,
  SPREAD_POST_ON_FAILURE,
  SPREAD_HANDLER,
};
EMSCRIPTEN_KEEPALIVE void js_bind_function(FuncID id, PTR_T ptr);

// error
EMSCRIPTEN_KEEPALIVE bool js_error_get_fatal(PTR_T err);
EMSCRIPTEN_KEEPALIVE unsigned int js_error_get_code(PTR_T err);
EMSCRIPTEN_KEEPALIVE PTR_T js_error_get_message(PTR_T err);
EMSCRIPTEN_KEEPALIVE int js_error_get_message_length(PTR_T err);
EMSCRIPTEN_KEEPALIVE bool js_error_get_line(PTR_T err);
EMSCRIPTEN_KEEPALIVE PTR_T js_error_get_file(PTR_T err);
EMSCRIPTEN_KEEPALIVE int js_error_get_file_length(PTR_T err);
// colonio
EMSCRIPTEN_KEEPALIVE COLONIO_T js_init(unsigned int seed_session_timeout_ms, bool disable_seed_verification);
EMSCRIPTEN_KEEPALIVE void js_connect(
    COLONIO_T c, PTR_T url, unsigned int url_siz, PTR_T token, unsigned int token_siz, PTR_T on_success,
    PTR_T on_failure);
EMSCRIPTEN_KEEPALIVE void js_disconnect(COLONIO_T c, PTR_T on_success, PTR_T on_failure);
EMSCRIPTEN_KEEPALIVE bool js_is_connected(COLONIO_T c);
EMSCRIPTEN_KEEPALIVE void js_get_local_nid(COLONIO_T c, PTR_T nid);
EMSCRIPTEN_KEEPALIVE PTR_T js_set_position(COLONIO_T c, double x, double y);
EMSCRIPTEN_KEEPALIVE void js_quit(COLONIO_T c);
// messaging
EMSCRIPTEN_KEEPALIVE void js_messaging_post(
    COLONIO_T c, PTR_T dst, PTR_T name, unsigned int name_siz, VALUE_T value, uint32_t opt, ID_T id);
EMSCRIPTEN_KEEPALIVE void js_messaging_set_handler(COLONIO_T c, PTR_T name, unsigned int name_siz, ID_T id);
EMSCRIPTEN_KEEPALIVE void js_messaging_response_writer(COLONIO_T c, PTR_T w, VALUE_T message);
EMSCRIPTEN_KEEPALIVE void js_messaging_unset_handler(COLONIO_T c, PTR_T name, unsigned int name_siz);
// kvs
EMSCRIPTEN_KEEPALIVE void js_kvs_get_local_data(COLONIO_T c, ID_T id);
EMSCRIPTEN_KEEPALIVE VALUE_T js_kvs_local_data_get_value(COLONIO_T c, LOCAL_DATA_T cur, unsigned int idx);
EMSCRIPTEN_KEEPALIVE void js_kvs_local_data_free(COLONIO_T c, LOCAL_DATA_T cur);
EMSCRIPTEN_KEEPALIVE void js_kvs_get(COLONIO_T c, PTR_T key, unsigned int key_siz, ID_T id);
EMSCRIPTEN_KEEPALIVE void js_kvs_set(
    COLONIO_T c, PTR_T key, unsigned int key_siz, VALUE_T value, uint32_t opt, ID_T id);
// spread
EMSCRIPTEN_KEEPALIVE void js_spread_post(
    COLONIO_T c, double x, double y, double r, PTR_T name, unsigned int name_siz, VALUE_T value, uint32_t opt, ID_T id);
EMSCRIPTEN_KEEPALIVE void js_spread_set_handler(COLONIO_T c, PTR_T name, unsigned int name_siz, ID_T id);
EMSCRIPTEN_KEEPALIVE void js_spread_unset_handler(COLONIO_T c, PTR_T name, unsigned int name_siz);
// value
EMSCRIPTEN_KEEPALIVE VALUE_T js_value_create();
EMSCRIPTEN_KEEPALIVE int js_value_get_type(VALUE_T v);
EMSCRIPTEN_KEEPALIVE bool js_value_get_bool(VALUE_T v);
EMSCRIPTEN_KEEPALIVE int64_t js_value_get_int(VALUE_T v);
EMSCRIPTEN_KEEPALIVE double js_value_get_double(VALUE_T v);
EMSCRIPTEN_KEEPALIVE int js_value_get_string_length(VALUE_T v);
EMSCRIPTEN_KEEPALIVE PTR_T js_value_get_string(VALUE_T v);
EMSCRIPTEN_KEEPALIVE int js_value_get_binary_length(VALUE_T v);
EMSCRIPTEN_KEEPALIVE PTR_T js_value_get_binary(VALUE_T v);
EMSCRIPTEN_KEEPALIVE void js_value_set_bool(VALUE_T v, bool b);
EMSCRIPTEN_KEEPALIVE void js_value_set_int(VALUE_T v, int64_t i);
EMSCRIPTEN_KEEPALIVE void js_value_set_double(VALUE_T v, double d);
EMSCRIPTEN_KEEPALIVE void js_value_set_string(VALUE_T v, PTR_T ptr, unsigned int siz);
EMSCRIPTEN_KEEPALIVE void js_value_set_binary(VALUE_T v, PTR_T ptr, unsigned int siz);
EMSCRIPTEN_KEEPALIVE void js_value_free(VALUE_T v);
}

// bind js functions
void (*logger_func)(colonio_t, const char*, unsigned int)                            = nullptr;
void (*messaging_post_on_success)(colonio_t, void*, colonio_const_value_t)           = nullptr;
void (*messaging_post_on_failure)(colonio_t, void*, const colonio_error_t*)          = nullptr;
void (*messaging_handler)(colonio_t, ID_T, PTR_T, VALUE_T, uint32_t, PTR_T)          = nullptr;
void (*kvs_get_on_success)(colonio_t, void*, colonio_const_value_t)                  = nullptr;
void (*kvs_get_on_failure)(colonio_t, void*, const colonio_error_t*)                 = nullptr;
void (*kvs_set_on_success)(colonio_t, void*)                                         = nullptr;
void (*kvs_set_on_failure)(colonio_t, void*, const colonio_error_t*)                 = nullptr;
void (*kvs_local_data_handler)(colonio_t, ID_T, colonio_kvs_data_t, PTR_T, uint32_t) = nullptr;
void (*spread_post_on_success)(colonio_t, void*)                                     = nullptr;
void (*spread_post_on_failure)(colonio_t, void*, const colonio_error_t*)             = nullptr;
void (*spread_handler)(colonio_t, ID_T, PTR_T, VALUE_T, uint32_t)                    = nullptr;

void js_bind_function(FuncID id, PTR_T func) {
  switch (id) {
    case LOGGER: {
      logger_func = reinterpret_cast<void (*)(colonio_t, const char*, unsigned int)>(func);
    } break;

    case MESSAGING_POST_ON_SUCCESS: {
      messaging_post_on_success = reinterpret_cast<void (*)(colonio_t, void*, colonio_const_value_t)>(func);
    } break;

    case MESSAGING_POST_ON_FAILURE: {
      messaging_post_on_failure = reinterpret_cast<void (*)(colonio_t, void*, const colonio_error_t*)>(func);
    } break;

    case MESSAGING_HANDLER: {
      messaging_handler = reinterpret_cast<void (*)(colonio_t, ID_T, PTR_T, VALUE_T, uint32_t, PTR_T)>(func);
    } break;

    case KVS_GET_ON_SUCCESS: {
      kvs_get_on_success = reinterpret_cast<void (*)(colonio_t, void*, colonio_const_value_t)>(func);
    } break;

    case KVS_GET_ON_FAILURE: {
      kvs_get_on_failure = reinterpret_cast<void (*)(colonio_t, void*, const colonio_error_t*)>(func);
    } break;

    case KVS_SET_ON_SUCCESS: {
      kvs_set_on_success = reinterpret_cast<void (*)(colonio_t, void*)>(func);
    } break;

    case KVS_SET_ON_FAILURE: {
      kvs_set_on_failure = reinterpret_cast<void (*)(colonio_t, void*, const colonio_error_t*)>(func);
    } break;

    case KVS_LOCAL_DATA_HANDLER: {
      kvs_local_data_handler = reinterpret_cast<void (*)(colonio_t, ID_T, colonio_kvs_data_t, PTR_T, uint32_t)>(func);
    } break;

    case SPREAD_POST_ON_SUCCESS: {
      spread_post_on_success = reinterpret_cast<void (*)(colonio_t, void*)>(func);
    } break;

    case SPREAD_POST_ON_FAILURE: {
      spread_post_on_failure = reinterpret_cast<void (*)(colonio_t, void*, const colonio_error_t*)>(func);
    } break;

    case SPREAD_HANDLER: {
      spread_handler = reinterpret_cast<void (*)(colonio_t, ID_T, PTR_T, VALUE_T, uint32_t)>(func);
    } break;

    default:
      break;
  }
}

bool js_error_get_fatal(PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return e->fatal;
}

unsigned int js_error_get_code(PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return e->code;
}

PTR_T js_error_get_message(PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return reinterpret_cast<PTR_T>(e->message);
}

int js_error_get_message_length(PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return e->message_siz;
}

bool js_error_get_line(PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return e->line;
}

PTR_T js_error_get_file(PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return reinterpret_cast<PTR_T>(e->file);
}

int js_error_get_file_length(PTR_T err) {
  colonio_error_t* e = reinterpret_cast<colonio_error_t*>(err);
  return e->file_siz;
}

COLONIO_T js_init(unsigned int seed_session_timeout_ms, bool disable_seed_verification) {
  colonio_config_t config;
  colonio_config_set_default(&config);
  config.disable_callback_thread   = true;
  config.seed_session_timeout_ms   = seed_session_timeout_ms;
  config.disable_seed_verification = disable_seed_verification;
  config.logger_func               = logger_func;

  colonio_t colonio;
  // TODO detect an error
  colonio_init(&colonio, &config);

  return reinterpret_cast<COLONIO_T>(colonio);
}

void js_connect(
    COLONIO_T c, PTR_T url, unsigned int url_siz, PTR_T token, unsigned int token_siz, PTR_T on_success,
    PTR_T on_failure) {
  colonio_t colonio = reinterpret_cast<colonio_t>(c);

  colonio_connect_async(
      colonio, reinterpret_cast<const char*>(url), url_siz, reinterpret_cast<const char*>(token), token_siz, nullptr,
      reinterpret_cast<void (*)(colonio_t, void*)>(on_success),
      reinterpret_cast<void (*)(colonio_t, void*, const colonio_error_t*)>(on_failure));
}

void js_disconnect(COLONIO_T c, PTR_T on_success, PTR_T on_failure) {
  colonio_t colonio = reinterpret_cast<colonio_t>(c);

  colonio_disconnect_async(
      colonio, nullptr, reinterpret_cast<void (*)(colonio_t, void*)>(on_success),
      reinterpret_cast<void (*)(colonio_t, void*, const colonio_error_t*)>(on_failure));
}

bool js_is_connected(COLONIO_T c) {
  colonio_t colonio = reinterpret_cast<colonio_t>(c);
  return colonio_is_connected(colonio);
}

void js_get_local_nid(COLONIO_T c, PTR_T nid) {
  colonio_t colonio = reinterpret_cast<colonio_t>(c);
  colonio_get_local_nid(colonio, reinterpret_cast<char*>(nid));
}

PTR_T js_set_position(COLONIO_T c, double x, double y) {
  colonio_t colonio = reinterpret_cast<colonio_t>(c);
  return reinterpret_cast<PTR_T>(colonio_set_position(colonio, &x, &y));
}

void js_quit(COLONIO_T c) {
  // TODO deal errors
  colonio_t colonio = reinterpret_cast<colonio_t>(c);
  colonio_quit(&colonio);
}

// messaging
void js_messaging_post(
    COLONIO_T c, PTR_T dst, PTR_T name, unsigned int name_siz, VALUE_T value, uint32_t opt, ID_T id) {
  colonio_messaging_post_async(
      reinterpret_cast<colonio_t>(c), reinterpret_cast<char*>(dst), reinterpret_cast<char*>(name), name_siz,
      reinterpret_cast<colonio_value_t>(value), opt, reinterpret_cast<void*>(id), messaging_post_on_success,
      messaging_post_on_failure);
}

void wrap_messaging_handler(
    colonio_t colonio, void* ptr, const colonio_messaging_request_t* request, colonio_messaging_writer_t writer) {
  messaging_handler(
      colonio, reinterpret_cast<ID_T>(ptr), reinterpret_cast<PTR_T>(request->source_nid),
      reinterpret_cast<VALUE_T>(request->message), request->options, reinterpret_cast<PTR_T>(writer));
}

void js_messaging_set_handler(COLONIO_T c, PTR_T name, unsigned int name_siz, ID_T id) {
  colonio_messaging_set_handler(
      reinterpret_cast<colonio_t>(c), reinterpret_cast<char*>(name), name_siz, reinterpret_cast<void*>(id),
      wrap_messaging_handler);
}

void js_messaging_response_writer(COLONIO_T c, PTR_T w, VALUE_T message) {
  colonio_messaging_response_writer(
      reinterpret_cast<colonio_t>(c), reinterpret_cast<colonio_messaging_writer_t>(w),
      reinterpret_cast<colonio_value_t>(message));
}

void js_messaging_unset_handler(COLONIO_T c, PTR_T name, unsigned int name_siz) {
  colonio_messaging_unset_handler(reinterpret_cast<colonio_t>(c), reinterpret_cast<char*>(name), name_siz);
}

// kvs
void wrap_kvs_local_data_handler(colonio_t colonio, void* ptr, colonio_kvs_data_t cur) {
  unsigned int siz = colonio_kvs_local_data_get_siz(colonio, cur);
  std::vector<const char*> keys(siz);
  for (unsigned int idx = 0; idx < siz; idx++) {
    keys.at(idx) = colonio_kvs_local_data_get_key(colonio, cur, idx, nullptr);
  }
  kvs_local_data_handler(colonio, reinterpret_cast<ID_T>(ptr), cur, reinterpret_cast<PTR_T>(keys.data()), siz);
}

void js_kvs_get_local_data(COLONIO_T c, ID_T id) {
  colonio_kvs_get_local_data_async(
      reinterpret_cast<colonio_t>(c), reinterpret_cast<void*>(id), wrap_kvs_local_data_handler);
}

VALUE_T js_kvs_local_data_get_value(COLONIO_T c, LOCAL_DATA_T cur, unsigned int idx) {
  colonio_value_t value;
  colonio_kvs_local_data_get_value(
      reinterpret_cast<colonio_t>(c), reinterpret_cast<colonio_kvs_data_t>(cur), idx, &value);
  return reinterpret_cast<VALUE_T>(value);
}

void js_kvs_local_data_free(COLONIO_T c, LOCAL_DATA_T cur) {
  colonio_kvs_local_data_free(reinterpret_cast<colonio_t>(c), reinterpret_cast<colonio_kvs_data_t>(cur));
}

void js_kvs_get(COLONIO_T c, PTR_T key, unsigned int key_siz, ID_T id) {
  colonio_kvs_get_async(
      reinterpret_cast<colonio_t>(c), reinterpret_cast<char*>(key), key_siz, reinterpret_cast<void*>(id),
      kvs_get_on_success, kvs_get_on_failure);
}

void js_kvs_set(COLONIO_T c, PTR_T key, unsigned int key_siz, VALUE_T value, uint32_t opt, ID_T id) {
  colonio_kvs_set_async(
      reinterpret_cast<colonio_t>(c), reinterpret_cast<char*>(key), key_siz, reinterpret_cast<colonio_value_t>(value),
      opt, reinterpret_cast<void*>(id), kvs_set_on_success, kvs_set_on_failure);
}

// spread
void js_spread_post(
    COLONIO_T c, double x, double y, double r, PTR_T name, unsigned int name_siz, VALUE_T value, uint32_t opt,
    ID_T id) {
  colonio_spread_post_async(
      reinterpret_cast<colonio_t>(c), x, y, r, reinterpret_cast<char*>(name), name_siz,
      reinterpret_cast<colonio_value_t>(value), opt, reinterpret_cast<void*>(id), spread_post_on_success,
      spread_post_on_failure);
}

void wrap_spread_handler(colonio_t colonio, void* ptr, const colonio_spread_request_t* request) {
  spread_handler(
      colonio, reinterpret_cast<ID_T>(ptr), reinterpret_cast<PTR_T>(request->source_nid),
      reinterpret_cast<VALUE_T>(request->message), request->options);
}

void js_spread_set_handler(COLONIO_T c, PTR_T name, unsigned int name_siz, ID_T id) {
  colonio_spread_set_handler(
      reinterpret_cast<colonio_t>(c), reinterpret_cast<char*>(name), name_siz, reinterpret_cast<void*>(id),
      wrap_spread_handler);
}

void js_spread_unset_handler(COLONIO_T c, PTR_T name, unsigned int name_siz) {
  colonio_spread_unset_handler(reinterpret_cast<colonio_t>(c), reinterpret_cast<char*>(name), name_siz);
}

VALUE_T js_value_create() {
  colonio_value_t value;
  colonio_value_create(&value);
  return reinterpret_cast<VALUE_T>(value);
}

int js_value_get_type(VALUE_T v) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  return static_cast<int>(colonio_value_get_type(value));
}

bool js_value_get_bool(VALUE_T v) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  return colonio_value_get_bool(value);
}

int64_t js_value_get_int(VALUE_T v) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  return colonio_value_get_int(value);
}

double js_value_get_double(VALUE_T v) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  return colonio_value_get_double(value);
}

int js_value_get_string_length(VALUE_T v) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  unsigned int siz;
  colonio_value_get_string(value, &siz);
  return siz;
}

PTR_T js_value_get_string(VALUE_T v) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  return reinterpret_cast<PTR_T>(colonio_value_get_string(value, nullptr));
}

int js_value_get_binary_length(VALUE_T v) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  unsigned int siz;
  colonio_value_get_binary(value, &siz);
  return siz;
}

PTR_T js_value_get_binary(VALUE_T v) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  return reinterpret_cast<PTR_T>(colonio_value_get_binary(value, nullptr));
}

void js_value_set_bool(VALUE_T v, bool b) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  colonio_value_set_bool(value, b);
}

void js_value_set_int(VALUE_T v, int64_t i) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  colonio_value_set_int(value, i);
}

void js_value_set_double(VALUE_T v, double d) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  colonio_value_set_double(value, d);
}

void js_value_set_string(VALUE_T v, PTR_T ptr, unsigned int siz) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  colonio_value_set_string(value, reinterpret_cast<const char*>(ptr), siz);
}

void js_value_set_binary(VALUE_T v, PTR_T ptr, unsigned int siz) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  colonio_value_set_binary(value, reinterpret_cast<const void*>(ptr), siz);
}

void js_value_free(VALUE_T v) {
  colonio_value_t value = reinterpret_cast<colonio_value_t>(v);
  colonio_value_free(&value);
}