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
#include "_cgo_export.h"

// copy functions from _obj/colonio/cgo2.c
static size_t _GoStringLen(_GoString_ s) {
  return (size_t)s.n;
}

static const char* _GoStringPtr(_GoString_ s) {
  return s.p;
}
// export constant value for golang
const unsigned int cgo_colonio_nid_length = COLONIO_NID_LENGTH;

// colonio
void cgo_wrap_colonio_logger(colonio_t colonio, const char* message, unsigned int len) {
  cgoWrapColonioLogger(colonio, (void*)message, (int)len);
}

colonio_error_t* cgo_colonio_init(colonio_t* colonio) {
  colonio_config_t config;
  colonio_config_set_default(&config);
  config.logger_func = cgo_wrap_colonio_logger;
  return colonio_init(colonio, &config);
}

colonio_error_t* cgo_colonio_connect(colonio_t colonio, _GoString_ url, _GoString_ token) {
  return colonio_connect(colonio, _GoStringPtr(url), _GoStringLen(url), _GoStringPtr(token), _GoStringLen(token));
}

int cgo_colonio_is_connected(colonio_t colonio) {
  return colonio_is_connected(colonio) ? 1 : 0;
}

// messaging
colonio_error_t* cgo_colonio_messaging_post(
    colonio_t colonio, _GoString_ dst, _GoString_ name, colonio_const_value_t val, uint32_t opt,
    colonio_value_t* result) {
  return colonio_messaging_post(colonio, _GoStringPtr(dst), _GoStringPtr(name), _GoStringLen(name), val, opt, result);
}

void cgo_wrap_colonio_messaging_handler(
    colonio_t colonio, void* id, const colonio_messaging_request_t* request, colonio_messaging_writer_t writer) {
  cgoWrapMessagingHandler(
      colonio, (unsigned long)id, (void*)request->source_nid, request->message, request->options, writer);
}

void cgo_colonio_messaging_set_handler(colonio_t colonio, _GoString_ name, unsigned long id) {
  colonio_messaging_set_handler(
      colonio, _GoStringPtr(name), _GoStringLen(name), (void*)id, cgo_wrap_colonio_messaging_handler);
}

void cgo_colonio_messaging_unset_handler(colonio_t colonio, _GoString_ name) {
  colonio_messaging_unset_handler(colonio, _GoStringPtr(name), _GoStringLen(name));
}

// kvs
/*
void cgo_cb_colonio_map_foreach_local_value(
    colonio_map_t *map, void *ptr, const colonio_value_t *key, const colonio_value_t *val, uint32_t attr) {
  cgoColonioMapForeachLocalValue(map, (void *)ptr, (void *)key, (void *)val, attr);
}

colonio_error_t *cgo_colonio_map_foreach_local_value(colonio_map_t *map, uintptr_t ptr) {
  return colonio_map_foreach_local_value(map, (void *)ptr, cgo_cb_colonio_map_foreach_local_value);
}
//*/
colonio_error_t* cgo_colonio_kvs_get(colonio_t colonio, _GoString_ key, colonio_value_t* dst) {
  return colonio_kvs_get(colonio, _GoStringPtr(key), _GoStringLen(key), dst);
}

colonio_error_t* cgo_colonio_kvs_set(colonio_t colonio, _GoString_ key, colonio_const_value_t value, uint32_t opt) {
  return colonio_kvs_set(colonio, _GoStringPtr(key), _GoStringLen(key), value, opt);
}

// spread
colonio_error_t* cgo_colonio_spread_post(
    colonio_t colonio, double x, double y, double r, _GoString_ name, colonio_const_value_t value, uint32_t opt) {
  return colonio_spread_post(colonio, x, y, r, _GoStringPtr(name), _GoStringLen(name), value, opt);
}

void cgo_wrap_colonio_spread_handler(colonio_t colonio, void* id, const colonio_spread_request_t* request) {
  cgoWrapSpreadHandler(colonio, (unsigned long)id, (void*)request->source_nid, request->message, request->options);
}

void cgo_colonio_spread_set_handler(colonio_t colonio, _GoString_ name, unsigned long id) {
  colonio_spread_set_handler(
      colonio, _GoStringPtr(name), _GoStringLen(name), (void*)id, cgo_wrap_colonio_spread_handler);
}

void cgo_colonio_spread_unset_handler(colonio_t colonio, _GoString_ name) {
  colonio_spread_unset_handler(colonio, _GoStringPtr(name), _GoStringLen(name));
}

// value
void cgo_colonio_value_set_string(colonio_value_t value, _GoString_ s) {
  colonio_value_set_string(value, _GoStringPtr(s), _GoStringLen(s));
}
